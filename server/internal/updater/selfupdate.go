package updater

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

// The self-update handover.
//
// The daemon a user asks to update does not install the new release itself.
// Step 1 (Handover, handover.go) runs in that daemon: it downloads the new
// release's binary of its own daemon as InstallerFile and runs
//
//	installer self-update --from <vX.Y.Z> --to <vX.Y.Z> --initiator <bot|webui> --chat-id <int64>
//
// Step 2 (RunSelfUpdate, below) runs in that new binary and installs the
// release with its own code: DownloadRelease, then RunUpdateScript with its own
// template. A release therefore installs itself, and may change its file list,
// its daemons, its manifest and its update script at will.
//
// Step 1 of a release starts step 2 of every later release, and a router
// cannot update the step 1 it runs. This is the contract between the two.
// Later releases may add to it; they must never remove or rename anything in
// it. testdata/selfupdate_argv.txt holds every invocation a released step 1
// builds, and step 2 must parse all of them.
//
//  1. Asset: step 1 runs "<daemon>-<arch>" of its own daemon, else of another
//     daemon in its Daemons table. Every such binary implements self-update
//     and stays below maxFileSize.
//  2. Invocation: the argv above, working directory "/", stdin /dev/null,
//     the daemon's environment. --chat-id is 0 when the Web UI started it.
//  3. Result: every stdout line is a progress line for the user. Exit 0 means
//     the update script has started and owns the lock and files/. Any other
//     exit means no script started and nothing outside UpdateDir changed; the
//     last non-empty stderr line is the reason shown to the user. A step 2
//     that dies of a Go runtime failure is reported by its first "panic:" or
//     "fatal error:" line instead: the last line of a crash is a stack frame.
//  4. Files: UpdateDir, and LockFile holding the PID of its owner - step 1
//     while step 2 runs, then the script. notify.json only gains fields, and
//     its status stays "ok" or "failed": a bot on this release reads every
//     other value as success, announces "Update complete: <from> → <to>" and
//     clears UpdateDir, update.log included.
//  5. Version: step 2 refuses unless --to is the version compiled into it.
//  6. Limits: step 1 fetches the installer under maxFileSize and
//     downloadTimeout (downloader.go). It passes on the first
//     maxProgressLines stdout lines, each cut to maxProgressRunes runes, and
//     only logs the rest. After defaultHandoverTimeout it asks step 2 to stop
//     with SIGTERM and kills it defaultInstallerWaitDelay later; output that
//     a process step 2 started keeps open is abandoned
//     defaultInstallerWaitDelay after step 2 exits.
//
// Step 2 never logs: its stderr is the channel for the reason in item 3.

// SelfUpdateCommand is the first argument that makes a daemon binary run
// step 2 instead of starting as a daemon.
const SelfUpdateCommand = "self-update"

// selfUpdateArgs is the parsed invocation of step 2.
type selfUpdateArgs struct {
	From      string
	To        string
	Initiator string
	ChatID    int64
}

// selfUpdateArgv is the invocation step 1 builds. Changing it means appending
// the new form to testdata/selfupdate_argv.txt.
func selfUpdateArgv(opts RunOptions) []string {
	return []string{
		SelfUpdateCommand,
		"--from", opts.OldVersion,
		"--to", opts.NewVersion,
		"--initiator", opts.Initiator,
		"--chat-id", strconv.FormatInt(opts.ChatID, 10),
	}
}

