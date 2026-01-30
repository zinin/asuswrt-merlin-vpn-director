package updater

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

//go:embed update_script.sh.tmpl
var updateScriptTemplate string

// scriptData holds the data for the update script template.
type scriptData struct {
	ChatID     int64
	OldVersion string
	NewVersion string
	UpdateDir  string
	FilesDir   string
	NotifyFile string
	LockFile   string
}

// RunUpdateScript generates the update script and runs it detached.
// Validates version strings before embedding to prevent shell injection.
func (s *Service) RunUpdateScript(chatID int64, oldVersion, newVersion string) error {
	// Validate versions before embedding in shell script
	if !IsValidVersion(oldVersion) {
		return fmt.Errorf("invalid old version: %q", oldVersion)
	}
	if !IsValidVersion(newVersion) {
		return fmt.Errorf("invalid new version: %q", newVersion)
	}

	script, err := s.generateScript(chatID, oldVersion, newVersion)
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

	// Run detached: nohup script >> log 2>&1 &
	updateDir := s.getUpdateDir()
	logFile := filepath.Join(updateDir, "update.log")

	cmd := exec.Command("/bin/sh", "-c",
		fmt.Sprintf("nohup %s >> %s 2>&1 &", scriptPath, logFile))

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start script: %w", err)
	}

	// Don't wait - script will kill this process
	return nil
}

// generateScript creates the update script content from template.
func (s *Service) generateScript(chatID int64, oldVersion, newVersion string) (string, error) {
	tmpl, err := template.New("update").Parse(updateScriptTemplate)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	updateDir := s.getUpdateDir()

	data := scriptData{
		ChatID:     chatID,
		OldVersion: oldVersion,
		NewVersion: newVersion,
		UpdateDir:  updateDir,
		FilesDir:   filepath.Join(updateDir, "files"),
		NotifyFile: filepath.Join(updateDir, "notify.json"),
		LockFile:   filepath.Join(updateDir, "lock"),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}
