package updater

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// validOpts returns options that pass every validation, so a test can flip a
// single field and assert that this one field is what got rejected.
func validOpts() RunOptions {
	return RunOptions{OldVersion: "v1.0.0", NewVersion: "v1.1.0", ChatID: 123, Initiator: "bot"}
}

// noExecService returns a Service whose interpreter path does not exist, so
// RunUpdateScript writes the script and then fails at exec. Without the seam
// these tests launch the real update script - pgrep and pkill -9 over the
// whole process table, cp over /opt - on the machine running go test.
func noExecService(t *testing.T) *Service {
	t.Helper()

	dir := t.TempDir()
	return &Service{
		updateDir:  dir,
		scriptFile: filepath.Join(dir, "update.sh"),
		shell:      filepath.Join(dir, "no-such-interpreter"),
	}
}

// assertExecRefused checks that RunUpdateScript got as far as launching the
// script and no further. A nil error would mean a real shell took the
// generated script and ran it.
func assertExecRefused(t *testing.T, s *Service, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("RunUpdateScript() succeeded, so a real shell just ran the update script")
	}
	if !strings.Contains(err.Error(), s.shell) {
		t.Fatalf("RunUpdateScript() error = %v, want the exec of %s to fail", err, s.shell)
	}
}

func TestService_ShellDefaultsToBinSh(t *testing.T) {
	// The seam must not change what ships: a router has /bin/sh and nothing
	// else is guaranteed.
	if got := (&Service{}).getShell(); got != "/bin/sh" {
		t.Errorf("getShell() = %q, want /bin/sh", got)
	}
	if got := (&Service{shell: "/bin/busybox"}).getShell(); got != "/bin/busybox" {
		t.Errorf("getShell() = %q, want the injected interpreter", got)
	}
}

func TestGenerateScript(t *testing.T) {
	tmpDir := t.TempDir()

	s := &Service{
		updateDir: tmpDir,
	}

	script, err := s.generateScript(RunOptions{ChatID: 123456789, OldVersion: "v1.0.0", NewVersion: "v1.1.0", Initiator: "bot"})
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	// Check that variables are embedded correctly
	checks := []string{
		"CHAT_ID=123456789",
		`OLD_VERSION="v1.0.0"`,
		`NEW_VERSION="v1.1.0"`,
		"set -e",
		`pgrep -f "$bin"`, // full binary path, from the daemon table
		"telegram-bot|/opt/vpn-director/telegram-bot|S98telegram-bot",
		"webui|/opt/vpn-director/webui|S98vpn-director-webui",
	}

	for _, check := range checks {
		if !strings.Contains(script, check) {
			t.Errorf("Script missing %q", check)
		}
	}

	// monit commands stay optional
	if !strings.Contains(script, `monit unmonitor "${entry%%|*}" 2>/dev/null || true`) {
		t.Error("Script missing || true for monit unmonitor")
	}
	if !strings.Contains(script, `monit monitor "$name" 2>/dev/null || true`) {
		t.Error("Script missing || true for monit monitor")
	}
	if !strings.Contains(script, "remonitor_running") {
		t.Error("script must remonitor only daemons that were running")
	}
	if !strings.Contains(script, "flock -n 9") {
		t.Error("script must wait for vpn-director.sh before rewriting it")
	}

	own := strings.Index(script, `mv -f "$LOCK_FILE.new" "$LOCK_FILE"`)
	stop := strings.Index(script, "# 3. Stop the running daemons")
	if own < 0 {
		t.Error("script must take lock ownership before it stops the initiator")
	} else if stop < 0 || own > stop {
		t.Error("lock ownership must be taken before the stop loop")
	}
	if !strings.Contains(script, `rm -rf "$FILES_DIR"`) {
		t.Error("script must drop files/ after the copy so a Web UI-only router does not keep binaries in tmpfs")
	}
	okAt := strings.Index(script, "write_notify ok")
	exceptAt := strings.Index(script, `start_except "$NOTIFY_INIT"`)
	if okAt < 0 || exceptAt < 0 || exceptAt > okAt {
		t.Error("write_notify ok must come after the non-bot daemons have started, or the bot can announce success while Web UI is down")
	}

	// Check that cp commands do NOT have || true (critical commands)
	for _, line := range strings.Split(script, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "cp -f") {
			if strings.HasSuffix(trimmed, "|| true") {
				t.Errorf("cp command should not have || true: %s", line)
			}
		}
	}
}

