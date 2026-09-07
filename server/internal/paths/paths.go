// Package paths provides centralized path configuration for the application
package paths

import "os"

// Paths holds all configurable paths for the application
type Paths struct {
	ScriptsDir     string // /opt/vpn-director
	BotConfigPath  string // /opt/vpn-director/telegram-bot.json
	DefaultDataDir string // /opt/vpn-director/data
	XrayTemplate   string // /opt/etc/xray/config.json.template
	XrayConfig     string // /opt/etc/xray/config.json
	BotLogPath     string // /tmp/telegram-bot.log
	VPNLogPath     string // /tmp/vpn-director.log
	WebUILogPath   string // /tmp/vpn-director-webui.log
	XrayLogPath    string // /tmp/xray-error.log (set by the log section of the Xray template)
}

// Default returns the default paths for production use
func Default() Paths {
	return Paths{
		ScriptsDir:     "/opt/vpn-director",
		BotConfigPath:  "/opt/vpn-director/telegram-bot.json",
		DefaultDataDir: "/opt/vpn-director/data",
		XrayTemplate:   "/opt/etc/xray/config.json.template",
		XrayConfig:     "/opt/etc/xray/config.json",
		BotLogPath:     "/tmp/telegram-bot.log",
		VPNLogPath:     "/tmp/vpn-director.log",
		WebUILogPath:   "/tmp/vpn-director-webui.log",
		XrayLogPath:    "/tmp/xray-error.log",
	}
}

// DevPaths returns paths for development mode using testdata/dev/
func DevPaths() Paths {
	return Paths{
		ScriptsDir:     "testdata/dev",
		BotConfigPath:  "testdata/dev/telegram-bot.json",
		DefaultDataDir: "testdata/dev/data",
		XrayTemplate:   "testdata/dev/xray.template.json",
		XrayConfig:     "testdata/dev/xray.json",
		BotLogPath:     "testdata/dev/bot.log",
		VPNLogPath:     "testdata/dev/vpn.log",
		WebUILogPath:   "testdata/dev/webui.log",
		XrayLogPath:    "testdata/dev/xray-error.log",
	}
}

// RotatedLogs lists every log file the daemons truncate at
// logging.DefaultMaxSize. The bot and the Web UI rotate the same list;
// os.Truncate is idempotent, so two processes rotating at once are safe.
func (p Paths) RotatedLogs() []string {
	return []string{p.BotLogPath, p.VPNLogPath, p.WebUILogPath, p.XrayLogPath}
}

// EnsureWorkingDirectory moves the process to / when the directory it was
// started in has been removed, and reports whether it had to.
//
// A daemon started by an update script that ran from /tmp/vpn-director-update
// inherits that directory, and the bot deletes it the moment it reports the
// update. What is left is a process whose working directory does not exist,
// which on the router is not cosmetic: monit refuses to run at all without
// one, and every shell spawned from here prints "shell-init: error retrieving
// current directory" into whatever the Web UI is showing.
//
// A working directory that exists is left where it is, whatever it is: dev
// mode resolves DevPaths against it.
func EnsureWorkingDirectory() bool {
	if _, err := os.Getwd(); err == nil {
		return false
	}
	return os.Chdir("/") == nil
}
