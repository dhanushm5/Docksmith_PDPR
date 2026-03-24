package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ContainerConfig holds runtime configuration
type ContainerConfig struct {
	RootFS      string            // Path to assembled filesystem
	WorkDir     string            // Working directory inside container
	Env         map[string]string // Environment variables
	Command     []string          // Command to execute
	OverrideEnv map[string]string // Environment overrides from -e flags
}

// Run executes a command in an isolated container
func Run(config *ContainerConfig) (int, error) {
	if os.Geteuid() != 0 {
		return 1, fmt.Errorf("container runtime requires root privileges")
	}

	// Create temporary working directory if WorkDir doesn't exist
	workDir := config.WorkDir
	if workDir == "" {
		workDir = "/"
	}

	// Ensure the workdir path exists in the root, or use /
	if workDir != "/" {
		workDirPath := filepath.Join(config.RootFS, workDir)
		if err := os.MkdirAll(workDirPath, 0755); err != nil {
			return 1, fmt.Errorf("failed to create workdir: %w", err)
		}
	}

	// Build command
	var cmdName string
	var cmdArgs []string

	if len(config.Command) == 0 {
		cmdName = "/bin/sh"
		cmdArgs = []string{"-c", "echo 'No command specified'"}
	} else {
		cmdName = config.Command[0]
		cmdArgs = config.Command[1:]
	}

	cmd := exec.Command(cmdName, cmdArgs...)

	// Prepare environment
	env := make([]string, 0)
	for k, v := range config.Env {
		if override, exists := config.OverrideEnv[k]; exists {
			env = append(env, k+"="+override)
		} else {
			env = append(env, k+"="+v)
		}
	}
	// Add overrides that don't override existing vars
	for k, v := range config.OverrideEnv {
		found := false
		for envVar := range config.Env {
			if envVar == k {
				found = true
				break
			}
		}
		if !found {
			env = append(env, k+"="+v)
		}
	}

	cmd.Env = env
	cmd.Dir = workDir

	// Setup process isolation (Linux-specific)
	// On other systems, this will not use chroot but still work
	setupContainerIsolation(cmd, config.RootFS)

	// Redirect I/O
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Execute
	err := cmd.Run()

	// Get exit code
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return 1, fmt.Errorf("execution failed: %w", err)
		}
	}

	return exitCode, nil
}

// BuildRun executes a command during build (similar to Run but returns output)
func BuildRun(rootFS, command string, env map[string]string, workDir string) (string, error) {
	if os.Geteuid() != 0 {
		return "", fmt.Errorf("container runtime requires root privileges")
	}

	if workDir == "" {
		workDir = "/"
	}

	cmd := exec.Command("/bin/sh", "-c", command)

	// Prepare environment
	envList := make([]string, 0)
	for k, v := range env {
		envList = append(envList, k+"="+v)
	}

	cmd.Env = envList
	cmd.Dir = workDir

	// Setup chroot with process isolation (Linux-specific)
	setupContainerIsolation(cmd, rootFS)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("command failed: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}
