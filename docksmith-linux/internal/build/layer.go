package build

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"docksmith/internal/utils"
)

func CreateCopyLayer(contextDir, srcArg, destArg, workDir string) ([]byte, []string, error) {
	srcPath, err := resolveCopySource(contextDir, srcArg)
	if err != nil {
		return nil, nil, err
	}
	copyHashes, err := collectCopyHashes(srcPath)
	if err != nil {
		return nil, nil, err
	}

	target := normalizeContainerPath(destArg, workDir)
	stageRoot, err := os.MkdirTemp("", "docksmith-copy-layer-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create temp copy stage: %w", err)
	}
	defer os.RemoveAll(stageRoot)

	if err := materializeCopy(stageRoot, srcPath, target); err != nil {
		return nil, nil, err
	}
	rels, err := listAllStageEntries(stageRoot)
	if err != nil {
		return nil, nil, err
	}

	layerTar, err := utils.CreateDeterministicTar(stageRoot, rels)
	if err != nil {
		return nil, nil, err
	}
	return layerTar, copyHashes, nil
}

func CreateRunDeltaTar(rootfs string, changed, deleted []string) ([]byte, error) {
	sort.Strings(changed)
	sort.Strings(deleted)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for _, rel := range deleted {
		clean := path.Clean(filepath.ToSlash(rel))
		if clean == "." || clean == "" {
			continue
		}
		hdr := &tar.Header{
			Name:       path.Join(path.Dir(clean), ".wh."+path.Base(clean)),
			Typeflag:   tar.TypeReg,
			Mode:       0o644,
			Size:       0,
			ModTime:    time.Unix(0, 0),
			AccessTime: time.Unix(0, 0),
			ChangeTime: time.Unix(0, 0),
			Uid:        0,
			Gid:        0,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, fmt.Errorf("write whiteout %q: %w", clean, err)
		}
	}

	for _, rel := range changed {
		clean := path.Clean(filepath.ToSlash(rel))
		if clean == "." || clean == "" {
			continue
		}
		full := filepath.Join(rootfs, filepath.FromSlash(clean))
		info, err := os.Lstat(full)
		if err != nil {
			return nil, fmt.Errorf("stat changed path %q: %w", full, err)
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return nil, fmt.Errorf("create tar header %q: %w", clean, err)
		}
		hdr.Name = clean
		hdr.ModTime = time.Unix(0, 0)
		hdr.AccessTime = time.Unix(0, 0)
		hdr.ChangeTime = time.Unix(0, 0)
		hdr.Uid = 0
		hdr.Gid = 0
		if info.IsDir() && !strings.HasSuffix(hdr.Name, "/") {
			hdr.Name += "/"
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(full)
			if err != nil {
				return nil, fmt.Errorf("readlink %q: %w", full, err)
			}
			hdr.Linkname = target
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, fmt.Errorf("write changed header %q: %w", clean, err)
		}
		if info.Mode().IsRegular() {
			f, err := os.Open(full)
			if err != nil {
				return nil, fmt.Errorf("open changed file %q: %w", full, err)
			}
			if _, err := io.Copy(tw, f); err != nil {
				f.Close()
				return nil, fmt.Errorf("write changed file %q: %w", clean, err)
			}
			if err := f.Close(); err != nil {
				return nil, fmt.Errorf("close changed file %q: %w", clean, err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("close run delta tar: %w", err)
	}
	return buf.Bytes(), nil
}

func SnapshotFS(root string) (map[string]string, error) {
	snap := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
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
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			snap[rel] = "DIR"
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			snap[rel] = "SYMLINK:" + target
			return nil
		}
		h, err := utils.SHA256File(p)
		if err != nil {
			return err
		}
		snap[rel] = "FILE:" + h
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("snapshot filesystem %q: %w", root, err)
	}
	return snap, nil
}

func DiffSnapshots(before, after map[string]string) (changed, deleted []string) {
	for p, oldVal := range before {
		newVal, ok := after[p]
		if !ok {
			deleted = append(deleted, p)
			continue
		}
		if newVal != oldVal {
			changed = append(changed, p)
		}
	}
	for p := range after {
		if _, ok := before[p]; !ok {
			changed = append(changed, p)
		}
	}
	sort.Strings(changed)
	sort.Strings(deleted)
	return changed, deleted
}

func resolveCopySource(contextDir, srcArg string) (string, error) {
	clean := filepath.Clean(srcArg)
	abs := filepath.Join(contextDir, clean)
	ctxAbs, err := filepath.Abs(contextDir)
	if err != nil {
		return "", fmt.Errorf("resolve context path: %w", err)
	}
	absSrc, err := filepath.Abs(abs)
	if err != nil {
		return "", fmt.Errorf("resolve COPY source %q: %w", srcArg, err)
	}
	if absSrc != ctxAbs && !strings.HasPrefix(absSrc, ctxAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("COPY source %q escapes build context", srcArg)
	}
	if _, err := os.Stat(absSrc); err != nil {
		return "", fmt.Errorf("COPY source %q not found: %w", srcArg, err)
	}
	return absSrc, nil
}

func normalizeContainerPath(destArg, workDir string) string {
	d := filepath.ToSlash(destArg)
	if strings.HasPrefix(d, "/") {
		return path.Clean(d)
	}
	wd := workDir
	if wd == "" {
		wd = "/"
	}
	return path.Clean(path.Join(wd, d))
}

func collectCopyHashes(srcPath string) ([]string, error) {
	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, fmt.Errorf("stat COPY source %q: %w", srcPath, err)
	}
	if !info.IsDir() {
		h, err := utils.SHA256File(srcPath)
		if err != nil {
			return nil, err
		}
		return []string{filepath.Base(srcPath) + "=" + h}, nil
	}
	var out []string
	err = filepath.WalkDir(srcPath, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		h, err := utils.SHA256File(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcPath, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel)+"="+h)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("hash COPY source tree %q: %w", srcPath, err)
	}
	sort.Strings(out)
	return out, nil
}

func materializeCopy(stageRoot, srcPath, target string) error {
	info, err := os.Stat(srcPath)
	if err != nil {
		return err
	}
	target = strings.TrimPrefix(path.Clean(target), "/")
	if target == "." {
		target = ""
	}
	if !info.IsDir() {
		dst := filepath.Join(stageRoot, filepath.FromSlash(target))
		if strings.HasSuffix(target, "/") || target == "" {
			dst = filepath.Join(stageRoot, filepath.FromSlash(path.Join(target, filepath.Base(srcPath))))
		}
		return utils.CopyFile(srcPath, dst, info.Mode().Perm())
	}

	return filepath.WalkDir(srcPath, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcPath, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		dst := filepath.Join(stageRoot, filepath.FromSlash(path.Join(target, filepath.ToSlash(rel))))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return utils.CopyFile(p, dst, info.Mode().Perm())
	})
}

func listAllStageEntries(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
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
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk copy stage: %w", err)
	}
	sort.Strings(out)
	return out, nil
}
