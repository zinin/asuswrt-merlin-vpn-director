// Package devmode provides development mode utilities for local testing
package devmode

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/zinin/vpn-director/server/internal/service"
	"github.com/zinin/vpn-director/server/internal/shell"
)

// devPlatformFile is what the mock answers for `vpn-director.sh platform`,
// relative to server/ like every other dev path.
const devPlatformFile = "testdata/dev/platform.json"

// safeCommands lists commands that are safe to execute in dev mode
var safeCommands = map[string]bool{
	"curl": true,
	"tail": true,
}

// Executor implements ShellExecutor with safe/mock command handling for dev mode.
// Safe commands (curl, tail) execute via real executor.
// Router commands (vpn-director.sh) return mock responses.
// Unknown commands fail with exit code 1.
type Executor struct {
	real         service.ShellExecutor
	platformFile string
}

// Compile-time interface check
var _ service.ShellExecutor = (*Executor)(nil)

// NewExecutor creates a new dev mode executor with default real executor
func NewExecutor() *Executor {
	return &Executor{real: service.DefaultExecutor(), platformFile: devPlatformFile}
}

// NewExecutorWithReal creates a new dev mode executor with a custom real executor
// (useful for testing)
func NewExecutorWithReal(real service.ShellExecutor) *Executor {
	return &Executor{real: real, platformFile: devPlatformFile}
}

// Exec executes a command, routing to real executor for safe commands,
// mock responses for router commands, or failing for unknown commands.
func (e *Executor) Exec(ctx context.Context, name string, args ...string) (*shell.Result, error) {
	baseName := filepath.Base(name)

	// Check if it's a safe command
	if e.isSafe(baseName) {
		slog.Info("DEV: executing safe command", "command", baseName, "args", args)
		return e.real.Exec(ctx, name, args...)
	}

	// Check if it's a vpn-director.sh command
	if baseName == "vpn-director.sh" {
		return e.mockVPNDirector(args...)
	}

	// Unknown command - fail
	slog.Warn("DEV: unknown command blocked", "command", name, "args", args)
	return &shell.Result{
		Output:   "dev mode: unknown command not allowed",
		ExitCode: 1,
	}, nil
}

// isSafe checks if a command is in the safe list
func (e *Executor) isSafe(baseName string) bool {
	return safeCommands[baseName]
}

// mockVPNDirector returns mock responses for vpn-director.sh commands
func (e *Executor) mockVPNDirector(args ...string) (*shell.Result, error) {
	// Options such as --wait precede the command; the mock ignores them.
	for len(args) > 0 && strings.HasPrefix(args[0], "--") {
		args = args[1:]
	}

	if len(args) == 0 {
		slog.Info("DEV: mock command", "command", "vpn-director.sh", "args", args)
		return &shell.Result{
			Output:   "[DEV MODE] vpn-director.sh: no command specified",
			ExitCode: 1,
		}, nil
	}

	cmd := args[0]
	slog.Info("DEV: mock command", "command", "vpn-director.sh", "subcommand", cmd)

	switch cmd {
	case "status":
		return &shell.Result{
			Output:   "[DEV MODE] VPN Director Status\n  Xray: running (mock)\n  Tunnel Director: running (mock)",
			ExitCode: 0,
		}, nil
	case "restart":
		return &shell.Result{
			Output:   "[DEV MODE] VPN Director restarted",
			ExitCode: 0,
		}, nil
	case "stop":
		return &shell.Result{
			Output:   "[DEV MODE] VPN Director stopped",
			ExitCode: 0,
		}, nil
	case "apply":
		return &shell.Result{
			Output:   "[DEV MODE] Configuration applied",
			ExitCode: 0,
		}, nil
	case "update":
		return &shell.Result{
			Output:   "[DEV MODE] IPsets updated and configuration reapplied",
			ExitCode: 0,
		}, nil
	case "platform":
		return e.mockPlatform()
	default:
		return &shell.Result{
			Output:   "[DEV MODE] vpn-director.sh: unknown command: " + cmd,
			ExitCode: 1,
		}, nil
	}
}

// mockPlatform answers with the dev platform document on one line, the way
// the CLI prints it (the service reads the last line of the output).
func (e *Executor) mockPlatform() (*shell.Result, error) {
	data, err := os.ReadFile(e.platformFile)
	if err != nil {
		return &shell.Result{Output: "dev mode: " + err.Error(), ExitCode: 1}, nil
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, data); err != nil {
		return &shell.Result{Output: "dev mode: " + e.platformFile + ": " + err.Error(), ExitCode: 1}, nil
	}
	return &shell.Result{Output: buf.String() + "\n", ExitCode: 0}, nil
}