func TestGenerateScript_PathsCorrect(t *testing.T) {
	tmpDir := t.TempDir()

	s := &Service{
		updateDir: tmpDir,
	}

	script, err := s.generateScript(RunOptions{ChatID: 999, OldVersion: "v1.0.0", NewVersion: "v2.0.0", Initiator: "bot"})
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	// Verify paths use the custom updateDir
	expectedPaths := []string{
		`UPDATE_DIR="` + tmpDir + `"`,
		`FILES_DIR="` + tmpDir + `/files"`,
		`NOTIFY_FILE="` + tmpDir + `/notify.json"`,
		`LOCK_FILE="` + tmpDir + `/lock"`,
	}

	for _, expected := range expectedPaths {
		if !strings.Contains(script, expected) {
			t.Errorf("Script missing path %q", expected)
		}
	}
}

func TestGenerateScript_EmbedsInitiator(t *testing.T) {
	tmpDir := t.TempDir()
	s := &Service{updateDir: tmpDir}

	script, err := s.generateScript(RunOptions{
		OldVersion: "v1.0.0", NewVersion: "v1.1.0", ChatID: 0, Initiator: "webui",
	})
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}
	if !strings.Contains(script, `INITIATOR="webui"`) {
		t.Error("script must carry the initiator so notify.json can name it")
	}
	if !strings.Contains(script, "CHAT_ID=0") {
		t.Error("a Web UI update has chat_id 0")
	}
}

func TestGenerateScript_CoversEveryDaemon(t *testing.T) {
	s := &Service{}
	script, err := s.generateScript(validOpts())
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	for _, d := range Daemons {
		for _, want := range []string{d.Name, d.Binary, d.InitScript} {
			if !strings.Contains(script, want) {
				t.Errorf("script missing %q for daemon %s", want, d.Name)
			}
		}
	}

	// The daemon table drives every loop, so it is the only place an init
	// script may be named. A second mention is a hardcoded call, and a
	// hardcoded call silently skips the other daemon.
	for _, d := range Daemons {
		if n := strings.Count(script, d.InitScript); n != 1 {
			t.Errorf("init script %s named %d times, want exactly 1 (only in the DAEMONS table)", d.InitScript, n)
		}
	}
}

func TestGenerateScript_RecoveryOnFailure(t *testing.T) {
	s := &Service{}
	script, err := s.generateScript(validOpts())
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	if !strings.Contains(script, "trap on_exit EXIT") {
		t.Error("script must install the EXIT trap: ash has no ERR trap")
	}

	// Every other needle has to sit inside the handler. Searching the whole
	// script would pass on the happy-path copy of the same line, so removing
	// the step from the recovery path would go unnoticed.
	body := onExitBody(t, script)
	checks := map[string]string{
		"code=$?":             "the EXIT trap must inspect the exit code",
		"set +e":              "recovery must survive its own failing steps",
		"write_notify failed": "a failed update must leave status failed in notify.json",
		"start_running":       "recovery must restart the daemons that were running",
		"remonitor_running":   "recovery must remonitor only daemons that were running",
		`rm -f "$LOCK_FILE"`:  "a failed update must release the lock",
	}
	for needle, why := range checks {
		if !strings.Contains(body, needle) {
			t.Errorf("on_exit missing %q: %s", needle, why)
		}
	}
}

// The lock taken in step 3b lives on the open file description, and a daemon
// started while fd 9 is still open inherits it: /var/lock/vpn-director.lock
// would stay held for as long as that daemon runs, turning every later
// vpn-director.sh into a silent skip (no --wait) or a lock timeout (--wait).
// Both the happy path and the recovery path must release before they start
// anything.
func TestGenerateScript_ReleasesApplyLockBeforeStartingDaemons(t *testing.T) {
	s := &Service{}
	script, err := s.generateScript(validOpts())
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	if body := functionBody(t, script, "release_apply_lock"); !strings.Contains(body, "exec 9>&-") {
		t.Error("release_apply_lock must close the descriptor, not just drop the lock")
	}

	paths := []struct {
		name  string
		body  string
		start string
	}{
		{"happy path", afterOnExit(t, script), "start_except"},
		{"recovery", onExitBody(t, script), "start_running"},
	}
	for _, p := range paths {
		release := strings.Index(p.body, "release_apply_lock")
		start := strings.Index(p.body, p.start)
		switch {
		case release < 0:
			t.Errorf("%s never releases the apply lock", p.name)
		case start < 0:
			t.Errorf("%s never starts a daemon through %s", p.name, p.start)
		case release > start:
			t.Errorf("%s releases the apply lock after %s, so the started daemon inherits fd 9", p.name, p.start)
		}
	}
}

