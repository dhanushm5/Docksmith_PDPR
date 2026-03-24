package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dhanushm/docksmith/parser"
	"github.com/dhanushm/docksmith/runtime"
	"github.com/dhanushm/docksmith/state"
	"github.com/dhanushm/docksmith/utils"
)

type BuildEngine struct {
	StateDir   string
	ContextDir string
	NoCache    bool
}

type BuildOutput struct {
	ImageName string
	ImageTag  string
	Digest    string
}

// Build executes a full build
func (be *BuildEngine) Build(imageName, imageTag string) (*BuildOutput, error) {
	// Find and parse Docksmithfile
	docksmithPath, err := parser.FindDocksmithfile(be.ContextDir)
	if err != nil {
		return nil, err
	}

	df, err := parser.Parse(docksmithPath)
	if err != nil {
		return nil, err
	}

	// Create temporary build directory
	tempDir, err := os.MkdirTemp("", "docksmith-build-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Process instructions
	var currentRootFS string
	var layers []state.LayerInfo
	config := &state.ImageConfig{
		Env:     make(map[string]string),
		WorkDir: "/",
	}

	isCacheHit := false

	for i, instr := range df.Instructions {
		switch instr.Type {
		case "FROM":
			// Load base image
			parts := strings.Split(instr.Args[0], ":")
			baseImage := parts[0]
			baseTag := "latest"
			if len(parts) == 2 {
				baseTag = parts[1]
			}

			baseManifest, err := state.LoadManifest(be.StateDir, baseImage, baseTag)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", instr.Line, err)
			}

			// Create a fresh working directory for this build
			currentRootFS = filepath.Join(tempDir, fmt.Sprintf("layer-%d", i))
			if err := os.MkdirAll(currentRootFS, 0755); err != nil {
				return nil, err
			}

			// Extract all base layers into working directory
			for _, layer := range baseManifest.Layers {
				layerPath := filepath.Join(be.StateDir, "layers", layer.Digest)
				if err := utils.ExtractTar(layerPath, currentRootFS); err != nil {
					return nil, fmt.Errorf("failed to extract base layer: %w", err)
				}
			}

			layers = baseManifest.Layers
			config.Env = baseManifest.Config.Env
			config.WorkDir = baseManifest.Config.WorkDir
			config.Cmd = baseManifest.Config.Cmd

		case "COPY":
			if len(instr.Args) < 2 {
				return nil, fmt.Errorf("line %d: COPY requires source and destination", instr.Line)
			}

			srcPatterns := instr.Args[:len(instr.Args)-1]
			dest := instr.Args[len(instr.Args)-1]

			// Calculate cache key for this layer
			cacheKey := be.calculateCacheKey(instr, config, srcPatterns)

			// Check cache
			if !be.NoCache && isCacheHit == false {
				if cachedDigest, err := state.GetCacheKey(be.StateDir, cacheKey); err == nil && cachedDigest != "" {
					fmt.Printf("[CACHE HIT] %s\n", instr.Raw)

					// Use cached layer
					layerPath := filepath.Join(be.StateDir, "layers", cachedDigest)
					size, err := state.GetLayerSize(be.StateDir, cachedDigest)
					if err != nil {
						return nil, fmt.Errorf("failed to get layer size: %w", err)
					}

					if err := utils.ExtractTar(layerPath, currentRootFS); err != nil {
						return nil, fmt.Errorf("failed to extract layer: %w", err)
					}

					layers = append(layers, state.LayerInfo{
						Digest:    cachedDigest,
						Size:      size,
						CreatedBy: instr.Raw,
					})
					isCacheHit = true
					continue
				}
			}

			// Cache miss - execute COPY
			fmt.Printf("[CACHE MISS] %s\n", instr.Raw)
			isCacheHit = false

			for _, pattern := range srcPatterns {
				// Copy files from context to current layer working directory
				matches, err := filepath.Glob(filepath.Join(be.ContextDir, pattern))
				if err != nil {
					return nil, fmt.Errorf("line %d: glob error: %w", instr.Line, err)
				}

				if len(matches) == 0 {
					return nil, fmt.Errorf("line %d: no files matched pattern: %s", instr.Line, pattern)
				}

				for _, srcPath := range matches {
					info, err := os.Stat(srcPath)
					if err != nil {
						return nil, err
					}

					if info.IsDir() {
						// Copy directory
						destPath := filepath.Join(currentRootFS, dest)
						if err := utils.CopyRecursive(srcPath, destPath); err != nil {
							return nil, fmt.Errorf("failed to copy dir: %w", err)
						}
					} else {
						// Copy file
						destPath := filepath.Join(currentRootFS, dest)
						os.MkdirAll(filepath.Dir(destPath), 0755)

						srcFile, err := os.Open(srcPath)
						if err != nil {
							return nil, err
						}
						dstFile, err := os.Create(destPath)
						if err != nil {
							srcFile.Close()
							return nil, err
						}

						_, err = os.ReadFile(srcPath)
						if err == nil {
							data, _ := os.ReadFile(srcPath)
							dstFile.Write(data)
						}
						srcFile.Close()
						dstFile.Close()
					}
				}
			}

			// Create layer tar
			layerTempDir := filepath.Join(tempDir, fmt.Sprintf("copy-%d", i))
			os.MkdirAll(layerTempDir, 0755)

			// Create delta tar (for simplicity, we'll just tar the whole rootfs)
			layerPath := filepath.Join(be.StateDir, "layers", "temp")
			digest, size, err := utils.CreateLayerTar(currentRootFS, "/", []string{"."}, layerPath)
			if err != nil {
				return nil, fmt.Errorf("failed to create layer: %w", err)
			}

			// Move to final location
			finalPath := filepath.Join(be.StateDir, "layers", digest)
			os.Rename(layerPath, finalPath)

			// Cache this layer
			if !be.NoCache {
				state.SetCacheKey(be.StateDir, cacheKey, digest)
			}

			layers = append(layers, state.LayerInfo{
				Digest:    digest,
				Size:      size,
				CreatedBy: instr.Raw,
			})

		case "RUN":
			command := instr.Args[0]

			// Calculate cache key
			cacheKey := be.calculateCacheKeyRun(instr, config)

			// Check cache
			if !be.NoCache && isCacheHit == false {
				if cachedDigest, err := state.GetCacheKey(be.StateDir, cacheKey); err == nil && cachedDigest != "" {
					fmt.Printf("[CACHE HIT] %s\n", instr.Raw)

					layerPath := filepath.Join(be.StateDir, "layers", cachedDigest)
					size, err := state.GetLayerSize(be.StateDir, cachedDigest)
					if err != nil {
						return nil, fmt.Errorf("failed to get layer size: %w", err)
					}

					if err := utils.ExtractTar(layerPath, currentRootFS); err != nil {
						return nil, fmt.Errorf("failed to extract layer: %w", err)
					}

					layers = append(layers, state.LayerInfo{
						Digest:    cachedDigest,
						Size:      size,
						CreatedBy: instr.Raw,
					})
					isCacheHit = true
					continue
				}
			}

			// Cache miss - execute RUN
			fmt.Printf("[CACHE MISS] %s\n", instr.Raw)
			isCacheHit = false

			// Execute command in container
			_, err := runtime.BuildRun(currentRootFS, command, config.Env, config.WorkDir)
			if err != nil {
				return nil, fmt.Errorf("line %d: RUN failed: %w", instr.Line, err)
			}

			// Create layer tar
			layerPath := filepath.Join(be.StateDir, "layers", "temp")
			digest, size, err := utils.CreateLayerTar(currentRootFS, "/", []string{"."}, layerPath)
			if err != nil {
				return nil, fmt.Errorf("failed to create layer: %w", err)
			}

			// Move to final location
			finalPath := filepath.Join(be.StateDir, "layers", digest)
			os.Rename(layerPath, finalPath)

			// Cache this layer
			if !be.NoCache {
				state.SetCacheKey(be.StateDir, cacheKey, digest)
			}

			layers = append(layers, state.LayerInfo{
				Digest:    digest,
				Size:      size,
				CreatedBy: instr.Raw,
			})

		case "WORKDIR":
			config.WorkDir = instr.Args[0]
			// Create the directory if it doesn't exist
			workdirPath := filepath.Join(currentRootFS, config.WorkDir)
			os.MkdirAll(workdirPath, 0755)

		case "ENV":
			parts := strings.SplitN(instr.Args[0], "=", 2)
			config.Env[parts[0]] = parts[1]

		case "CMD":
			config.Cmd = instr.Args
		}
	}

	// Create manifest
	manifest := &state.ImageManifest{
		Name:    imageName,
		Tag:     imageTag,
		Created: time.Now().UTC().Format(time.RFC3339),
		Config:  *config,
		Layers:  layers,
	}

	if err := state.SaveManifest(be.StateDir, manifest); err != nil {
		return nil, fmt.Errorf("failed to save manifest: %w", err)
	}

	return &BuildOutput{
		ImageName: imageName,
		ImageTag:  imageTag,
		Digest:    manifest.Digest,
	}, nil
}

