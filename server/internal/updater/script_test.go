package updater

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// validOpts returns options that pass every validation, so a test can flip a
// single field and assert that this one field is what got rejected.
func validOpts() RunOptions {
	return RunOptions{OldVersion: "v1.0.0", NewVersion: "v1.1.0", ChatID: 123, Initiator: "bot"}
}

func TestGenerateScript(t *testing.T) {
	tmpDir := t.TempDir()

	s := &Service{
		updateDir: tmpDir,
	}

	script, err := s.generateScript(RunOptions{ChatID: 123456789, OldVersion: "v1.0.0", NewVersion: "v1.1.0", Initiator: "bot"})
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	// Check that variables are embedded correctly
	checks := []string{
		"CHAT_ID=123456789",
		`OLD_VERSION="v1.0.0"`,
		`NEW_VERSION="v1.1.0"`,
		"set -e",
		`pgrep -f "$bin"`, // full binary path, from the daemon table
		"telegram-bot|/opt/vpn-director/telegram-bot|S98telegram-bot",
		"webui|/opt/vpn-director/webui|S98vpn-director-webui",
	}

	for _, check := range checks {
		if !strings.Contains(script, check) {
			t.Errorf("Script missing %q", check)
		}
	}

	// monit commands stay optional
	if !strings.Contains(script, `monit unmonitor "${entry%%|*}" 2>/dev/null || true`) {
		t.Error("Script missing || true for monit unmonitor")
	}
	if !strings.Contains(script, `monit monitor "${entry%%|*}" 2>/dev/null || true`) {
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

	script, err := s.generateScript(RunOptions{ChatID: 999, OldVersion: "v1.0.0", NewVersion: "v2.0.0", Initiator: "bot"})
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

func TestGenerateScript_EmbedsInitiator(t *testing.T) {
	tmpDir := t.TempDir()
	s := &Service{updateDir: tmpDir}

	script, err := s.generateScript(RunOptions{
		OldVersion: "v1.0.0", NewVersion: "v1.1.0", ChatID: 0, Initiator: "webui",
	})
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}
	if !strings.Contains(script, `INITIATOR="webui"`) {
		t.Error("script must carry the initiator so notify.json can name it")
	}
	if !strings.Contains(script, "CHAT_ID=0") {
		t.Error("a Web UI update has chat_id 0")
	}
}

func TestGenerateScript_CoversEveryDaemon(t *testing.T) {
	s := &Service{}
	script, err := s.generateScript(validOpts())
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	for _, d := range Daemons {
		for _, want := range []string{d.Name, d.Binary, d.InitScript} {
			if !strings.Contains(script, want) {
				t.Errorf("script missing %q for daemon %s", want, d.Name)
			}
		}
	}

	// The daemon table drives every loop, so it is the only place an init
	// script may be named. A second mention is a hardcoded call, and a
	// hardcoded call silently skips the other daemon.
	for _, d := range Daemons {
		if n := strings.Count(script, d.InitScript); n != 1 {
			t.Errorf("init script %s named %d times, want exactly 1 (only in the DAEMONS table)", d.InitScript, n)
		}
	}
}

func TestGenerateScript_RecoveryOnFailure(t *testing.T) {
	s := &Service{}
	script, err := s.generateScript(validOpts())
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	if !strings.Contains(script, "trap on_exit EXIT") {
		t.Error("script must install the EXIT trap: ash has no ERR trap")
	}

	// Every other needle has to sit inside the handler. Searching the whole
	// script would pass on the happy-path copy of the same line, so removing
	// the step from the recovery path would go unnoticed.
	body := onExitBody(t, script)
	checks := map[string]string{
		"code=$?":             "the EXIT trap must inspect the exit code",
		"set +e":              "recovery must survive its own failing steps",
		"write_notify failed": "a failed update must leave status failed in notify.json",
		"start_running":       "recovery must restart the daemons that were running",
		`rm -f "$LOCK_FILE"`:  "a failed update must release the lock",
	}
	for needle, why := range checks {
		if !strings.Contains(body, needle) {
			t.Errorf("on_exit missing %q: %s", needle, why)
		}
	}
}

// onExitBody returns the body of the on_exit handler, so a recovery
// assertion is about the recovery path and not about the script mentioning
// the words somewhere.
func onExitBody(t *testing.T, script string) string {
	t.Helper()

	const open = "on_exit() {\n"
	i := strings.Index(script, open)
	if i < 0 {
		t.Fatal("script has no on_exit handler")
	}
	body := script[i+len(open):]
	j := strings.Index(body, "\n}\n")
	if j < 0 {
		t.Fatal("on_exit handler is never closed")
	}
	return body[:j]
}

func TestGenerateScript_NotifyFormat(t *testing.T) {
	s := &Service{}
	script, err := s.generateScript(RunOptions{
		OldVersion: "v1.2.0", NewVersion: "v1.3.0", ChatID: 0, Initiator: "webui",
	})
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	const want = `{"chat_id":$CHAT_ID,"old_version":"$OLD_VERSION","new_version":"$NEW_VERSION","status":"$1","initiator":"$INITIATOR"}`
	if !strings.Contains(script, want) {
		t.Errorf("notify.json template line missing or changed, want %s", want)
	}
}

func TestGenerateScript_IsValidShell(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}

	s := &Service{}
	script, err := s.generateScript(validOpts())
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	path := filepath.Join(t.TempDir(), "update.sh")
	if err := os.WriteFile(path, []byte(script), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(sh, "-n", path).CombinedOutput()
	if err != nil {
		t.Fatalf("generated script has a syntax error: %v\n%s", err, out)
	}
}

func TestGenerateScript_Golden(t *testing.T) {
	// Default paths keep the render deterministic, so the golden file shows
	// the exact script that ships to routers. Regenerate with:
	//   UPDATE_GOLDEN=1 go test ./internal/updater -run TestGenerateScript_Golden -count=1
	s := &Service{}
	script, err := s.generateScript(RunOptions{
		OldVersion: "v1.2.0", NewVersion: "v1.3.0", ChatID: 42, Initiator: "bot",
	})
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	golden := filepath.Join("testdata", "update_script.golden.sh")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(script), 0644); err != nil {
			t.Fatal(err)
		}
		t.Logf("rewrote %s", golden)
		return
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden: %v (regenerate with UPDATE_GOLDEN=1)", err)
	}
	if string(want) != script {
		t.Errorf("generated script differs from %s; review the diff and regenerate with UPDATE_GOLDEN=1 if the change is intended", golden)
	}
}

