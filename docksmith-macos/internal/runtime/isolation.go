package runtime

import (
	"fmt"
	"io"
)

type ExecOptions struct {
	RootFS  string
	WorkDir string
	Env     []string
	Command []string
	Stdout  io.Writer
	Stderr  io.Writer
	Stdin   io.Reader
}

func ExecuteIsolated(opts ExecOptions) (int, error) {
	if len(opts.Command) == 0 {
		return 0, fmt.Errorf("empty command")
	}
	return executeIsolated(opts)
}
