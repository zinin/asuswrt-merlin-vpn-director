// internal/service/logs.go
package service

import "fmt"

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
	result, err := s.executor.Exec("tail", "-n", fmt.Sprintf("%d", lines), path)
	if err != nil {
		return "", err
	}
	return result.Output, nil
}
