//go:build !linux
// +build !linux

package runtime

import (
	"os/exec"
)

// setupContainerIsolation is a no-op on non-Linux systems
func setupContainerIsolation(cmd *exec.Cmd, rootFS string) {
	// On non-Linux systems, we can't use chroot/namespaces
	// This is a limitation of the design
}
