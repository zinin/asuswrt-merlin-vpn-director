// Package logging provides log management utilities
package logging

import (
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
)

// Logger manages the log file, level, and provides rotation
type Logger struct {
	file     *os.File
	levelVar *slog.LevelVar
}

// NewSlogLogger creates a new slog.Logger that writes to both stdout and the specified file.
// Initial level is INFO. Use SetLevel() to change after loading config.
// Also redirects standard log package output to the same destinations.
func NewSlogLogger(logPath string) (*slog.Logger, *Logger, error) {
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, nil, err
	}

	multiWriter := io.MultiWriter(os.Stdout, file)

	// Redirect standard log to the same output (for dependencies like telegram-bot-api)
	log.SetOutput(multiWriter)
	log.SetFlags(log.Ldate | log.Ltime)

	levelVar := new(slog.LevelVar)
	levelVar.Set(slog.LevelInfo)

	handler := slog.NewTextHandler(
		multiWriter,
		&slog.HandlerOptions{
			Level:     levelVar,
			AddSource: true,
		},
	)

	return slog.New(handler), &Logger{file: file, levelVar: levelVar}, nil
}

// SetLevel changes the log level dynamically.
// Valid levels: debug, info, warn, error (case-insensitive).
// Invalid or empty values default to info with a warning.
func (l *Logger) SetLevel(level string) {
	parsedLevel, valid := parseLevel(level)
	if !valid && level != "" {
		slog.Warn("Unknown log_level, using info", "value", level)
	}
	l.levelVar.Set(parsedLevel)
}

func parseLevel(level string) (slog.Level, bool) {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug, true
	case "info", "":
		return slog.LevelInfo, true
	case "warn":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return slog.LevelInfo, false
	}
}

// Close closes the log file
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
