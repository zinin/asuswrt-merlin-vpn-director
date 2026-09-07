package paths

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDefault(t *testing.T) {
	p := Default()

	tests := []struct {
		name    string
		got     string
		wantPfx string
		wantSfx string
	}{
		{"ScriptsDir", p.ScriptsDir, "/opt/vpn-director", ""},
		{"BotConfigPath", p.BotConfigPath, "/opt/vpn-director/", "telegram-bot.json"},
		{"DefaultDataDir", p.DefaultDataDir, "/opt/vpn-director/", "data"},
		{"XrayTemplate", p.XrayTemplate, "/opt/etc/xray/", ".template"},
		{"XrayConfig", p.XrayConfig, "/opt/etc/xray/", ".json"},
		{"BotLogPath", p.BotLogPath, "/tmp/", "telegram-bot.log"},
		{"VPNLogPath", p.VPNLogPath, "/tmp/", "vpn-director.log"},
		{"WebUILogPath", p.WebUILogPath, "/tmp/", "vpn-director-webui.log"},
		{"XrayLogPath", p.XrayLogPath, "/tmp/", "xray-error.log"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got == "" {
				t.Errorf("%s is empty", tt.name)
			}
			if tt.wantPfx != "" && !strings.HasPrefix(tt.got, tt.wantPfx) {
				t.Errorf("%s = %q, want prefix %q", tt.name, tt.got, tt.wantPfx)
			}
			if tt.wantSfx != "" && !strings.HasSuffix(tt.got, tt.wantSfx) {
				t.Errorf("%s = %q, want suffix %q", tt.name, tt.got, tt.wantSfx)
			}
		})
	}
}

func TestDefaultNotEmpty(t *testing.T) {
	p := Default()

	if p.ScriptsDir == "" {
		t.Error("ScriptsDir should not be empty")
	}
	if p.BotConfigPath == "" {
		t.Error("BotConfigPath should not be empty")
	}
	if p.DefaultDataDir == "" {
		t.Error("DefaultDataDir should not be empty")
	}
	if p.XrayTemplate == "" {
		t.Error("XrayTemplate should not be empty")
	}
	if p.XrayConfig == "" {
		t.Error("XrayConfig should not be empty")
	}
	if p.BotLogPath == "" {
		t.Error("BotLogPath should not be empty")
	}
	if p.VPNLogPath == "" {
		t.Error("VPNLogPath should not be empty")
	}
	if p.WebUILogPath == "" {
		t.Error("WebUILogPath should not be empty")
	}
	if p.XrayLogPath == "" {
		t.Error("XrayLogPath should not be empty")
	}
}

func TestDevPaths(t *testing.T) {
	p := DevPaths()

	tests := []struct {
		name    string
		got     string
		wantPfx string
		wantSfx string
	}{
		{"ScriptsDir", p.ScriptsDir, "testdata/dev", ""},
		{"BotConfigPath", p.BotConfigPath, "testdata/dev/", "telegram-bot.json"},
		{"DefaultDataDir", p.DefaultDataDir, "testdata/dev/", "data"},
		{"XrayTemplate", p.XrayTemplate, "testdata/dev/", "xray.template.json"},
		{"XrayConfig", p.XrayConfig, "testdata/dev/", "xray.json"},
		{"BotLogPath", p.BotLogPath, "testdata/dev/", "bot.log"},
		{"VPNLogPath", p.VPNLogPath, "testdata/dev/", "vpn.log"},
		{"WebUILogPath", p.WebUILogPath, "testdata/dev/", "webui.log"},
		{"XrayLogPath", p.XrayLogPath, "testdata/dev/", "xray-error.log"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got == "" {
				t.Errorf("%s is empty", tt.name)
			}
			if tt.wantPfx != "" && !strings.HasPrefix(tt.got, tt.wantPfx) {
				t.Errorf("%s = %q, want prefix %q", tt.name, tt.got, tt.wantPfx)
			}
			if tt.wantSfx != "" && !strings.HasSuffix(tt.got, tt.wantSfx) {
				t.Errorf("%s = %q, want suffix %q", tt.name, tt.got, tt.wantSfx)
			}
		})
	}
}

func TestRotatedLogs(t *testing.T) {
	p := Default()
	want := []string{p.BotLogPath, p.VPNLogPath, p.WebUILogPath, p.XrayLogPath}
	if got := p.RotatedLogs(); !reflect.DeepEqual(got, want) {
		t.Errorf("RotatedLogs() = %v, want %v", got, want)
	}
	for _, path := range p.RotatedLogs() {
		if path == "" {
			t.Error("RotatedLogs contains an empty path")
		}
	}
}

// A daemon started by an update script older than the fix inherits
// /tmp/vpn-director-update as its working directory, and the bot deletes that
// directory the moment it reports the update. On the router the leftover is
// not cosmetic: monit refuses to run without a working directory, and every
// shell spawned from here prints "shell-init: error retrieving current
// directory" into whatever the Web UI is showing.
func TestEnsureWorkingDirectory_LeavesADirectoryThatIsGone(t *testing.T) {
	before, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(before) })

	dir, err := os.MkdirTemp("", "deleted-cwd")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Getwd(); err == nil {
		t.Skip("this platform still resolves a deleted working directory")
	}

	if !EnsureWorkingDirectory() {
		t.Error("EnsureWorkingDirectory() = false, want it to report the move")
	}
	got, err := os.Getwd()
	if err != nil {
		t.Fatalf("the working directory is still gone: %v", err)
	}
	if got != "/" {
		t.Errorf("moved to %q, want /", got)
	}
}

// Dev mode resolves testdata/dev/... against the working directory, so one
// that exists is never taken away.
func TestEnsureWorkingDirectory_LeavesALiveDirectoryAlone(t *testing.T) {
	before, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(before) })

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	if EnsureWorkingDirectory() {
		t.Error("EnsureWorkingDirectory() = true, want it to leave a live directory alone")
	}
	got, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil && got != resolved && got != dir {
		t.Errorf("working directory is %q, want it left at %q", got, dir)
	}
}
