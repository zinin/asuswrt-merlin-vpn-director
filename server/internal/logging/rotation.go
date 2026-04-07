package logging

import (
	"context"
	"log/slog"
	"os"
	"time"
)

// TruncateIfNeeded truncates file if it exceeds maxSize
func TruncateIfNeeded(path string, maxSize int64) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if info.Size() > maxSize {
		if err := os.Truncate(path, 0); err != nil {
			slog.Warn("Failed to truncate log file", "path", path, "error", err)
		} else {
			slog.Info("Truncated log file", "path", path, "prev_size", info.Size())
		}
	}
}

// StartRotation starts a goroutine that periodically checks and truncates log files
func (l *Logger) StartRotation(ctx context.Context, paths []string, maxSize int64, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				for _, path := range paths {
					TruncateIfNeeded(path, maxSize)
				}
			}
		}
	}()
}
