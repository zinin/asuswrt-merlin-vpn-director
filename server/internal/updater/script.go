package updater

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"text/template"
)

// LogFileName is the name of the update log file.
const LogFileName = "update.log"

//go:embed update_script.sh.tmpl
var updateScriptTemplate string

// RunOptions describes one update run.
type RunOptions struct {
	OldVersion string
	NewVersion string
	ChatID     int64  // 0 when the update was started from the Web UI
	Initiator  string // "bot" or "webui"
}

// scriptData holds the data for the update script template.
type scriptData struct {
	ChatID     int64
	Initiator  string
	OldVersion string
	NewVersion string
	UpdateDir  string
	FilesDir   string
	NotifyFile string
	LockFile   string
	Daemons    []Daemon
}

// RunUpdateScript generates the update script and runs it detached.
// Validates every string embedded into the script to prevent shell injection.
func (s *Service) RunUpdateScript(opts RunOptions) error {
	// Validate versions before embedding in shell script
	if !IsValidVersion(opts.OldVersion) {
		return fmt.Errorf("invalid old version: %q", opts.OldVersion)
	}
	if !IsValidVersion(opts.NewVersion) {
		return fmt.Errorf("invalid new version: %q", opts.NewVersion)
	}
	if opts.Initiator != "bot" && opts.Initiator != "webui" {
		return fmt.Errorf("invalid initiator: %q", opts.Initiator)
	}

	script, err := s.generateScript(opts)
	if err != nil {
		return fmt.Errorf("generate script: %w", err)
	}

	scriptPath := s.getScriptFile()

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0755); err != nil {
		return fmt.Errorf("create script directory: %w", err)
	}

	// Write script with execute permission
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("write script: %w", err)
	}

	// Open log file for script output
	updateDir := s.getUpdateDir()
	logFile := filepath.Join(updateDir, LogFileName)

	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	// Note: f will be inherited by child process via fork, so it stays open
	// even after bot dies. We don't close it here to avoid race with child.

	// Run script directly with Setsid for proper detachment.
	// Setsid creates a new session, so script survives bot termination.
	// Unlike "nohup ... &", this gives us proper error detection at exec level.
	cmd := exec.Command(s.getShell(), scriptPath)
	cmd.Dir = updateDir
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Detach into new session - survives parent death
	}

	if err := cmd.Start(); err != nil {
		f.Close()
		return fmt.Errorf("start script: %w", err)
	}

	// Don't wait - script runs in its own session and will kill this process.
	// Script continues running after bot dies because of Setsid.
	return nil
}

// generateScript creates the update script content from template.
func (s *Service) generateScript(opts RunOptions) (string, error) {
	tmpl, err := template.New("update").Parse(updateScriptTemplate)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	updateDir := s.getUpdateDir()

	data := scriptData{
		ChatID:     opts.ChatID,
		Initiator:  opts.Initiator,
		OldVersion: opts.OldVersion,
		NewVersion: opts.NewVersion,
		UpdateDir:  updateDir,
		FilesDir:   filepath.Join(updateDir, "files"),
		NotifyFile: filepath.Join(updateDir, "notify.json"),
		LockFile:   filepath.Join(updateDir, "lock"),
		Daemons:    Daemons,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}
