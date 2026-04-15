package utils

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func CreateDeterministicTar(root string, relPaths []string) ([]byte, error) {
	sorted := append([]string(nil), relPaths...)
	sort.Strings(sorted)

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, rel := range sorted {
		clean := strings.TrimPrefix(path.Clean(filepath.ToSlash(rel)), "./")
		if clean == "." || clean == "" {
			continue
		}
		fullPath := filepath.Join(root, filepath.FromSlash(clean))
		info, err := os.Lstat(fullPath)
		if err != nil {
			return nil, fmt.Errorf("stat tar path %q: %w", fullPath, err)
		}
		if err := writeDeterministicEntry(tw, fullPath, clean, info); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("close tar writer: %w", err)
	}
	return buf.Bytes(), nil
}

func writeDeterministicEntry(tw *tar.Writer, fullPath, name string, info fs.FileInfo) error {
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("create tar header for %q: %w", fullPath, err)
	}
	header.Name = path.Clean(filepath.ToSlash(name))
	header.ModTime = zeroTime()
	header.AccessTime = zeroTime()
	header.ChangeTime = zeroTime()
	header.Uid = 0
	header.Gid = 0
	header.Uname = ""
	header.Gname = ""

	if info.IsDir() && !strings.HasSuffix(header.Name, "/") {
		header.Name += "/"
	}

	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(fullPath)
		if err != nil {
			return fmt.Errorf("readlink %q: %w", fullPath, err)
		}
		header.Linkname = target
	}

	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write tar header for %q: %w", fullPath, err)
	}
	if info.Mode().IsRegular() {
		f, err := os.Open(fullPath)
		if err != nil {
			return fmt.Errorf("open regular file %q: %w", fullPath, err)
		}
		defer f.Close()
		if _, err := io.Copy(tw, f); err != nil {
			return fmt.Errorf("write regular file %q to tar: %w", fullPath, err)
		}
	}
	return nil
}

func ApplyTar(r io.Reader, dest string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		name := path.Clean(hdr.Name)
		if name == "." {
			// Some archives include a root marker entry ("./"). Ignore it.
			continue
		}
		if strings.HasPrefix(name, "../") || strings.Contains(name, "/../") {
			return fmt.Errorf("unsafe tar path %q", hdr.Name)
		}

		if path.Base(name) != ".wh..wh..opq" && strings.HasPrefix(path.Base(name), ".wh.") {
			target := path.Join(path.Dir(name), strings.TrimPrefix(path.Base(name), ".wh."))
			if err := os.RemoveAll(filepath.Join(dest, filepath.FromSlash(target))); err != nil {
				return fmt.Errorf("apply whiteout remove %q: %w", target, err)
			}
			continue
		}

		full := filepath.Join(dest, filepath.FromSlash(name))
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(full, os.FileMode(hdr.Mode)); err != nil {
				return fmt.Errorf("create directory %q: %w", full, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := EnsureDir(filepath.Dir(full)); err != nil {
				return err
			}
			out, err := os.OpenFile(full, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				return fmt.Errorf("open extracted file %q: %w", full, err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return fmt.Errorf("write extracted file %q: %w", full, err)
			}
			if err := out.Close(); err != nil {
				return fmt.Errorf("close extracted file %q: %w", full, err)
			}
		case tar.TypeSymlink:
			if err := EnsureDir(filepath.Dir(full)); err != nil {
				return err
			}
			if err := os.RemoveAll(full); err != nil {
				return fmt.Errorf("replace symlink path %q: %w", full, err)
			}
			if err := os.Symlink(hdr.Linkname, full); err != nil {
				return fmt.Errorf("create symlink %q -> %q: %w", full, hdr.Linkname, err)
			}
		default:
			return fmt.Errorf("unsupported tar entry type %v for %q", hdr.Typeflag, hdr.Name)
		}
	}
}

func zeroTime() time.Time {
	return time.Unix(0, 0).UTC()
}