// POSIX allows exactly one digit in a redirection; a multi-digit descriptor is
// a bash/ksh extension. getShell() runs the generated script with /bin/sh, and
// dash rejects "exec 201>" outright.
func TestGenerateScript_UsesPOSIXFileDescriptors(t *testing.T) {
	s := &Service{}
	script, err := s.generateScript(validOpts())
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	multiDigit := regexp.MustCompile(`exec\s+[0-9]{2,}[<>]|flock\s+(?:-[a-z]+\s+)*[0-9]{2,}\b`)
	if m := multiDigit.FindString(script); m != "" {
		t.Errorf("script uses the multi-digit file descriptor %q; /bin/sh may be dash, which rejects it", m)
	}
}

// The bot reads notify.json once, on startup. Restarted before the file
// exists it finds nothing, and a running process never looks again, so the
// failure would stay unreported until the next restart.
func TestGenerateScript_CommitsTheFailedStatusBeforeRestarting(t *testing.T) {
	s := &Service{}
	script, err := s.generateScript(validOpts())
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	body := onExitBody(t, script)
	notify := strings.Index(body, "write_notify failed")
	start := strings.Index(body, "start_running")
	switch {
	case notify < 0:
		t.Fatal("recovery never writes the failed status")
	case start < 0:
		t.Fatal("recovery never restarts the daemons")
	case notify > start:
		t.Error("recovery restarts the daemons before it writes notify.json")
	}
}

// monit starts a stopped daemon on its own check interval. Handing the
// daemons back before the status is committed lets it put the bot up without
// a notify.json to read, and CheckAndSendNotify runs at startup only.
func TestGenerateScript_RestoresMonitLast(t *testing.T) {
	s := &Service{}
	script, err := s.generateScript(validOpts())
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	paths := []struct {
		name   string
		body   string
		starts string
	}{
		{"happy path", afterOnExit(t, script), "start_except"},
		{"recovery", onExitBody(t, script), "start_running"},
	}
	for _, p := range paths {
		remonitor := strings.Index(p.body, "remonitor_running")
		if remonitor < 0 {
			t.Errorf("%s never hands the daemons back to monit", p.name)
			continue
		}
		if start := strings.Index(p.body, p.starts); start < 0 || remonitor < start {
			t.Errorf("%s restores monit before it starts the daemons itself", p.name)
		}
		if notify := strings.Index(p.body, "write_notify"); notify < 0 || remonitor < notify {
			t.Errorf("%s restores monit before it commits the status", p.name)
		}
	}
}

// A reader that catches the lock truncated but not yet written parses no PID,
// deletes the lock as stale, and the running update ends up unlocked - free
// for a second update to start and wipe the shared files/ directory.
func TestGenerateScript_PublishesThePIDByRename(t *testing.T) {
	s := &Service{}
	script, err := s.generateScript(validOpts())
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	if strings.Contains(script, `echo $$ > "$LOCK_FILE"`) {
		t.Error("the PID is written straight into the lock file, truncating it first")
	}
	if !strings.Contains(script, `mv -f "$LOCK_FILE.new" "$LOCK_FILE"`) {
		t.Error("the script must publish the new PID with a rename")
	}
}

