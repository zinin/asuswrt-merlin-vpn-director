// internal/service/logs.go
package service

import (
	"context"
	"fmt"
	"time"
)

// TailTimeout bounds `tail -n` on a log file; reading a local file should
// never take long, so a hit means a hung filesystem, not a slow log.
const TailTimeout = 10 * time.Second

// LogService handles reading log files
type LogService struct {
	executor ShellExecutor
}

// Compile-time check that LogService implements LogReader
var _ LogReader = (*LogService)(nil)

// NewLogService creates a new LogService
func NewLogService(executor ShellExecutor) *LogService {
	if executor == nil {
		executor = DefaultExecutor()
	}
	return &LogService{executor: executor}
}

// Read reads the last n lines from a log file
func (s *LogService) Read(path string, lines int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), TailTimeout)
	defer cancel()
	result, err := s.executor.Exec(ctx, "tail", "-n", fmt.Sprintf("%d", lines), path)
	if err != nil {
		return "", err
	}
	return result.Output, nil
}
