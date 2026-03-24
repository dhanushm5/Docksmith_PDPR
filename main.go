package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dhanushm/docksmith/cli"
	"github.com/dhanushm/docksmith/state"
)

func main() {
	// Ensure ~/.docksmith exists
	stateDir := filepath.Join(os.Getenv("HOME"), ".docksmith")
	if err := state.InitStateDir(stateDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing state: %v\n", err)
		os.Exit(1)
	}

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "build":
		buildCmd := flag.NewFlagSet("build", flag.ExitOnError)
		tagFlag := buildCmd.String("t", "", "Image name and tag (name:tag)")
		buildCmd.Parse(os.Args[2:])

		if *tagFlag == "" {
			fmt.Fprintf(os.Stderr, "Error: -t flag is required\n")
			os.Exit(1)
		}

		if buildCmd.NArg() != 1 {
			fmt.Fprintf(os.Stderr, "Error: build context directory required\n")
			os.Exit(1)
		}

		contextDir := buildCmd.Arg(0)
		if err := cli.Build(stateDir, *tagFlag, contextDir); err != nil {
			fmt.Fprintf(os.Stderr, "Build failed: %v\n", err)
			os.Exit(1)
		}

	case "images":
		if err := cli.Images(stateDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error listing images: %v\n", err)
			os.Exit(1)
		}

	case "run":
		runCmd := flag.NewFlagSet("run", flag.ExitOnError)
		envVars := cli.StringSlice{}
		runCmd.Var(&envVars, "e", "Environment variable (KEY=VALUE)")
		runCmd.Parse(os.Args[2:])

		if runCmd.NArg() < 1 {
			fmt.Fprintf(os.Stderr, "Error: image name:tag required\n")
			os.Exit(1)
		}

		imageTag := runCmd.Arg(0)
		cmdArgs := runCmd.Args()[1:]

		if err := cli.Run(stateDir, imageTag, cmdArgs, envVars); err != nil {
			fmt.Fprintf(os.Stderr, "Run failed: %v\n", err)
			os.Exit(1)
		}

	case "rmi":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: image name:tag required\n")
			os.Exit(1)
		}
		imageTag := os.Args[2]
		if err := cli.Rmi(stateDir, imageTag); err != nil {
			fmt.Fprintf(os.Stderr, "Remove failed: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Docksmith - Docker-like build system

Usage:
  docksmith build -t name:tag <context>
  docksmith images
  docksmith run [-e KEY=VALUE] name:tag [command args...]
  docksmith rmi name:tag
`)
}