// BusyBox sh on Asuswrt-Merlin has no "command" builtin: `command -v pgrep`
// exits 127 whether or not pgrep is installed. A guard written that way refuses
// every update on the very router it was meant to protect, and a monit gate
// written that way silently never unmonitors anything. Nothing in the generated
// script may depend on it.
func TestGenerateScript_DoesNotUseTheCommandBuiltin(t *testing.T) {
	s := &Service{}
	script, err := s.generateScript(validOpts())
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	// Comments are free to name the trap they explain; only what the shell
	// executes is pinned.
	for i, line := range strings.Split(script, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if strings.Contains(line, "command -v") {
			t.Errorf("line %d probes with `command -v`, which BusyBox sh does not have; use have_cmd: %s", i+1, line)
		}
	}
	if !strings.Contains(script, "have_cmd() {") {
		t.Error("script must define have_cmd to probe for a command")
	}
	for _, cmd := range []string{"pgrep", "monit"} {
		if !strings.Contains(script, "have_cmd "+cmd) {
			t.Errorf("script must probe for %s through have_cmd", cmd)
		}
	}
}

func TestGenerateScript_RefusesWithoutPgrep(t *testing.T) {
	s := &Service{}
	script, err := s.generateScript(validOpts())
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	// pgrep decides both what gets stopped and what gets started again. When
	// it is missing its 127 reads as "this daemon is not running" inside
	// every `if` below, so nothing is stopped, cp -f lands under live
	// processes and notify.json still says ok.
	steps := afterOnExit(t, script)
	const guard = "if ! have_cmd pgrep; then"
	i := strings.Index(steps, guard)
	if i < 0 {
		t.Fatal("script must refuse to run without pgrep, not read its absence as \"nothing is running\"")
	}
	if j := strings.Index(steps, `pgrep -f "$bin"`); j >= 0 && j < i {
		t.Error("the pgrep guard must come before the first pgrep use")
	}

	block := steps[i:]
	if k := strings.Index(block, "\nfi\n"); k >= 0 {
		block = block[:k]
	}
	if !strings.Contains(block, "exit 1") {
		t.Error("the pgrep guard must abort the update, not just log and carry on")
	}
}

func TestGenerateScript_ReportsAFailedStart(t *testing.T) {
	s := &Service{}
	script, err := s.generateScript(validOpts())
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	// Non-bot daemons start before notify.json is committed as ok, so a
	// failed Web UI start can still rewrite it. Both needles have a copy
	// inside the trap, so this looks below it only.
	steps := afterOnExit(t, script)
	if !strings.Contains(steps, `if ! start_except "$NOTIFY_INIT"; then`) {
		t.Error("the happy path must notice a failed start instead of reporting the update complete")
	}
	if !strings.Contains(steps, "write_notify failed") {
		t.Error("a daemon that fails to come back must turn notify.json to failed")
	}

	body := functionBody(t, script, "start_except")
	if strings.Contains(body, "start || log") {
		t.Error("start_running must not swallow a failed start behind || log")
	}
	if !strings.Contains(body, "return") {
		t.Error("start_running must return a status its caller can branch on")
	}
}

// functionBody returns the body of a shell function, so an assertion lands on
// the path that function is on and not on the script mentioning the words
// somewhere.
func functionBody(t *testing.T, script, name string) string {
	t.Helper()

	open := name + "() {\n"
	i := strings.Index(script, open)
	if i < 0 {
		t.Fatalf("script has no %s function", name)
	}
	body := script[i+len(open):]
	j := strings.Index(body, "\n}\n")
	if j < 0 {
		t.Fatalf("%s is never closed", name)
	}
	return body[:j]
}

// onExitBody returns the body of the on_exit handler, so a recovery
// assertion is about the recovery path and not about the script mentioning
// the words somewhere.
func onExitBody(t *testing.T, script string) string {
	t.Helper()

	return functionBody(t, script, "on_exit")
}

// afterOnExit returns everything below the EXIT trap: the numbered steps the
// script walks on the happy path. Asserting there keeps a happy-path claim
// from being satisfied by the recovery copy of the same line.
func afterOnExit(t *testing.T, script string) string {
	t.Helper()

	const marker = "\ntrap on_exit EXIT\n"
	i := strings.Index(script, marker)
	if i < 0 {
		t.Fatal("script does not install on_exit as its EXIT trap")
	}
	return script[i+len(marker):]
}

