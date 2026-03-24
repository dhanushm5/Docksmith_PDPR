package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dhanushm/docksmith/engine"
	"github.com/dhanushm/docksmith/runtime"
	"github.com/dhanushm/docksmith/state"
	"github.com/dhanushm/docksmith/utils"
)

// StringSlice is a flag.Value implementation for string arrays
type StringSlice []string

func (ss *StringSlice) String() string {
	return strings.Join(*ss, ",")
}

func (ss *StringSlice) Set(value string) error {
	*ss = append(*ss, value)
	return nil
}

// Build command handler
func Build(stateDir, imageTag, contextDir string) error {
	// Parse image name and tag
	parts := strings.SplitN(imageTag, ":", 2)
	imageName := parts[0]
	tag := "latest"
	if len(parts) == 2 {
		tag = parts[1]
	}

	// Validate context directory
	if _, err := os.Stat(contextDir); err != nil {
		return fmt.Errorf("context directory not found: %s", contextDir)
	}

	// Get absolute path
	absCtx, err := filepath.Abs(contextDir)
	if err != nil {
		return err
	}

	fmt.Printf("Building %s:%s from context %s\n", imageName, tag, absCtx)

	be := &engine.BuildEngine{
		StateDir:   stateDir,
		ContextDir: absCtx,
		NoCache:    false,
	}

	output, err := be.Build(imageName, tag)
	if err != nil {
		return err
	}

	fmt.Printf("\nBuild completed successfully!\n")
	fmt.Printf("Image: %s:%s\n", output.ImageName, output.ImageTag)
	fmt.Printf("Digest: %s\n", output.Digest)

	return nil
}

// Images command handler
func Images(stateDir string) error {
	manifests, err := state.ListManifests(stateDir)
	if err != nil {
		return err
	}

	if len(manifests) == 0 {
		fmt.Println("No images found")
		return nil
	}

	fmt.Println("REPOSITORY\tTAG\tIMAGE ID\tCREATED")
	fmt.Println("-\t-\t-\t-")

	for _, m := range manifests {
		digestShort := m.Digest[:12]
		fmt.Printf("%s\t%s\t%s\t%s\n", m.Name, m.Tag, digestShort, m.Created)
	}

	return nil
}

// Run command handler
func Run(stateDir, imageTag string, cmdArgs []string, envVars StringSlice) error {
	// Parse image name and tag
	parts := strings.SplitN(imageTag, ":", 2)
	imageName := parts[0]
	tag := "latest"
	if len(parts) == 2 {
		tag = parts[1]
	}

	// Load image manifest
	manifest, err := state.LoadManifest(stateDir, imageName, tag)
	if err != nil {
		return err
	}

	// Create assembled filesystem in temp directory
	tempDir, err := os.MkdirTemp("", "docksmith-run-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Extract all layers in order
	for _, layer := range manifest.Layers {
		layerPath := filepath.Join(stateDir, "layers", layer.Digest)
		if err := utils.ExtractTar(layerPath, tempDir); err != nil {
			return fmt.Errorf("failed to extract layer: %w", err)
		}
	}

	// Parse environment overrides
	overrideEnv := make(map[string]string)
	for _, envVar := range envVars {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) == 2 {
			overrideEnv[parts[0]] = parts[1]
		}
	}

	// Determine command to run
	var command []string
	if len(cmdArgs) > 0 {
		command = cmdArgs
	} else if len(manifest.Config.Cmd) > 0 {
		command = manifest.Config.Cmd
	} else {
		command = []string{"/bin/sh"}
	}

	// Setup runtime config
	config := &runtime.ContainerConfig{
		RootFS:      tempDir,
		WorkDir:     manifest.Config.WorkDir,
		Env:         manifest.Config.Env,
		Command:     command,
		OverrideEnv: overrideEnv,
	}

	// Run container
	exitCode, err := runtime.Run(config)
	if err != nil {
		return err
	}

	if exitCode != 0 {
		os.Exit(exitCode)
	}

	return nil
}

// Rmi command handler
func Rmi(stateDir, imageTag string) error {
	// Parse image name and tag
	parts := strings.SplitN(imageTag, ":", 2)
	imageName := parts[0]
	tag := "latest"
	if len(parts) == 2 {
		tag = parts[1]
	}

	// Check if image exists
	_, err := state.LoadManifest(stateDir, imageName, tag)
	if err != nil {
		return fmt.Errorf("image not found: %s:%s", imageName, tag)
	}

	// Delete manifest
	if err := state.DeleteManifest(stateDir, imageName, tag); err != nil {
		return fmt.Errorf("failed to delete image: %w", err)
	}

	fmt.Printf("Removed %s:%s\n", imageName, tag)

	return nil
}
