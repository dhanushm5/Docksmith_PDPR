package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"docksmith/internal/utils"
)

type ImageStorage struct {
	root string
}

func NewImageStorage(storeRoot string) (*ImageStorage, error) {
	root := filepath.Join(storeRoot, "images")
	if err := utils.EnsureDir(root); err != nil {
		return nil, err
	}
	return &ImageStorage{root: root}, nil
}

func (s *ImageStorage) ManifestPath(digest string) string {
	return filepath.Join(s.root, digest+".json")
}

func (s *ImageStorage) tagsPath() string {
	return filepath.Join(s.root, "tags.json")
}

func (s *ImageStorage) SaveManifestJSON(digest string, payload []byte) error {
	if err := os.WriteFile(s.ManifestPath(digest), payload, 0o644); err != nil {
		return fmt.Errorf("write manifest %q: %w", digest, err)
	}
	return nil
}

func (s *ImageStorage) LoadManifestJSON(digest string) ([]byte, error) {
	payload, err := os.ReadFile(s.ManifestPath(digest))
	if err != nil {
		return nil, fmt.Errorf("read manifest %q: %w", digest, err)
	}
	return payload, nil
}

func (s *ImageStorage) LoadTags() (map[string]string, error) {
	path := s.tagsPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return map[string]string{}, nil
	}

	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tag index: %w", err)
	}

	var tags map[string]string
	if err := json.Unmarshal(payload, &tags); err != nil {
		return nil, fmt.Errorf("parse tag index: %w", err)
	}
	if tags == nil {
		return map[string]string{}, nil
	}
	return tags, nil
}

func (s *ImageStorage) SaveTags(tags map[string]string) error {
	payload, err := json.MarshalIndent(tags, "", "  ")
	if err != nil {
		return fmt.Errorf("encode tag index: %w", err)
	}
	if err := os.WriteFile(s.tagsPath(), payload, 0o644); err != nil {
		return fmt.Errorf("write tag index: %w", err)
	}
	return nil
}

func (s *ImageStorage) DeleteManifest(digest string) error {
	if err := os.Remove(s.ManifestPath(digest)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete manifest %q: %w", digest, err)
	}
	return nil
}