// calculateCacheKey creates a deterministic cache key for an instruction
func (be *BuildEngine) calculateCacheKey(instr *parser.Instruction, config *state.ImageConfig, filePaths []string) string {
	hash := sha256.New()

	// Write instruction
	hash.Write([]byte(instr.Raw))

	// Write current config
	hash.Write([]byte(config.WorkDir))
	for _, k := range sortKeys(config.Env) {
		hash.Write([]byte(k + "=" + config.Env[k]))
	}

	// For COPY, hash the source files
	if instr.Type == "COPY" {
		for _, pattern := range filePaths {
			matches, _ := filepath.Glob(filepath.Join(be.ContextDir, pattern))
			sort.Strings(matches)
			for _, match := range matches {
				if fh, err := state.ComputeFileHash(match); err == nil {
					hash.Write([]byte(fh))
				}
			}
		}
	}

	return hex.EncodeToString(hash.Sum(nil))
}

// calculateCacheKeyRun creates a cache key for RUN instructions
func (be *BuildEngine) calculateCacheKeyRun(instr *parser.Instruction, config *state.ImageConfig) string {
	hash := sha256.New()

	// Write instruction
	hash.Write([]byte(instr.Raw))

	// Write current config
	hash.Write([]byte(config.WorkDir))
	for _, k := range sortKeys(config.Env) {
		hash.Write([]byte(k + "=" + config.Env[k]))
	}

	return hex.EncodeToString(hash.Sum(nil))
}

func sortKeys(m map[string]string) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
