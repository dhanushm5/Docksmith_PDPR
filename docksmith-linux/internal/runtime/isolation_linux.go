//go:build linux

package runtime

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	if errors.Is(err, syscall.EPERM) {
		w := opts.Stderr
		if w == nil {
			w = os.Stderr
		}
		fmt.Fprintln(w, "docksmith: strict linux isolation unavailable (operation not permitted); falling back to compatibility mode")
		return executeCompat(opts)
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus(), nil
		}
	}
	return 0, fmt.Errorf("run isolated command: %w", err)
}

func executeCompat(opts ExecOptions) (int, error) {
	if len(opts.Command) == 0 {
		return 0, fmt.Errorf("empty command")
	}

	workDir := opts.WorkDir
	if strings.HasPrefix(workDir, "/") {
		workDir = filepath.Join(opts.RootFS, strings.TrimPrefix(workDir, "/"))
	}
	if workDir == "" {
		workDir = opts.RootFS
	}

	cmdArgs := append([]string(nil), opts.Command...)
	if len(cmdArgs) >= 3 && (filepath.Base(cmdArgs[0]) == "sh" || filepath.Base(cmdArgs[0]) == "bash") && cmdArgs[1] == "-c" {
		script := cmdArgs[2]
		script = strings.ReplaceAll(script, " /app/", " "+filepath.Join(opts.RootFS, "app")+string(filepath.Separator))
		script = strings.ReplaceAll(script, " /app", " "+filepath.Join(opts.RootFS, "app"))
		script = strings.ReplaceAll(script, "'/app/", "'"+filepath.Join(opts.RootFS, "app")+string(filepath.Separator))
		script = strings.ReplaceAll(script, "'/app", "'"+filepath.Join(opts.RootFS, "app"))
		cmdArgs[2] = script
	} else {
		for i := 1; i < len(cmdArgs); i++ {
			if strings.HasPrefix(cmdArgs[i], "/") {
				cmdArgs[i] = filepath.Join(opts.RootFS, strings.TrimPrefix(cmdArgs[i], "/"))
			}
		}
	}

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Env = append([]string(nil), opts.Env...)
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.Stdin = opts.Stdin
	cmd.Dir = workDir

	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus(), nil
		}
	}
	return 0, fmt.Errorf("run command in compatibility mode: %w", err)
}
