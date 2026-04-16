package build

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"docksmith/internal/image"
	"docksmith/internal/storage"
	"docksmith/internal/utils"
)

type BuildOptions struct {
	ContextDir string
	Tag        string
	NoCache    bool
}

type Engine struct {
	layerStore *storage.LayerStore
	imageStore *image.Store
	cache      *Cache
}

func NewEngine(storeRoot string) (*Engine, error) {
	layerStore, err := storage.NewLayerStore(storeRoot)
	if err != nil {
		return nil, err
	}
	imageStore, err := image.NewStore(storeRoot)
	if err != nil {
		return nil, err
	}
	cache, err := NewCache(storeRoot)
	if err != nil {
		return nil, err
	}
	return &Engine{layerStore: layerStore, imageStore: imageStore, cache: cache}, nil
}

func (e *Engine) Build(opts BuildOptions) (image.Manifest, error) {
	buildStart := time.Now()
	if opts.Tag == "" {
		return image.Manifest{}, fmt.Errorf("missing build tag, expected name:tag")
	}
	ref, err := image.ParseRef(opts.Tag)
	if err != nil {
		return image.Manifest{}, err
	}
	parsed, err := ParseDocksmithfile(opts.ContextDir)
	if err != nil {
		return image.Manifest{}, err
	}

	state := image.Config{WorkingDir: "/", Env: []string{}, Cmd: []string{}}
	envMap := map[string]string{}
	layers := []string{}
	cacheBroken := opts.NoCache

	for i, ins := range parsed.Instructions {
		stepStart := time.Now()
		fmt.Printf("Step %d/%d : %s\n", i+1, len(parsed.Instructions), ins.Raw)
		switch ins.Op {
		case "FROM":
			if i != 0 {
				return image.Manifest{}, fmt.Errorf("FROM must be first instruction")
			}
			base := ins.Args[0]
			if base == "scratch" {
				layers = nil
				state = image.Config{WorkingDir: "/", Env: []string{}, Cmd: []string{}}
				envMap = map[string]string{}
				fmt.Printf("%s\n", formatStepTiming(stepStart, ""))
				continue
			}
			baseManifest, err := e.imageStore.LoadByRef(base)
			if err != nil {
				if runtime.GOOS == "linux" {
					return image.Manifest{}, fmt.Errorf("FROM %q failed: %w", base, err)
				}
				// On non-Linux hosts we allow demo builds by treating missing base as scratch.
				layers = nil
				state = image.Config{WorkingDir: "/", Env: []string{}, Cmd: []string{}}
				envMap = map[string]string{}
				fmt.Printf("%s\n", formatStepTiming(stepStart, ""))
				continue
			}
			layers = append([]string(nil), baseManifest.Layers...)
			state = baseManifest.Config
			envMap, err = ParseEnvPairs(state.Env)
			if err != nil {
				return image.Manifest{}, err
			}
			fmt.Printf("%s\n", formatStepTiming(stepStart, ""))

		case "WORKDIR":
			state.WorkingDir = normalizeWorkDir(state.WorkingDir, ins.Args[0])
			fmt.Printf("%s\n", formatStepTiming(stepStart, ""))

		case "ENV":
			if len(ins.Args) == 1 && strings.Contains(ins.Args[0], "=") {
				k, v, _ := strings.Cut(ins.Args[0], "=")
				envMap[k] = v
			} else if len(ins.Args) >= 2 {
				envMap[ins.Args[0]] = strings.Join(ins.Args[1:], " ")
			} else {
				return image.Manifest{}, fmt.Errorf("invalid ENV instruction %q", ins.Raw)
			}
			state.Env = utils.SortedEnvKV(envMap)
			fmt.Printf("%s\n", formatStepTiming(stepStart, ""))

		case "CMD":
			cmdRaw := strings.TrimSpace(ins.Args[0])
			if strings.HasPrefix(cmdRaw, "[") {
				var parts []string
				if err := json.Unmarshal([]byte(cmdRaw), &parts); err != nil {
					return image.Manifest{}, fmt.Errorf("invalid CMD json array %q: %w", cmdRaw, err)
				}
				state.Cmd = parts
			} else {
				state.Cmd = []string{"/bin/sh", "-c", cmdRaw}
			}
			fmt.Printf("%s\n", formatStepTiming(stepStart, ""))

		case "COPY":
			if len(ins.Args) != 2 {
				return image.Manifest{}, fmt.Errorf("COPY requires exactly 2 arguments")
			}
			layerDigest, miss, cacheLabel, err := e.executeCopy(ins, opts.ContextDir, state.WorkingDir, envMap, layers, cacheBroken)
			if err != nil {
				return image.Manifest{}, err
			}
			if miss {
				cacheBroken = true
			}
			layers = append(layers, layerDigest)
			fmt.Printf("%s\n", formatStepTiming(stepStart, cacheLabel))

		case "RUN":
			layerDigest, miss, cacheLabel, err := e.executeRun(ins, state.WorkingDir, envMap, layers, cacheBroken)
			if err != nil {
				return image.Manifest{}, err
			}
			if miss {
				cacheBroken = true
			}
			layers = append(layers, layerDigest)
			fmt.Printf("%s\n", formatStepTiming(stepStart, cacheLabel))
		}
	}

	state.Env = utils.SortedEnvKV(envMap)

	// Try to load existing manifest to preserve timestamp on cache hits
	var created string
	oldManifest, err := e.imageStore.LoadByRef(ref.Name + ":" + ref.Tag)
	if err == nil && layersEqual(oldManifest.Layers, layers) {
		// All cache hits - preserve original timestamp
		created = oldManifest.Created
	} else {
		// New build - generate new timestamp
		created = time.Now().UTC().Format(time.RFC3339)
	}

	manifest := image.NewManifestWithCreated(ref.Name, ref.Tag, state, layers, created)
	stored, err := e.imageStore.Save(manifest)
	if err != nil {
		return image.Manifest{}, err
	}
	fmt.Printf("Successfully built sha256:%s %s:%s (%.2fs)\n", stored.Digest, stored.Name, stored.Tag, time.Since(buildStart).Seconds())
	return stored, nil
}

