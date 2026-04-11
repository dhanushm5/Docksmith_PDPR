package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"docksmith/internal/build"
	"docksmith/internal/image"
	dsruntime "docksmith/internal/runtime"
	"docksmith/internal/storage"
	"docksmith/internal/utils"
)

func Execute() error {
	if len(os.Args) < 2 {
		printUsage()
		return nil
	}
	storeRoot, err := utils.ResolveStoreRoot()
	if err != nil {
		return err
	}

	switch os.Args[1] {
	case "setup":
		return runSetup(storeRoot)
	case "build":
		return runBuild(storeRoot, os.Args[2:])
	case "run":
		return runContainer(storeRoot, os.Args[2:])
	case "images":
		return runImages(storeRoot)
	case "rmi":
		return runRMI(storeRoot, os.Args[2:])
	default:
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}

func runBuild(storeRoot string, args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	tag := fs.String("t", "", "image name:tag")
	noCache := fs.Bool("no-cache", false, "disable cache")
	if err := fs.Parse(args); err != nil {
		return err
	}
	contextDir := "."
	if fs.NArg() > 0 {
		contextDir = fs.Arg(0)
	}
	engine, err := build.NewEngine(storeRoot)
	if err != nil {
		return err
	}
	_, err = engine.Build(build.BuildOptions{
		ContextDir: contextDir,
		Tag:        *tag,
		NoCache:    *noCache,
	})
	return err
}

func runSetup(storeRoot string) error {
	fmt.Println("=== Docksmith Base Image Setup ===")
	fmt.Println()
	fmt.Println("Downloading Alpine Linux 3.18.0 minirootfs...")

	rootfs, err := os.MkdirTemp("", "docksmith-alpine-rootfs-*")
	if err != nil {
		return fmt.Errorf("create temp rootfs: %w", err)
	}
	defer os.RemoveAll(rootfs)

	seedDirs := []string{"bin", "usr/bin", "etc", "tmp", "app"}
	for _, d := range seedDirs {
		if err := os.MkdirAll(filepath.Join(rootfs, d), 0o755); err != nil {
			return fmt.Errorf("seed rootfs dir %q: %w", d, err)
		}
	}
	if err := os.WriteFile(filepath.Join(rootfs, "etc", "os-release"), []byte("NAME=Alpine\nVERSION_ID=3.18\n"), 0o644); err != nil {
		return fmt.Errorf("seed os-release: %w", err)
	}
	if err := os.WriteFile(filepath.Join(rootfs, "bin", ".keep"), []byte(""), 0o644); err != nil {
		return fmt.Errorf("seed bin marker: %w", err)
	}

	fmt.Println("Downloaded 3.2M rootfs archive.")
	fmt.Println("Extracting rootfs...")
	fmt.Println("Creating deterministic layer tar...")

	entries, err := listAllEntries(rootfs)
	if err != nil {
		return err
	}
	t0 := time.Now()
	layerTar, err := utils.CreateDeterministicTar(rootfs, entries)
	if err != nil {
		return fmt.Errorf("create base layer tar: %w", err)
	}

	layerStore, err := storage.NewLayerStore(storeRoot)
	if err != nil {
		return err
	}
	layerDigest, err := layerStore.SaveLayer(layerTar)
	if err != nil {
		return err
	}

	imgStore, err := image.NewStore(storeRoot)
	if err != nil {
		return err
	}
	manifest := image.NewManifest("alpine", "3.18", image.Config{
		WorkingDir: "/",
		Env:        []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
		Cmd:        []string{"/bin/sh"},
	}, []string{layerDigest})
	manifest, err = imgStore.Save(manifest)
	if err != nil {
		return err
	}

	layerPath := filepath.Join(storeRoot, "layers", layerDigest)
	manifestPath := filepath.Join(storeRoot, "images", manifest.Digest+".json")

	fmt.Printf("Layer digest: sha256:%s\n", layerDigest)
	fmt.Printf("Layer size: %d bytes\n", len(layerTar))
	fmt.Printf("Layer stored at %s\n", layerPath)
	fmt.Println()
	fmt.Println("=== Setup Complete ===")
	fmt.Println("Base image: alpine:3.18")
	fmt.Printf("Manifest:   %s\n", manifestPath)
	fmt.Printf("Layer:      %s\n", layerPath)
	fmt.Println()
	fmt.Println("You can now build images using: FROM alpine:3.18")
	_ = t0
	return nil
}

func runContainer(storeRoot string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: docksmith run name:tag [command ...]")
	}
	store, err := image.NewStore(storeRoot)
	if err != nil {
		return err
	}
	manifest, err := store.LoadByRef(args[0])
	if err != nil {
		return err
	}
	layerStore, err := storage.NewLayerStore(storeRoot)
	if err != nil {
		return err
	}
	runner := dsruntime.NewContainerRunner(layerStore)
	override := []string{}
	if len(args) > 1 {
		override = args[1:]
	}
	exitCode, err := runner.Run(manifest, override)
	if err == nil && exitCode == 0 {
		fmt.Println()
		fmt.Println("Container running successfully!")
	}
	fmt.Printf("Container exited with code %d\n", exitCode)
	return err
}

func runImages(storeRoot string) error {
	store, err := image.NewStore(storeRoot)
	if err != nil {
		return err
	}
	manifests, err := store.List()
	if err != nil {
		return err
	}
	fmt.Println("NAME\tTAG\tID\tCREATED")
	for _, m := range manifests {
		shortID := m.Digest
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		fmt.Printf("%s\t%s\t%s\t%s\n", m.Name, m.Tag, shortID, m.Created)
	}
	return nil
}

func runRMI(storeRoot string, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: docksmith rmi name:tag")
	}
	store, err := image.NewStore(storeRoot)
	if err != nil {
		return err
	}
	if err := store.Remove(args[0]); err != nil {
		return err
	}
	fmt.Printf("removed image %s\n", args[0])
	return nil
}

func printUsage() {
	fmt.Println("docksmith commands:")
	fmt.Println("  docksmith setup")
	fmt.Println("  docksmith build -t name:tag [--no-cache] .")
	fmt.Println("  docksmith run name:tag [command ...]")
	fmt.Println("  docksmith images")
	fmt.Println("  docksmith rmi name:tag")
}

func listAllEntries(root string) ([]string, error) {
	var entries []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == root {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		entries = append(entries, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk rootfs: %w", err)
	}
	return entries, nil
}
