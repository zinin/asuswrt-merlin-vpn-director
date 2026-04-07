package updater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateScript(t *testing.T) {
	tmpDir := t.TempDir()

	s := &Service{
		updateDir: tmpDir,
	}

	script, err := s.generateScript(123456789, "v1.0.0", "v1.1.0")
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	// Check that variables are embedded correctly
	checks := []string{
		"CHAT_ID=123456789",
		`OLD_VERSION="v1.0.0"`,
		`NEW_VERSION="v1.1.0"`,
		"set -e",
		"S98telegram-bot stop",
		"S98telegram-bot start",
		`pgrep -f "$BOT_PATH"`, // full path variable used
		"BOT_PATH=\"/opt/vpn-director/telegram-bot\"",
	}

	for _, check := range checks {
		if !strings.Contains(script, check) {
			t.Errorf("Script missing %q", check)
		}
	}

	// Check that monit commands have || true (optional commands)
	if !strings.Contains(script, "monit unmonitor telegram-bot 2>/dev/null || true") {
		t.Error("Script missing || true for monit unmonitor")
	}
	if !strings.Contains(script, "monit monitor telegram-bot 2>/dev/null || true") {
		t.Error("Script missing || true for monit monitor")
	}

	// Check that cp commands do NOT have || true (critical commands)
	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "cp -f") {
			if strings.HasSuffix(trimmed, "|| true") {
				t.Errorf("cp command should not have || true: %s", line)
			}
		}
	}
}

func TestGenerateScript_PathsCorrect(t *testing.T) {
	tmpDir := t.TempDir()

	s := &Service{
		updateDir: tmpDir,
	}

	script, err := s.generateScript(999, "v1.0.0", "v2.0.0")
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	// Verify paths use the custom updateDir
	expectedPaths := []string{
		`UPDATE_DIR="` + tmpDir + `"`,
		`FILES_DIR="` + tmpDir + `/files"`,
		`NOTIFY_FILE="` + tmpDir + `/notify.json"`,
		`LOCK_FILE="` + tmpDir + `/lock"`,
	}

	for _, expected := range expectedPaths {
		if !strings.Contains(script, expected) {
			t.Errorf("Script missing path %q", expected)
		}
	}
}

func TestRunUpdateScript_InvalidVersion(t *testing.T) {
	tmpDir := t.TempDir()

	s := &Service{
		updateDir:  tmpDir,
		scriptFile: filepath.Join(tmpDir, "update.sh"),
	}

	tests := []struct {
		name       string
		oldVersion string
		newVersion string
		wantErr    string
	}{
		{
			name:       "shell injection in old version",
			oldVersion: "v1.0.0;rm -rf /",
			newVersion: "v1.1.0",
			wantErr:    "invalid old version",
		},
		{
			name:       "shell injection in new version",
			oldVersion: "v1.0.0",
			newVersion: "v1.1.0$(whoami)",
			wantErr:    "invalid new version",
		},
		{
			name:       "backticks in old version",
			oldVersion: "`id`",
			newVersion: "v1.1.0",
			wantErr:    "invalid old version",
		},
		{
			name:       "quotes in new version",
			oldVersion: "v1.0.0",
			newVersion: `v1.1.0"test`,
			wantErr:    "invalid new version",
		},
		{
			name:       "empty old version",
			oldVersion: "",
			newVersion: "v1.1.0",
			wantErr:    "invalid old version",
		},
		{
			name:       "too long version",
			oldVersion: "v1.0.0",
			newVersion: strings.Repeat("v", 100),
			wantErr:    "invalid new version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.RunUpdateScript(123, tt.oldVersion, tt.newVersion)
			if err == nil {
				t.Error("Expected error for invalid version")
				return
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRunUpdateScript_ValidVersion(t *testing.T) {
	tmpDir := t.TempDir()

	s := &Service{
		updateDir:  tmpDir,
		scriptFile: filepath.Join(tmpDir, "update.sh"),
	}

	// This will fail at cmd.Start() because nohup/sh may not exist in test env
	// but the important part is that it passes validation
	err := s.RunUpdateScript(123, "v1.0.0", "v1.1.0")

	// If we get "start script" error, validation passed
	// If we get "invalid version", validation failed
	if err != nil && strings.Contains(err.Error(), "invalid") {
		t.Errorf("Valid versions should pass validation: %v", err)
	}

	// Check that script file was created
	scriptPath := filepath.Join(tmpDir, "update.sh")
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		t.Error("Script file should be created")
	}
}

func TestRunUpdateScript_ScriptContent(t *testing.T) {
	tmpDir := t.TempDir()

	s := &Service{
		updateDir:  tmpDir,
		scriptFile: filepath.Join(tmpDir, "update.sh"),
	}

	// Run will likely fail, but script should be written
	_ = s.RunUpdateScript(42, "v1.2.3", "v2.0.0")

	// Read the generated script
	scriptPath := filepath.Join(tmpDir, "update.sh")
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("Failed to read script: %v", err)
	}

	script := string(content)

	// Verify script has correct shebang
	if !strings.HasPrefix(script, "#!/bin/sh") {
		t.Error("Script should start with #!/bin/sh")
	}

	// Verify embedded values
	if !strings.Contains(script, "CHAT_ID=42") {
		t.Error("Script missing correct CHAT_ID")
	}
	if !strings.Contains(script, `OLD_VERSION="v1.2.3"`) {
		t.Error("Script missing correct OLD_VERSION")
	}
	if !strings.Contains(script, `NEW_VERSION="v2.0.0"`) {
		t.Error("Script missing correct NEW_VERSION")
	}
}
