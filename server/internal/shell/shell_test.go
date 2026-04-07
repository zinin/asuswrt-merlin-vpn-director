package shell

import (
	"strings"
	"testing"
)

func TestExec_EchoHello(t *testing.T) {
	result, err := Exec("echo", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	expected := "hello\n"
	if result.Output != expected {
		t.Errorf("expected output %q, got %q", expected, result.Output)
	}
}

func TestExec_NonZeroExitCode(t *testing.T) {
	result, err := Exec("sh", "-c", "exit 42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", result.ExitCode)
	}
}

func TestExec_CommandWithOutput(t *testing.T) {
	result, err := Exec("sh", "-c", "echo first; echo second")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	if !strings.Contains(result.Output, "first") || !strings.Contains(result.Output, "second") {
		t.Errorf("expected output to contain 'first' and 'second', got %q", result.Output)
	}
}

func TestExec_StderrCaptured(t *testing.T) {
	result, err := Exec("sh", "-c", "echo error >&2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	if !strings.Contains(result.Output, "error") {
		t.Errorf("expected stderr to be captured, got %q", result.Output)
	}
}

func TestExec_CommandNotFound(t *testing.T) {
	_, err := Exec("/nonexistent/command/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for non-existent command")
	}
}

func TestExec_ExitCodeWithOutput(t *testing.T) {
	result, err := Exec("sh", "-c", "echo output; exit 5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ExitCode != 5 {
		t.Errorf("expected exit code 5, got %d", result.ExitCode)
	}

	if !strings.Contains(result.Output, "output") {
		t.Errorf("expected output to contain 'output', got %q", result.Output)
	}
}
