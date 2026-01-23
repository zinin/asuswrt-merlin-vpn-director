// Package devmode provides development mode utilities for local testing
package devmode

import (
	"log/slog"
	"path/filepath"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/service"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/shell"
)

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
	real service.ShellExecutor
}

// Compile-time interface check
var _ service.ShellExecutor = (*Executor)(nil)

// NewExecutor creates a new dev mode executor with default real executor
func NewExecutor() *Executor {
	return &Executor{real: service.DefaultExecutor()}
}

// NewExecutorWithReal creates a new dev mode executor with a custom real executor
// (useful for testing)
func NewExecutorWithReal(real service.ShellExecutor) *Executor {
	return &Executor{real: real}
}

// Exec executes a command, routing to real executor for safe commands,
// mock responses for router commands, or failing for unknown commands.
func (e *Executor) Exec(name string, args ...string) (*shell.Result, error) {
	baseName := filepath.Base(name)

	// Check if it's a safe command
	if e.isSafe(baseName) {
		slog.Info("DEV: executing safe command", "command", baseName, "args", args)
		return e.real.Exec(name, args...)
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
	default:
		return &shell.Result{
			Output:   "[DEV MODE] vpn-director.sh: unknown command: " + cmd,
			ExitCode: 1,
		}, nil
	}
}
