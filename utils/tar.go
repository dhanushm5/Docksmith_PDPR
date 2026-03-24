package utils

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CreateLayerTar creates a tar archive from source files, storing only modified/new files
// Returns the tar digest and size
func CreateLayerTar(sourceDir, destDir string, filePaths []string, tarPath string) (string, int64, error) {
	file, err := os.Create(tarPath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create tar file: %w", err)
	}
	defer file.Close()

	// Use a hash writer to compute digest while writing
	hashWriter := sha256.New()
	multiWriter := io.MultiWriter(file, hashWriter)

	tw := tar.NewWriter(multiWriter)
	defer tw.Close()

	// Collect all files to add
	filesToAdd := make(map[string]string) // relative path -> full path

	for _, pattern := range filePaths {
		matches, err := filepath.Glob(filepath.Join(sourceDir, pattern))
		if err != nil {
			return "", 0, fmt.Errorf("glob pattern error for %s: %w", pattern, err)
		}

		for _, match := range matches {
			relPath, err := filepath.Rel(sourceDir, match)
			if err != nil {
				continue
			}
			// Ensure forward slashes for tarball
			relPath = filepath.ToSlash(relPath)
			filesToAdd[relPath] = match
		}
	}

	// Sort by relative path for deterministic tar
	var sortedPaths []string
	for path := range filesToAdd {
		sortedPaths = append(sortedPaths, path)
	}
	sort.Strings(sortedPaths)

	// Add files to tar
	for _, relPath := range sortedPaths {
		fullPath := filesToAdd[relPath]

		info, err := os.Stat(fullPath)
		if err != nil {
			return "", 0, fmt.Errorf("failed to stat %s: %w", fullPath, err)
		}

		if info.IsDir() {
			// Add directory header
			header := &tar.Header{
				Name:     relPath + "/",
				Mode:     0755,
				ModTime:  time.Unix(0, 0),
				Typeflag: tar.TypeDir,
			}
			if err := tw.WriteHeader(header); err != nil {
				return "", 0, err
			}
		} else {
			// Add file
			f, err := os.Open(fullPath)
			if err != nil {
				return "", 0, fmt.Errorf("failed to open %s: %w", fullPath, err)
			}

			header := &tar.Header{
				Name:    relPath,
				Size:    info.Size(),
				Mode:    0644,
				ModTime: time.Unix(0, 0),
			}

			if err := tw.WriteHeader(header); err != nil {
				f.Close()
				return "", 0, err
			}

			if _, err := io.Copy(tw, f); err != nil {
				f.Close()
				return "", 0, err
			}
			f.Close()
		}
	}

	if err := tw.Close(); err != nil {
		return "", 0, err
	}

	if err := file.Close(); err != nil {
		return "", 0, err
	}

	// Get file size
	info, err := os.Stat(tarPath)
	if err != nil {
		return "", 0, err
	}

	digest := hex.EncodeToString(hashWriter.Sum(nil))
	return digest, info.Size(), nil
}

// ExtractTar extracts a tar archive to a destination directory
func ExtractTar(tarPath, destDir string) error {
	file, err := os.Open(tarPath)
	if err != nil {
		return fmt.Errorf("failed to open tar: %w", err)
	}
	defer file.Close()

	tr := tar.NewReader(file)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read error: %w", err)
		}

		target := filepath.Join(destDir, header.Name)
		target = filepath.Clean(target)

		// Prevent path traversal
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) &&
			target != filepath.Clean(destDir) {
			return fmt.Errorf("tar contains invalid path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

		case tar.TypeReg:
			// Create parent directories
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create parent dir: %w", err)
			}

			f, err := os.Create(target)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}

			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return fmt.Errorf("failed to extract file: %w", err)
			}
			f.Close()

			if err := os.Chmod(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to set permissions: %w", err)
			}
		}
	}

	return nil
}

// ComputeGlobFilesHash computes hash of all files matching glob patterns
func ComputeGlobFilesHash(baseDir string, patterns []string) (string, error) {
	hash := sha256.New()
	var allPaths []string

	// Collect all matching files
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(baseDir, pattern))
		if err != nil {
			return "", err
		}
		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil || info.IsDir() {
				continue
			}
			allPaths = append(allPaths, match)
		}
	}

	// Sort for determinism
	sort.Strings(allPaths)

	// Hash file contents in order
	for _, path := range allPaths {
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(hash, f); err != nil {
			f.Close()
			return "", err
		}
		f.Close()
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// CopyRecursive copies a directory recursively
func CopyRecursive(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		dstFile, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer dstFile.Close()

		_, err = io.Copy(dstFile, srcFile)
		return err
	})
}