// parseSelfUpdateArgs parses the arguments after SelfUpdateCommand. The values
// end up in a shell script, so they pass the checks RunUpdateScript makes.
func parseSelfUpdateArgs(args []string) (selfUpdateArgs, error) {
	var a selfUpdateArgs
	fs := flag.NewFlagSet(SelfUpdateCommand, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&a.From, "from", "", "version being replaced")
	fs.StringVar(&a.To, "to", "", "version to install, the one compiled into this binary")
	fs.StringVar(&a.Initiator, "initiator", "", "bot or webui")
	fs.Int64Var(&a.ChatID, "chat-id", 0, "chat to report to, 0 for the Web UI")
	if err := fs.Parse(args); err != nil {
		return a, err
	}
	if fs.NArg() != 0 {
		return a, fmt.Errorf("unexpected arguments %q", fs.Args())
	}
	if !IsValidVersion(a.From) {
		return a, fmt.Errorf("invalid --from %q", a.From)
	}
	if !IsValidVersion(a.To) {
		return a, fmt.Errorf("invalid --to %q", a.To)
	}
	if a.Initiator != "bot" && a.Initiator != "webui" {
		return a, fmt.Errorf("invalid --initiator %q", a.Initiator)
	}
	return a, nil
}

// RunSelfUpdate is step 2. main calls it with os.Args[2:] when os.Args[1] is
// SelfUpdateCommand, before anything else a daemon does at startup. daemon is
// the daemon this binary is, version the version compiled into it. The result
// is the process exit code.
func RunSelfUpdate(args []string, daemon, version string, stdout, stderr io.Writer) int {
	s := NewForDaemon(daemon)
	if err := s.selfUpdate(context.Background(), args, version, stdout); err != nil {
		// Step 1 shows the last stderr line as the reason (contract item 3),
		// so a multi-line error would reach the user as its tail.
		fmt.Fprintln(stderr, strings.ReplaceAll(err.Error(), "\n", " "))
		return 1
	}
	return 0
}

// selfUpdate installs the release this binary belongs to.
func (s *Service) selfUpdate(ctx context.Context, args []string, version string, stdout io.Writer) error {
	a, err := parseSelfUpdateArgs(args)
	if err != nil {
		return err
	}
	if a.To != version {
		return fmt.Errorf("this binary is %s, asked to install %s", version, a.To)
	}
	if err := s.requireParentLock(); err != nil {
		return err
	}
	release, err := s.GetReleaseByTag(ctx, a.To)
	if err != nil {
		return fmt.Errorf("release %s: %w", a.To, err)
	}
	self, err := s.getExecutable()
	if err != nil {
		return fmt.Errorf("locate this binary: %w", err)
	}
	s.selfBinary = self
	// The download takes minutes, and every file in it goes into a directory
	// this step only borrows: from here on each write asks whether the parent
	// that started this step still holds the claim. A parent that is gone
	// fails that check on its own, since os.Getppid then answers 1.
	s.checkClaim = s.requireParentLock
	if err := s.DownloadRelease(ctx, release); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Files downloaded, starting update...")
	// Checked again at the last moment: the download can take minutes, and
	// the script must not start for a claim lost meanwhile.
	if err := s.requireParentLock(); err != nil {
		return err
	}
	// From here on a SIGTERM must not end this process. Step 1 sends one when
	// its timeout fires, and a step 2 killed between the script's start and its
	// own exit is reported as a timeout, whereupon step 1 removes files/ and the
	// lock from under the script that has just started. Notify, not Ignore: an
	// ignored disposition is inherited across exec by the update script and by
	// the daemons it restarts, and a Go daemon started with SIGTERM ignored keeps
	// ignoring it. A caught signal is reset to the default in the child.
	signal.Notify(make(chan os.Signal, 1), syscall.SIGTERM)
	return s.RunUpdateScript(RunOptions{
		OldVersion: a.From,
		NewVersion: a.To,
		ChatID:     a.ChatID,
		Initiator:  a.Initiator,
	})
}

// requireParentLock refuses unless the lock names the process that started
// this step: step 1 took it before running us. A parent that died and left a
// lock another update may have replaced, or a self-update typed into a shell,
// stops here.
func (s *Service) requireParentLock() error {
	if !s.lockNamesPID(s.getParentPID()) {
		return errClaimLost
	}
	return nil
}