func TestGenerateScript_NotifyFormat(t *testing.T) {
	s := &Service{}
	script, err := s.generateScript(RunOptions{
		OldVersion: "v1.2.0", NewVersion: "v1.3.0", ChatID: 0, Initiator: "webui",
	})
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	const want = `{"chat_id":$CHAT_ID,"old_version":"$OLD_VERSION","new_version":"$NEW_VERSION","status":"$1","initiator":"$INITIATOR"}`
	if !strings.Contains(script, want) {
		t.Errorf("notify.json template line missing or changed, want %s", want)
	}
}

// Note: this test is only as strict as the developer's /bin/sh. On a box where
// /bin/sh is dash (a good ash proxy) it catches bashisms; where /bin/sh is bash
// it does not. The golden test is the real contract; this one is a cheap extra.
func TestGenerateScript_IsValidShell(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}

	s := &Service{}
	script, err := s.generateScript(validOpts())
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	path := filepath.Join(t.TempDir(), "update.sh")
	if err := os.WriteFile(path, []byte(script), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(sh, "-n", path).CombinedOutput()
	if err != nil {
		t.Fatalf("generated script has a syntax error: %v\n%s", err, out)
	}
}

func TestGenerateScript_Golden(t *testing.T) {
	// Default paths keep the render deterministic, so the golden file shows
	// the exact script that ships to routers. Regenerate with:
	//   UPDATE_GOLDEN=1 go test ./internal/updater -run TestGenerateScript_Golden -count=1
	s := &Service{}
	script, err := s.generateScript(RunOptions{
		OldVersion: "v1.2.0", NewVersion: "v1.3.0", ChatID: 42, Initiator: "bot",
	})
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	golden := filepath.Join("testdata", "update_script.golden.sh")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(script), 0644); err != nil {
			t.Fatal(err)
		}
		t.Logf("rewrote %s", golden)
		return
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden: %v (regenerate with UPDATE_GOLDEN=1)", err)
	}
	if string(want) != script {
		t.Errorf("generated script differs from %s; review the diff and regenerate with UPDATE_GOLDEN=1 if the change is intended", golden)
	}
}

