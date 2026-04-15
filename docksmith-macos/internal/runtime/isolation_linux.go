//go:build linux

package runtime

import (
	"fmt"
	"os/exec"
	"syscall"
)

func executeIsolated(opts ExecOptions) (int, error) {
	cmd := exec.Command(opts.Command[0], opts.Command[1:]...)
	cmd.Env = append([]string(nil), opts.Env...)
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.Stdin = opts.Stdin
	cmd.Dir = opts.WorkDir
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Chroot: opts.RootFS,
		Cloneflags: syscall.CLONE_NEWNS |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWIPC |
			syscall.CLONE_NEWNET,
	}

	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus(), nil
		}
	}
	return 0, fmt.Errorf("run isolated command: %w", err)
}
