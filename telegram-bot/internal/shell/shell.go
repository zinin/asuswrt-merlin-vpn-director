package shell

import "os/exec"

type Result struct {
	Output   string
	ExitCode int
}

func Exec(command string, args ...string) (*Result, error) {
	cmd := exec.Command(command, args...)
	output, err := cmd.CombinedOutput()

	exitCode := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
		err = nil // non-zero exit is not an error
	}

	return &Result{
		Output:   string(output),
		ExitCode: exitCode,
	}, err
}
