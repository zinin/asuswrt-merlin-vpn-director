package updater

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"
)

// fakeInstaller returns a shell script to serve as a release asset. Run as the
// installer, it records its arguments and working directory under record,
// runs body and exits with code.
func fakeInstaller(record, body string, code int) string {
	return "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > '" + record + "/argv'\n" +
		"pwd > '" + record + "/cwd'\n" +
		body + "\n" +
		"exit " + strconv.Itoa(code) + "\n"
}

// newHandoverService serves assets (name -> body) over TLS as a release and
// returns a Service for the webui daemon whose update directory, lock and
// installer live in a temp dir. The lock is taken, as Start takes it, and
// files/ holds a marker, so a test can see what a cleanup removed.
func newHandoverService(t *testing.T, assets map[string]string) (*Service, *Release) {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := assets[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	dir := t.TempDir()
	s := &Service{
		httpClient: server.Client(),
		updateDir:  dir,
		lockFile:   filepath.Join(dir, "lock"),
		archSuffix: "arm64",
		daemon:     DaemonWebUI,
	}
	release := &Release{TagName: "v1.1.0"}
	for name := range assets {
		release.Assets = append(release.Assets, Asset{Name: name, DownloadURL: server.URL + "/" + name})
	}
	if err := s.CreateLock(); err != nil {
		t.Fatalf("CreateLock() error = %v", err)
	}
	if err := os.MkdirAll(s.getFilesDir(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.getFilesDir(), "marker"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	return s, release
}

// collect returns a progress func and the lines it received. Handover calls
// progress from the goroutine that copies the installer's output and returns
// only after that goroutine is done, so the lines are safe to read afterwards.
func collect() (func(string), *[]string) {
	var lines []string
	return func(l string) { lines = append(lines, l) }, &lines
}

func assertHandoverPhase(t *testing.T, err error, phase HandoverPhase) *HandoverError {
	t.Helper()
	var he *HandoverError
	if !errors.As(err, &he) {
		t.Fatalf("Handover() error = %v, want a *HandoverError", err)
	}
	if he.Phase != phase {
		t.Fatalf("Handover() failed in phase %d (%v), want phase %d", he.Phase, err, phase)
	}
	return he
}

func assertCleanedUp(t *testing.T, s *Service) {
	t.Helper()
	for what, path := range map[string]string{
		"files/":    s.getFilesDir(),
		"the lock":  s.getLockFile(),
		"installer": s.getInstallerFile(),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s survived a failed handover", what)
		}
	}
}

func TestHandover_RunsStep2OfTheNewVersionAndRelaysItsProgress(t *testing.T) {
	record := t.TempDir()
	s, release := newHandoverService(t, map[string]string{
		"webui-arm64":        fakeInstaller(record, "echo 'Files downloaded, starting update...'", 0),
		"telegram-bot-arm64": "#!/bin/sh\nexit 99\n",
	})
	progress, lines := collect()

	if err := s.Handover(context.Background(), release, validOpts(), progress); err != nil {
		t.Fatalf("Handover() error = %v", err)
	}

	argv, err := os.ReadFile(filepath.Join(record, "argv"))
	if err != nil {
		t.Fatalf("the webui's binary did not run: %v", err)
	}
	if got, want := strings.Join(strings.Fields(string(argv)), " "), strings.Join(selfUpdateArgv(validOpts()), " "); got != want {
		t.Errorf("installer argv = %q, want %q", got, want)
	}
	if cwd, _ := os.ReadFile(filepath.Join(record, "cwd")); strings.TrimSpace(string(cwd)) != "/" {
		t.Errorf("installer ran in %q, want /", strings.TrimSpace(string(cwd)))
	}
	if got := strings.Join(*lines, "\n"); got != "Files downloaded, starting update..." {
		t.Errorf("progress = %q, want the installer's line", got)
	}
	for _, leftover := range []string{s.getInstallerFile(), s.getInstallerFile() + ".part"} {
		if _, err := os.Stat(leftover); !os.IsNotExist(err) {
			t.Errorf("%s left behind", leftover)
		}
	}
	if !s.lockNamesPID(os.Getpid()) {
		t.Error("a handover that succeeded touched the lock the update script now owns")
	}
	if _, err := os.Stat(filepath.Join(s.getFilesDir(), "marker")); err != nil {
		t.Error("a handover that succeeded removed files/ from under the update script")
	}
}

func TestHandover_RunsAnotherDaemonsBinaryWhenItsOwnIsMissing(t *testing.T) {
	record := t.TempDir()
	s, release := newHandoverService(t, map[string]string{
		"telegram-bot-arm64": fakeInstaller(record, "", 0),
	})
	progress, _ := collect()

	if err := s.Handover(context.Background(), release, validOpts(), progress); err != nil {
		t.Fatalf("Handover() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(record, "argv")); err != nil {
		t.Error("the bot's binary did not run although the release lacks the webui's")
	}
}

func TestHandover_WithoutAnyDaemonBinaryIsADownloadFailure(t *testing.T) {
	s, release := newHandoverService(t, map[string]string{"xray-arm64": "x"})
	progress, lines := collect()

	err := s.Handover(context.Background(), release, validOpts(), progress)

	assertHandoverPhase(t, err, PhaseDownload)
	assertCleanedUp(t, s)
	if len(*lines) != 0 {
		t.Errorf("progress = %q from a step 2 that never ran", *lines)
	}
}

// The installer is executed as root, so the one scheme downgrade that would
// hand a network attacker that file is refused, as for the payload binaries.
func TestHandover_RefusesAPlainHTTPInstaller(t *testing.T) {
	s, release := newHandoverService(t, map[string]string{"webui-arm64": "x"})
	release.Assets[0].DownloadURL = strings.Replace(release.Assets[0].DownloadURL, "https://", "http://", 1)
	progress, _ := collect()

	err := s.Handover(context.Background(), release, validOpts(), progress)

	if he := assertHandoverPhase(t, err, PhaseDownload); !strings.Contains(he.Err.Error(), "https") {
		t.Errorf("error = %v, want it to name the scheme requirement", err)
	}
	assertCleanedUp(t, s)
}

func TestHandover_AnInstallerThatDoesNotExecuteIsAStartFailure(t *testing.T) {
	s, release := newHandoverService(t, map[string]string{"webui-arm64": "no shebang, no ELF header\n"})
	progress, _ := collect()

	err := s.Handover(context.Background(), release, validOpts(), progress)

	assertHandoverPhase(t, err, PhaseStart)
	assertCleanedUp(t, s)
}

func TestHandover_AFailingStep2IsReportedWithItsLastStderrLine(t *testing.T) {
	record := t.TempDir()
	s, release := newHandoverService(t, map[string]string{
		"webui-arm64": fakeInstaller(record, "echo 'first complaint' >&2\necho 'the real reason' >&2", 1),
	})
	progress, _ := collect()

	err := s.Handover(context.Background(), release, validOpts(), progress)

	if he := assertHandoverPhase(t, err, PhaseInstaller); he.Err.Error() != "the real reason" {
		t.Errorf("reason = %q, want the last stderr line", he.Err)
	}
	assertCleanedUp(t, s)
}

func TestHandover_KillsAStep2ThatRunsOutOfTime(t *testing.T) {
	record := t.TempDir()
	s, release := newHandoverService(t, map[string]string{
		"webui-arm64": fakeInstaller(record, "exec sleep 30", 0),
	})
	s.handoverTimeout = 200 * time.Millisecond
	progress, _ := collect()

	start := time.Now()
	err := s.Handover(context.Background(), release, validOpts(), progress)

	assertHandoverPhase(t, err, PhaseTimeout)
	if waited := time.Since(start); waited > 10*time.Second {
		t.Errorf("Handover() took %v to give up on a killed step 2", waited)
	}
	assertCleanedUp(t, s)
}

// Step 1 asks before it kills, and a step 2 that takes the offer decides the
// outcome with its exit status as any other would. That matters for a step 2
// no release has shipped yet: one that has already started the update script
// can catch the signal and exit 0 instead of being killed with the script
// running and having files/ and the lock removed from under it. A POSIX shell
// runs a trap only once the foreground command returns, so the fake waits on a
// background sleep - whose output goes nowhere, or it would hold step 2's pipes
// open past its exit and cost this test installerWaitDelay. The trailing exit 1
// is what a fake that was killed outright, or never signalled, would report.
func TestHandover_AsksATimedOutStep2ToStopBeforeKillingIt(t *testing.T) {
	record := t.TempDir()
	held := filepath.Join(record, "held")
	s, release := newHandoverService(t, map[string]string{
		"webui-arm64": fakeInstaller(record,
			"trap 'exit 0' TERM\nsleep 30 >/dev/null 2>&1 &\necho $! > '"+held+"'\nwait", 1),
	})
	s.handoverTimeout = 200 * time.Millisecond
	t.Cleanup(func() {
		data, _ := os.ReadFile(held)
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
			syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	progress, _ := collect()

	start := time.Now()
	if err := s.Handover(context.Background(), release, validOpts(), progress); err != nil {
		t.Fatalf("Handover() error = %v, want the exit 0 of a step 2 that stopped when asked", err)
	}
	if waited := time.Since(start); waited >= installerWaitDelay {
		t.Errorf("Handover() took %v: step 2 was killed rather than asked to stop", waited)
	}
	if !s.lockNamesPID(os.Getpid()) {
		t.Error("a handover whose step 2 exited 0 touched the lock the update script now owns")
	}
	if _, err := os.Stat(filepath.Join(s.getFilesDir(), "marker")); err != nil {
		t.Error("a handover whose step 2 exited 0 removed files/ from under the update script")
	}
}

// Exit 0 decides whatever else Wait reports. Here something step 2 started
// keeps its output open, and Wait reports ErrWaitDelay once installerWaitDelay
// has passed. The other such case, the deadline firing between step 2's exit
// and its reap, cannot be timed from a test.
func TestHandover_AStep2ThatExitsZeroSucceedsWhileItsOutputStaysOpen(t *testing.T) {
	record := t.TempDir()
	held := filepath.Join(record, "held")
	s, release := newHandoverService(t, map[string]string{
		"webui-arm64": fakeInstaller(record, "sleep 10 &\necho $! > '"+held+"'", 0),
	})
	t.Cleanup(func() {
		data, _ := os.ReadFile(held)
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
			syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	progress, _ := collect()

	start := time.Now()
	if err := s.Handover(context.Background(), release, validOpts(), progress); err != nil {
		t.Fatalf("Handover() error = %v, want the exit status to decide", err)
	}
	if waited := time.Since(start); waited < installerWaitDelay {
		t.Fatalf("Handover() returned after %v, before Wait gave up on the open output", waited)
	}
	if !s.lockNamesPID(os.Getpid()) {
		t.Error("a handover whose step 2 exited 0 touched the lock the update script now owns")
	}
	if _, err := os.Stat(filepath.Join(s.getFilesDir(), "marker")); err != nil {
		t.Error("a handover whose step 2 exited 0 removed files/ from under the update script")
	}
}

// Once the update script has republished the lock, the directory is the
// script's even if step 2 then exits non-zero: removing files/ would pull the
// payload from under the copy step.
func TestHandover_LeavesTheDirectoryToAScriptThatTookTheLock(t *testing.T) {
	record := t.TempDir()
	s, release := newHandoverService(t, map[string]string{
		"webui-arm64": fakeInstaller(record, `printf '1\n' > "$FAKE_LOCK"`, 1),
	})
	t.Setenv("FAKE_LOCK", s.getLockFile())
	progress, _ := collect()

	err := s.Handover(context.Background(), release, validOpts(), progress)

	assertHandoverPhase(t, err, PhaseInstaller)
	if _, err := os.Stat(filepath.Join(s.getFilesDir(), "marker")); err != nil {
		t.Error("files/ was removed although the lock names the update script")
	}
	if data, _ := os.ReadFile(s.getLockFile()); strings.TrimSpace(string(data)) != "1" {
		t.Errorf("lock = %q, want the script's claim left in place", data)
	}
	// The installer is step 1's own file wherever the lock has gone, and the
	// contract has it removed whatever the outcome.
	if _, err := os.Stat(s.getInstallerFile()); !os.IsNotExist(err) {
		t.Error("the installer was left behind in a directory the update script now owns")
	}
}

func TestHandover_CapsWhatStep2CanPutInFrontOfTheUser(t *testing.T) {
	record := t.TempDir()
	body := "echo '" + strings.Repeat("я", 400) + "'\n" +
		"i=0; while [ $i -lt 14 ]; do echo \"line $i\"; i=$((i+1)); done"
	s, release := newHandoverService(t, map[string]string{"webui-arm64": fakeInstaller(record, body, 0)})
	progress, lines := collect()

	if err := s.Handover(context.Background(), release, validOpts(), progress); err != nil {
		t.Fatalf("Handover() error = %v", err)
	}

	if len(*lines) != maxProgressLines {
		t.Fatalf("progress got %d lines, want %d", len(*lines), maxProgressLines)
	}
	if first := (*lines)[0]; first != strings.Repeat("я", maxProgressRunes) {
		t.Errorf("first line has %d runes, want it cut to %d", utf8.RuneCountInString(first), maxProgressRunes)
	}
	if last := (*lines)[maxProgressLines-1]; last != "line 8" {
		t.Errorf("last forwarded line = %q, want line 8", last)
	}
}

// A process forked elsewhere in the daemon while the installer was still open
// for writing keeps it open until that process execs, and an exec of the
// installer fails with "text file busy" meanwhile (golang/go#22315). Holding a
// write descriptor here reproduces that on kernels that refuse the exec; on
// the others the first attempt already succeeds.
func TestStartInstaller_RetriesATextFileBusyExec(t *testing.T) {
	installer := filepath.Join(t.TempDir(), "installer")
	if err := os.WriteFile(installer, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	w, err := os.OpenFile(installer, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		w.Close()
	}()

	discard := func(string) {}
	cmd, err := startInstaller(context.Background(), installer, nil, &lineWriter{line: discard}, &lineWriter{line: discard})
	if err != nil {
		t.Fatalf("startInstaller() error = %v, want it to retry past the busy text file", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Errorf("installer exit = %v", err)
	}
}

// exitedState is the ProcessState of a process that chose its own exit code,
// as a step 2 with something to say does.
func exitedState(t *testing.T, code int) *os.ProcessState {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", "exit "+strconv.Itoa(code))
	cmd.Run()
	if cmd.ProcessState == nil {
		t.Fatal("/bin/sh did not run")
	}
	return cmd.ProcessState
}

// signalledState is the ProcessState of a process that was killed before it
// could choose an exit code, as a step 2 that ignores the SIGTERM is.
func signalledState(t *testing.T) *os.ProcessState {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", "sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	if cmd.ProcessState == nil {
		t.Fatal("no process state for a killed process")
	}
	return cmd.ProcessState
}

// The outcome is step 2's exit status read together with the context: a
// timeout is only what the deadline took away from step 2 by force. A step 2
// that picked its own exit code has a reason for the user, and throwing that
// away for "Update timed out" throws away the one thing they can act on.
func TestHandoverOutcome_ReportsATimeoutOnlyForADeadlineThatKilledStep2(t *testing.T) {
	exited0 := exitedState(t, 0)
	exited3 := exitedState(t, 3)
	killed := signalledState(t)
	waitErr := errors.New("exit status 3")

	tests := []struct {
		name string
		// state, ctxErr, waitErr and reason are what runInstaller holds after
		// Wait; wantPhase 0 means the handover succeeded.
		state      *os.ProcessState
		ctxErr     error
		waitErr    error
		reason     string
		wantPhase  HandoverPhase
		wantReason string
	}{
		{name: "exit 0 is the update script started", state: exited0},
		{
			name: "exit 0 counts even past the deadline", state: exited0,
			ctxErr: context.DeadlineExceeded, waitErr: context.DeadlineExceeded,
		},
		{
			name: "the deadline killed step 2", state: killed,
			ctxErr: context.DeadlineExceeded, waitErr: errors.New("signal: killed"),
			wantPhase: PhaseTimeout, wantReason: context.DeadlineExceeded.Error(),
		},
		{
			name: "the deadline killed a step 2 that left no state", state: nil,
			ctxErr: context.DeadlineExceeded, waitErr: errors.New("signal: killed"),
			wantPhase: PhaseTimeout, wantReason: context.DeadlineExceeded.Error(),
		},
		{
			name: "step 2 answered the deadline with an exit code of its own", state: exited3,
			ctxErr: context.DeadlineExceeded, waitErr: waitErr, reason: "no space left on the router",
			wantPhase: PhaseInstaller, wantReason: "no space left on the router",
		},
		{
			name: "step 2 failed with time to spare", state: exited3,
			waitErr: waitErr, reason: "no space left on the router",
			wantPhase: PhaseInstaller, wantReason: "no space left on the router",
		},
		{
			name: "a cancelled context is not a timeout", state: killed,
			ctxErr: context.Canceled, waitErr: errors.New("signal: killed"), reason: "stopping",
			wantPhase: PhaseInstaller, wantReason: "stopping",
		},
		{
			name: "a step 2 that said nothing is reported with the wait error", state: exited3,
			waitErr: waitErr, wantPhase: PhaseInstaller, wantReason: waitErr.Error(),
		},
		{
			// Nothing to show at all would reach the user as "Update failed:"
			// with an empty space after it, which says less than the least
			// this code can say.
			name: "a step 2 that left nothing to report at all", state: exited3,
			wantPhase: PhaseInstaller, wantReason: "no reason given",
		},
		{
			name: "a long reason is cut to what a message can hold", state: exited3,
			waitErr: waitErr, reason: strings.Repeat("я", maxProgressRunes+100),
			wantPhase: PhaseInstaller, wantReason: strings.Repeat("я", maxProgressRunes),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handoverOutcome(tt.state, tt.ctxErr, tt.waitErr, tt.reason)
			if tt.wantPhase == 0 {
				if err != nil {
					t.Fatalf("handoverOutcome() = %v, want the handover to have succeeded", err)
				}
				return
			}
			he := assertHandoverPhase(t, err, tt.wantPhase)
			if he.Err.Error() != tt.wantReason {
				t.Errorf("reason = %q, want %q", he.Err, tt.wantReason)
			}
		})
	}
}

// A step 2 that catches the SIGTERM the deadline sends, says why it is giving
// up and picks its own exit code is a failure with a reason, not a timeout.
func TestHandover_ReportsTheReasonOfAStep2ThatAnsweredTheDeadline(t *testing.T) {
	record := t.TempDir()
	held := filepath.Join(record, "held")
	s, release := newHandoverService(t, map[string]string{
		"webui-arm64": fakeInstaller(record,
			"trap \"echo 'no space left on the router' >&2; exit 3\" TERM\n"+
				"sleep 30 >/dev/null 2>&1 &\necho $! > '"+held+"'\nwait", 1),
	})
	s.handoverTimeout = 200 * time.Millisecond
	t.Cleanup(func() {
		data, _ := os.ReadFile(held)
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
			syscall.Kill(pid, syscall.SIGKILL)
		}
	})
	progress, _ := collect()

	err := s.Handover(context.Background(), release, validOpts(), progress)

	if he := assertHandoverPhase(t, err, PhaseInstaller); he.Err.Error() != "no space left on the router" {
		t.Errorf("reason = %q, want the reason step 2 gave", he.Err)
	}
	assertCleanedUp(t, s)
}

// A line that outgrows maxPendingLine is handed over in pieces. A piece cut at
// a byte offset can end inside a rune, and what reaches the user is then
// invalid UTF-8, which Telegram refuses to deliver at all.
func TestLineWriter_SplitsAnOverlongLineAtARuneBoundary(t *testing.T) {
	var got []string
	w := &lineWriter{line: func(l string) { got = append(got, l) }}
	// Three bytes a rune, half as long again as the limit.
	line := strings.Repeat("中", maxPendingLine/2)

	// Four bytes at a time never aligns with a three-byte rune, so the limit
	// is crossed with two bytes of one written and the third still to come.
	for b := []byte(line); len(b) > 0; {
		n := min(4, len(b))
		if _, err := w.Write(b[:n]); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		b = b[n:]
	}
	w.flush()

	if len(got) < 2 {
		t.Fatalf("the line came out in %d pieces, want it split at the limit", len(got))
	}
	for i, piece := range got {
		if !utf8.ValidString(piece) {
			t.Errorf("piece %d of %d ends inside a rune", i+1, len(got))
		}
	}
	if strings.Join(got, "") != line {
		t.Error("the pieces do not add up to the line that was written")
	}
}

// The installer is part of what a failed handover owns, so it goes while the
// lock still names this process - a removal after the claim is dropped reaches
// into a directory the next attempt may already be filling.
func TestCleanUpOwned_RemovesTheInstallerUnderTheClaim(t *testing.T) {
	s, _ := newHandoverService(t, map[string]string{})
	if err := os.WriteFile(s.getInstallerFile(), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	s.cleanUpOwned()

	assertCleanedUp(t, s)
}

// The claim gates the whole cleanup, the installer included. Handover's own
// deferred removal is what still takes the installer away in this case, so
// the contract's promise that it goes whatever the outcome is kept.
func TestCleanUpOwned_TouchesNothingOnceTheLockNamesAnotherProcess(t *testing.T) {
	s, _ := newHandoverService(t, map[string]string{})
	if err := os.WriteFile(s.getInstallerFile(), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.getLockFile(), []byte("1"), 0644); err != nil {
		t.Fatal(err)
	}

	s.cleanUpOwned()

	if _, err := os.Stat(s.getInstallerFile()); err != nil {
		t.Errorf("the installer was removed although the lock names another process: %v", err)
	}
}

// syncBuffer collects log records: os/exec's output copiers write them from
// their own goroutines, and WaitDelay lets one outlive the Wait that reaps
// step 2.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// The caps keep a faulty step 2 from flooding a chat, but what they hold back
// still has to reach whoever reads the daemon's log: the progress past the
// limit, and everything step 2 said on stderr, which no user ever sees.
func TestHandover_LogsWhatItKeepsFromTheUser(t *testing.T) {
	var logged syncBuffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	const surplus = 3
	record := t.TempDir()
	body := "i=0; while [ $i -lt " + strconv.Itoa(maxProgressLines+surplus) + " ]; do echo \"line $i\"; i=$((i+1)); done\n" +
		"echo 'no space left on the router' >&2"
	s, release := newHandoverService(t, map[string]string{"webui-arm64": fakeInstaller(record, body, 0)})
	progress, lines := collect()

	if err := s.Handover(context.Background(), release, validOpts(), progress); err != nil {
		t.Fatalf("Handover() error = %v", err)
	}

	if len(*lines) != maxProgressLines {
		t.Fatalf("progress got %d lines, want %d", len(*lines), maxProgressLines)
	}
	log := logged.String()
	for i := maxProgressLines; i < maxProgressLines+surplus; i++ {
		if line := "line " + strconv.Itoa(i); !strings.Contains(log, line) {
			t.Errorf("the log lost %q, a progress line the user never got", line)
		}
	}
	if !strings.Contains(log, "no space left on the router") {
		t.Error("the log lost what step 2 wrote on stderr, which nothing else records")
	}
	if last := (*lines)[maxProgressLines-1]; strings.Contains(log, last) {
		t.Errorf("the log repeats %q, a line the user already has", last)
	}
}
