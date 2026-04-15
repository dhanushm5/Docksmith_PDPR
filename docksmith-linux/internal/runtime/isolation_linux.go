//go:build linux

package runtime

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"
)

func executeIsolated(opts ExecOptions) (int, error) {
	cmdPath, err := resolveCommandPath(opts.RootFS, opts.Command[0], opts.Env)
	if err != nil {
		return 0, err
	}
	cmd := exec.Command(cmdPath, opts.Command[1:]...)
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

	runErr := cmd.Run()
	if runErr == nil {
		return 0, nil
	}
	if errors.Is(runErr, syscall.EPERM) {
		w := opts.Stderr
		if w == nil {
			w = os.Stderr
		}
		fmt.Fprintln(w, "docksmith: strict linux isolation unavailable (operation not permitted); falling back to compatibility mode")
		return executeCompat(opts)
	}
	if exitErr, ok := runErr.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus(), nil
		}
	}
	return 0, fmt.Errorf("run isolated command: %w", runErr)
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

	cmdPath, err := resolveCommandPath(opts.RootFS, opts.Command[0], opts.Env)
	if err != nil {
		return 0, err
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

	cmd := exec.Command(cmdPath, cmdArgs[1:]...)
	cmd.Env = append([]string(nil), opts.Env...)
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.Stdin = opts.Stdin
	cmd.Dir = workDir

	runErr := cmd.Run()
	if runErr == nil {
		return 0, nil
	}
	if exitErr, ok := runErr.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus(), nil
		}
	}
	return 0, fmt.Errorf("run command in compatibility mode: %w", runErr)
}

func resolveCommandPath(rootfs, command string, env []string) (string, error) {
	if command == "" {
		return "", fmt.Errorf("empty command")
	}
	if strings.Contains(command, "/") {
		return command, nil
	}

	searchPath := "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	for _, item := range env {
		if strings.HasPrefix(item, "PATH=") {
			searchPath = strings.TrimPrefix(item, "PATH=")
			break
		}
	}

	for _, dir := range strings.Split(searchPath, ":") {
		if dir == "" {
			continue
		}
		candidateRel := strings.TrimPrefix(path.Join(dir, command), "/")
		candidate := filepath.Join(rootfs, filepath.FromSlash(candidateRel))
		if info, err := os.Lstat(candidate); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return path.Join(dir, command), nil
			}
			if info.Mode()&0o111 != 0 {
				return path.Join(dir, command), nil
			}
		}
		if info, err := os.Stat(candidate); err == nil && info.Mode()&0o111 != 0 {
			return path.Join(dir, command), nil
		}
	}

	return "", fmt.Errorf("executable %q not found in rootfs", command)
}