func TestRunUpdateScript_InvalidVersion(t *testing.T) {
	s := noExecService(t)

	tests := []struct {
		name    string
		mutate  func(*RunOptions)
		wantErr string
	}{
		{
			name:    "shell injection in old version",
			mutate:  func(o *RunOptions) { o.OldVersion = "v1.0.0;rm -rf /" },
			wantErr: "invalid old version",
		},
		{
			name:    "shell injection in new version",
			mutate:  func(o *RunOptions) { o.NewVersion = "v1.1.0$(whoami)" },
			wantErr: "invalid new version",
		},
		{
			name:    "backticks in old version",
			mutate:  func(o *RunOptions) { o.OldVersion = "`id`" },
			wantErr: "invalid old version",
		},
		{
			name:    "quotes in new version",
			mutate:  func(o *RunOptions) { o.NewVersion = `v1.1.0"test` },
			wantErr: "invalid new version",
		},
		{
			name:    "empty old version",
			mutate:  func(o *RunOptions) { o.OldVersion = "" },
			wantErr: "invalid old version",
		},
		{
			name:    "too long version",
			mutate:  func(o *RunOptions) { o.NewVersion = strings.Repeat("v", 100) },
			wantErr: "invalid new version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := validOpts()
			tt.mutate(&opts)

			err := s.RunUpdateScript(opts)
			if err == nil {
				t.Error("Expected error for invalid version")
				return
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRunUpdateScript_InvalidInitiator(t *testing.T) {
	// Initiator is interpolated into the shell script the same way versions
	// are, so it gets the same allow-list treatment.
	s := noExecService(t)

	for _, initiator := range []string{"", "cron", `bot";rm -rf /;"`} {
		t.Run(initiator, func(t *testing.T) {
			opts := validOpts()
			opts.Initiator = initiator
			err := s.RunUpdateScript(opts)
			if err == nil || !strings.Contains(err.Error(), "invalid initiator") {
				t.Fatalf("RunUpdateScript(initiator=%q) error = %v, want invalid initiator", initiator, err)
			}
		})
	}
}

func TestRunUpdateScript_ValidVersion(t *testing.T) {
	s := noExecService(t)

	// The exec is expected to fail - the interpreter does not exist. A
	// validation error is not: it would mean valid versions were rejected.
	err := s.RunUpdateScript(validOpts())

	if err != nil && strings.Contains(err.Error(), "invalid") {
		t.Errorf("Valid versions should pass validation: %v", err)
	}
	assertExecRefused(t, s, err)

	// Check that script file was created
	if _, err := os.Stat(s.getScriptFile()); os.IsNotExist(err) {
		t.Error("Script file should be created")
	}
}

func TestRunUpdateScript_ScriptContent(t *testing.T) {
	s := noExecService(t)

	// The exec fails, but the script is written before it is launched
	err := s.RunUpdateScript(RunOptions{ChatID: 42, OldVersion: "v1.2.3", NewVersion: "v2.0.0", Initiator: "bot"})
	assertExecRefused(t, s, err)

	// Read the generated script
	content, err := os.ReadFile(s.getScriptFile())
	if err != nil {
		t.Fatalf("Failed to read script: %v", err)
	}

	script := string(content)

	// Verify script has correct shebang
	if !strings.HasPrefix(script, "#!/bin/sh") {
		t.Error("Script should start with #!/bin/sh")
	}

	// Verify embedded values
	if !strings.Contains(script, "CHAT_ID=42") {
		t.Error("Script missing correct CHAT_ID")
	}
	if !strings.Contains(script, `OLD_VERSION="v1.2.3"`) {
		t.Error("Script missing correct OLD_VERSION")
	}
	if !strings.Contains(script, `NEW_VERSION="v2.0.0"`) {
		t.Error("Script missing correct NEW_VERSION")
	}
}

// The bot deletes the whole update directory the moment it has reported a
// successful update - while this script is still on its last steps. Started
// inside that directory, the script is left with a working directory that no
// longer exists, and so is every daemon it starts. On the router monit then
// refuses to run at all ("Monit: Cannot read current directory"), so step 7's
// re-monitor is swallowed by its own `2>/dev/null || true` and the bot stays
// unmonitored; and every shell the daemons spawn prints "shell-init: error
// retrieving current directory" into whatever the Web UI is showing.
func TestUpdateCommand_StartsTheScriptOutsideTheDirectoryTheUpdateDeletes(t *testing.T) {
	updateDir := t.TempDir()
	s := &Service{updateDir: updateDir, shell: "/bin/sh"}

	cwdFile := filepath.Join(t.TempDir(), "cwd")
	probe := filepath.Join(updateDir, "probe.sh")
	if err := os.WriteFile(probe, []byte("pwd > "+cwdFile+"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	out, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()

	if err := s.updateCommand(probe, out).Run(); err != nil {
		t.Fatalf("run the probe script: %v", err)
	}

	data, err := os.ReadFile(cwdFile)
	if err != nil {
		t.Fatalf("read the probe's working directory: %v", err)
	}
	got := strings.TrimSpace(string(data))
	doomed, err := filepath.EvalSymlinks(updateDir)
	if err != nil {
		t.Fatal(err)
	}
	if got == doomed || strings.HasPrefix(got, doomed+string(os.PathSeparator)) {
		t.Errorf("the script runs from %q, inside the directory the update deletes", got)
	}
}

// The same deletion takes update.log with it, so every log call after that
// point writes into a directory that is gone. Under set -e a failing redirect
// ends the script where it stands - taking the monit re-monitor and the lock
// removal of steps 7 and 8 with it.
func TestGenerateScript_LogSurvivesTheDirectoryTheBotDeletes(t *testing.T) {
	s := &Service{}
	script, err := s.generateScript(validOpts())
	if err != nil {
		t.Fatalf("generateScript() error = %v", err)
	}

	logFn := regexp.MustCompile(`(?ms)^log\(\) \{.*?^\}`).FindString(script)
	if logFn == "" {
		t.Fatal("no log() definition in the generated script")
	}

	gone := filepath.Join(t.TempDir(), "removed", "update.log")
	snippet := "set -e\n" + logFn + "\nLOG_FILE=" + gone + "\nlog 'a line'\necho SURVIVED\n"
	out, err := exec.Command("/bin/sh", "-c", snippet).CombinedOutput()
	if err != nil {
		t.Fatalf("the script dies once its log file is gone: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "SURVIVED") {
		t.Errorf("output %q, want the shell to carry on past the failed log", out)
	}
}
