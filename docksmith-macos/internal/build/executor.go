package build

import (
	"fmt"
	"os"
	"sort"
	"strings"

	dsruntime "docksmith/internal/runtime"
)

func ExecuteRunInstruction(rootfs, workDir string, env map[string]string, runCommand string) (int, error) {
	if runCommand == "" {
		return 0, fmt.Errorf("RUN command is empty")
	}

	pairs := make([]string, 0, len(env))
	for k, v := range env {
		pairs = append(pairs, k+"="+v)
	}
	sort.Strings(pairs)
	if !hasEnvKey(pairs, "PATH") {
		pairs = append(pairs, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	}

	return dsruntime.ExecuteIsolated(dsruntime.ExecOptions{
		RootFS:  rootfs,
		WorkDir: workDir,
		Env:     pairs,
		Command: []string{"/bin/sh", "-c", runCommand},
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Stdin:   os.Stdin,
	})
}

func hasEnvKey(env []string, key string) bool {
	prefix := key + "="
	for _, pair := range env {
		if strings.HasPrefix(pair, prefix) {
			return true
		}
	}
	return false
}