func TestRunUpdateScript_InvalidVersion(t *testing.T) {
	tmpDir := t.TempDir()

	s := &Service{
		updateDir:  tmpDir,
		scriptFile: filepath.Join(tmpDir, "update.sh"),
	}

	tests := []struct {
		name    string
		mutate  func(*RunOptions)
		wantErr string
	}{
		{
			name:    "shell injection in old version",
			mutate:  func(o *RunOptions) { o.OldVersion = "v1.0.0;rm -rf /" },
			wantErr: "invalid old version",
		},
		{
			name:    "shell injection in new version",
			mutate:  func(o *RunOptions) { o.NewVersion = "v1.1.0$(whoami)" },
			wantErr: "invalid new version",
		},
		{
			name:    "backticks in old version",
			mutate:  func(o *RunOptions) { o.OldVersion = "`id`" },
			wantErr: "invalid old version",
		},
		{
			name:    "quotes in new version",
			mutate:  func(o *RunOptions) { o.NewVersion = `v1.1.0"test` },
			wantErr: "invalid new version",
		},
		{
			name:    "empty old version",
			mutate:  func(o *RunOptions) { o.OldVersion = "" },
			wantErr: "invalid old version",
		},
		{
			name:    "too long version",
			mutate:  func(o *RunOptions) { o.NewVersion = strings.Repeat("v", 100) },
			wantErr: "invalid new version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := validOpts()
			tt.mutate(&opts)

			err := s.RunUpdateScript(opts)
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

func TestRunUpdateScript_InvalidInitiator(t *testing.T) {
	// Initiator is interpolated into the shell script the same way versions
	// are, so it gets the same allow-list treatment.
	tmpDir := t.TempDir()
	s := &Service{updateDir: tmpDir, scriptFile: filepath.Join(tmpDir, "update.sh")}

	for _, initiator := range []string{"", "cron", `bot";rm -rf /;"`} {
		t.Run(initiator, func(t *testing.T) {
			opts := validOpts()
			opts.Initiator = initiator
			err := s.RunUpdateScript(opts)
			if err == nil || !strings.Contains(err.Error(), "invalid initiator") {
				t.Fatalf("RunUpdateScript(initiator=%q) error = %v, want invalid initiator", initiator, err)
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
	err := s.RunUpdateScript(validOpts())

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
	_ = s.RunUpdateScript(RunOptions{ChatID: 42, OldVersion: "v1.2.3", NewVersion: "v2.0.0", Initiator: "bot"})

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
