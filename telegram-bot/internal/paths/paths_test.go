package paths

import (
	"strings"
	"testing"
)

func TestDefault(t *testing.T) {
	p := Default()

	tests := []struct {
		name     string
		got      string
		wantPfx  string
		wantSfx  string
	}{
		{"ScriptsDir", p.ScriptsDir, "/opt/vpn-director", ""},
		{"BotConfigPath", p.BotConfigPath, "/opt/vpn-director/", "telegram-bot.json"},
		{"DefaultDataDir", p.DefaultDataDir, "/opt/vpn-director/", "data"},
		{"XrayTemplate", p.XrayTemplate, "/opt/etc/xray/", ".template"},
		{"XrayConfig", p.XrayConfig, "/opt/etc/xray/", ".json"},
		{"BotLogPath", p.BotLogPath, "/tmp/", "telegram-bot.log"},
		{"VPNLogPath", p.VPNLogPath, "/tmp/", "vpn-director.log"},
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
}

func TestDevPaths(t *testing.T) {
	p := DevPaths()

	tests := []struct {
		name     string
		got      string
		wantPfx  string
		wantSfx  string
	}{
		{"ScriptsDir", p.ScriptsDir, "testdata/dev", ""},
		{"BotConfigPath", p.BotConfigPath, "testdata/dev/", "telegram-bot.json"},
		{"DefaultDataDir", p.DefaultDataDir, "testdata/dev/", "data"},
		{"XrayTemplate", p.XrayTemplate, "testdata/dev/", "xray.template.json"},
		{"XrayConfig", p.XrayConfig, "testdata/dev/", "xray.json"},
		{"BotLogPath", p.BotLogPath, "testdata/dev/", "bot.log"},
		{"VPNLogPath", p.VPNLogPath, "testdata/dev/", "vpn.log"},
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