func (e *Engine) executeCopy(ins Instruction, contextDir, workDir string, env map[string]string, layers []string, cacheBroken bool) (string, bool, string, error) {
	srcPath, err := resolveCopySource(contextDir, ins.Args[0])
	if err != nil {
		return "", false, "", err
	}
	copyHashes, err := collectCopyHashes(srcPath)
	if err != nil {
		return "", false, "", err
	}

	cacheKey := ComputeCacheKey(lastLayerDigest(layers), ins.Raw, workDir, env, copyHashes)
	if !cacheBroken {
		if cached, ok := e.cache.Lookup(cacheKey); ok {
			return cached, false, "[CACHE HIT]", nil
		}
	}

	layerTar, _, err := CreateCopyLayer(contextDir, ins.Args[0], ins.Args[1], workDir)
	if err != nil {
		return "", false, "", err
	}
	layerDigest, err := e.layerStore.SaveLayer(layerTar)
	if err != nil {
		return "", false, "", err
	}
	if err := e.cache.Store(cacheKey, layerDigest); err != nil {
		return "", false, "", err
	}
	return layerDigest, true, "[CACHE MISS]", nil
}

func (e *Engine) executeRun(ins Instruction, workDir string, env map[string]string, layers []string, cacheBroken bool) (string, bool, string, error) {
	cacheKey := ComputeCacheKey(lastLayerDigest(layers), ins.Raw, workDir, env, nil)
	if !cacheBroken {
		if cached, ok := e.cache.Lookup(cacheKey); ok {
			return cached, false, "[CACHE HIT]", nil
		}
	}

	rootfs, err := os.MkdirTemp("", "docksmith-run-buildfs-*")
	if err != nil {
		return "", false, "", fmt.Errorf("create temp build fs: %w", err)
	}
	defer os.RemoveAll(rootfs)

	if err := e.layerStore.ApplyLayers(layers, rootfs); err != nil {
		return "", false, "", err
	}
	before, err := SnapshotFS(rootfs)
	if err != nil {
		return "", false, "", err
	}

	exitCode, err := ExecuteRunInstruction(rootfs, workDir, env, ins.Args[0])
	if err != nil {
		return "", false, "", err
	}
	if exitCode != 0 {
		return "", false, "", fmt.Errorf("RUN failed with exit code %d", exitCode)
	}

	after, err := SnapshotFS(rootfs)
	if err != nil {
		return "", false, "", err
	}
	changed, deleted := DiffSnapshots(before, after)
	deltaTar, err := CreateRunDeltaTar(rootfs, changed, deleted)
	if err != nil {
		return "", false, "", err
	}
	layerDigest, err := e.layerStore.SaveLayer(deltaTar)
	if err != nil {
		return "", false, "", err
	}
	if err := e.cache.Store(cacheKey, layerDigest); err != nil {
		return "", false, "", err
	}
	return layerDigest, true, "[CACHE MISS]", nil
}

func formatStepTiming(stepStart time.Time, cacheLabel string) string {
	elapsed := time.Since(stepStart).Seconds()
	if cacheLabel == "" {
		return fmt.Sprintf("%.2fs", elapsed)
	}
	return fmt.Sprintf("%s %.2fs", cacheLabel, elapsed)
}

func lastLayerDigest(layers []string) string {
	if len(layers) == 0 {
		return ""
	}
	return layers[len(layers)-1]
}

func layersEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func normalizeWorkDir(current, next string) string {
	next = strings.TrimSpace(next)
	if strings.HasPrefix(next, "/") {
		return path.Clean(next)
	}
	if current == "" {
		current = "/"
	}
	return path.Clean(path.Join(current, next))
}
