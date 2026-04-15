package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"docksmith/internal/utils"
)

type CacheStorage struct {
	root string
}

func NewCacheStorage(storeRoot string) (*CacheStorage, error) {
	root := filepath.Join(storeRoot, "cache")
	if err := utils.EnsureDir(root); err != nil {
		return nil, err
	}
	return &CacheStorage{root: root}, nil
}

func (s *CacheStorage) indexPath() string {
	return filepath.Join(s.root, "index.json")
}

func (s *CacheStorage) LoadIndex() (map[string]string, error) {
	path := s.indexPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return map[string]string{}, nil
	}

	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read cache index: %w", err)
	}

	var index map[string]string
	if err := json.Unmarshal(payload, &index); err != nil {
		return nil, fmt.Errorf("parse cache index: %w", err)
	}
	if index == nil {
		return map[string]string{}, nil
	}
	return index, nil
}

func (s *CacheStorage) SaveIndex(index map[string]string) error {
	payload, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("encode cache index: %w", err)
	}
	if err := os.WriteFile(s.indexPath(), payload, 0o644); err != nil {
		return fmt.Errorf("write cache index: %w", err)
	}
	return nil
}
