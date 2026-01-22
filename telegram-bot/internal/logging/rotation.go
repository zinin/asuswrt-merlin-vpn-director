package logging

import (
	"context"
	"log"
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
			log.Printf("[WARN] Failed to truncate %s: %v", path, err)
		} else {
			log.Printf("[INFO] Truncated log file %s (was %d bytes)", path, info.Size())
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
