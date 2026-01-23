// Package logging provides log management utilities
package logging

import (
	"io"
	"log"
	"os"
)

// Logger manages application logging
type Logger struct {
	file *os.File
}

// New creates a new Logger that writes to both stdout and the specified file
func New(logPath string) (*Logger, error) {
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.SetOutput(io.MultiWriter(os.Stdout, file))

	return &Logger{file: file}, nil
}

// Close closes the log file
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
