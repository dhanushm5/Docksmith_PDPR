package runtime

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"docksmith/internal/image"
	"docksmith/internal/storage"
)

type ContainerRunner struct {
	layerStore *storage.LayerStore
}

func NewContainerRunner(layerStore *storage.LayerStore) *ContainerRunner {
	return &ContainerRunner{layerStore: layerStore}
}

func (r *ContainerRunner) Run(manifest image.Manifest, overrideCmd []string) (int, error) {
	rootfs, err := os.MkdirTemp("", "docksmith-rootfs-*")
	if err != nil {
		return 0, fmt.Errorf("create temp rootfs: %w", err)
	}
	defer os.RemoveAll(rootfs)

	if err := r.layerStore.ApplyLayers(manifest.Layers, rootfs); err != nil {
		return 0, err
	}

	cmd := manifest.Config.Cmd
	if len(overrideCmd) > 0 {
		cmd = overrideCmd
	}
	if len(cmd) == 0 {
		return 0, fmt.Errorf("no command to run")
	}

	workdir := manifest.Config.WorkingDir
	if workdir == "" {
		workdir = "/"
	}
	mergedEnv := mergeEnv(append([]string(nil), manifest.Config.Env...), os.Environ())

	exitCode, err := ExecuteIsolated(ExecOptions{
		RootFS:  rootfs,
		WorkDir: workdir,
		Env:     mergedEnv,
		Command: cmd,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Stdin:   os.Stdin,
	})
	if err != nil {
		return exitCode, err
	}
	return exitCode, nil
}

func mergeEnv(base, override []string) []string {
	pairs := map[string]string{}
	for _, item := range base {
		k, v, ok := strings.Cut(item, "=")
		if ok {
			pairs[k] = v
		}
	}
	for _, item := range override {
		k, v, ok := strings.Cut(item, "=")
		if ok {
			pairs[k] = v
		}
	}
	out := make([]string, 0, len(pairs))
	for k, v := range pairs {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}
