package webapi

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/service"
)

// Response deadlines for handlers that run shell commands. Each is the
// command's own timeout plus deadlineSlack, so the handler can still write
// its response after the service layer has cut the command off. The
// server-wide WriteTimeout (30 s, server.go) stays in force for every other
// route.
const (
	deadlineSlack  = 30 * time.Second
	applyDeadline  = service.ApplyTimeout + deadlineSlack
	updateDeadline = service.UpdateTimeout + deadlineSlack
	// importDeadline covers the 10-second subscription download plus one DNS
	// lookup per server; the import runs no shell command.
	importDeadline = 2 * time.Minute
)

// extendWriteDeadline pushes the response deadline past the server-wide
// WriteTimeout for handlers that run long shell commands. Writers that do
// not support deadlines (httptest.ResponseRecorder) are ignored; any other
// failure is logged, because a silently lost deadline reproduces the torn
// connection this helper exists to prevent.
func extendWriteDeadline(w http.ResponseWriter, d time.Duration) {
	err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(d))
	if err != nil && !errors.Is(err, http.ErrNotSupported) {
		slog.Warn("failed to extend write deadline", "error", err)
	}
}

// lockLongOp serializes a shell-running handler on deps.OpMutex and extends
// the write deadline both before and after the wait, so the deadline covers
// the command itself and not the time spent queued behind another one. The
// returned func releases the mutex.
func lockLongOp(w http.ResponseWriter, deps *Deps, d time.Duration) func() {
	extendWriteDeadline(w, d)
	deps.OpMutex.Lock()
	extendWriteDeadline(w, d)
	return deps.OpMutex.Unlock
}
