//go:build linux
// +build linux

package runtime

import (
	"os/exec"
	"syscall"
)

// setupContainerIsolation configures Linux-specific process isolation
func setupContainerIsolation(cmd *exec.Cmd, rootFS string) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Chroot:     rootFS,
		Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS | syscall.CLONE_NEWIPC,
	}
}
