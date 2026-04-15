package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"docksmith/internal/utils"
)

type LayerStore struct {
	root string
}

func NewLayerStore(storeRoot string) (*LayerStore, error) {
	root := filepath.Join(storeRoot, "layers")
	if err := utils.EnsureDir(root); err != nil {
		return nil, err
	}
	return &LayerStore{root: root}, nil
}

func (s *LayerStore) SaveLayer(tarBytes []byte) (string, error) {
	digest := utils.SHA256Bytes(tarBytes)
	path := filepath.Join(s.root, digest)
	if _, err := os.Stat(path); err == nil {
		return digest, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat layer %q: %w", path, err)
	}

	if err := os.WriteFile(path, tarBytes, 0o644); err != nil {
		return "", fmt.Errorf("write layer %q: %w", path, err)
	}
	return digest, nil
}

func (s *LayerStore) OpenLayer(digest string) (io.ReadCloser, error) {
	path := filepath.Join(s.root, digest)
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open layer %q: %w", digest, err)
	}
	return f, nil
}

func (s *LayerStore) ApplyLayers(digests []string, dest string) error {
	for _, digest := range digests {
		r, err := s.OpenLayer(digest)
		if err != nil {
			return err
		}
		if err := utils.ApplyTar(r, dest); err != nil {
			r.Close()
			return fmt.Errorf("apply layer %q: %w", digest, err)
		}
		if err := r.Close(); err != nil {
			return fmt.Errorf("close layer %q: %w", digest, err)
		}
	}
	return nil
}

func (s *LayerStore) LayerExists(digest string) (bool, error) {
	_, err := os.Stat(filepath.Join(s.root, digest))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("stat layer %q: %w", digest, err)
}
