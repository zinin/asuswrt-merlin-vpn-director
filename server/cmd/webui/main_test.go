package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// runMainEnv makes the test binary run main() instead of its tests, so a test
// can start the real entry point as a child process.
const runMainEnv = "VPD_TEST_RUN_MAIN"

func TestMain(m *testing.M) {
	if os.Getenv(runMainEnv) == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestSelfUpdateIsDispatchedBeforeTheDaemonStarts pins the entry point of the
// self-update contract (internal/updater/selfupdate.go): the version being
// replaced runs this binary as "self-update ...", and nothing a daemon does at
// startup may come first. This binary is a dev build, so step 2 refuses the
// version it is asked to install - and only step 2 prints that refusal.
func TestSelfUpdateIsDispatchedBeforeTheDaemonStarts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0],
		"self-update", "--from", "v9.9.8", "--to", "v1.0.0", "--initiator", "webui", "--chat-id", "0")
	cmd.Env = append(os.Environ(), runMainEnv+"=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("exit = %v, want status 1\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "asked to install v1.0.0") {
		t.Errorf("stderr = %q, want step 2's version refusal", stderr.String())
	}
	// Contract item 3: every stdout line is a progress line for the user, and
	// this daemon's logger writes to stdout - a dispatch that ever moved below
	// a startup log line would put that line in front of the user.
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing but what step 2 prints", stdout.String())
	}
}
