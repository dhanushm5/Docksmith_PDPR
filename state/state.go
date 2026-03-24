package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Image manifest structure
type ImageManifest struct {
	Name    string      `json:"name"`
	Tag     string      `json:"tag"`
	Digest  string      `json:"digest"`
	Created string      `json:"created"`
	Config  ImageConfig `json:"config"`
	Layers  []LayerInfo `json:"layers"`
}

type ImageConfig struct {
	Env     map[string]string `json:"env"`
	Cmd     []string          `json:"cmd"`
	WorkDir string            `json:"workdir"`
}

type LayerInfo struct {
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	CreatedBy string `json:"created_by"`
}

func InitStateDir(stateDir string) error {
	dirs := []string{
		stateDir,
		filepath.Join(stateDir, "images"),
		filepath.Join(stateDir, "layers"),
		filepath.Join(stateDir, "cache"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}

	return nil
}

// SaveManifest saves an image manifest and calculates digest
func SaveManifest(stateDir string, manifest *ImageManifest) error {
	// Calculate digest with empty digest field
	manifest.Digest = ""
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}

	hash := sha256.Sum256(data)
	manifest.Digest = hex.EncodeToString(hash[:])

	// Write with final digest
	data, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	filename := filepath.Join(stateDir, "images", fmt.Sprintf("%s_%s.json", manifest.Name, manifest.Tag))
	return os.WriteFile(filename, data, 0644)
}

// LoadManifest loads a manifest by name and tag
func LoadManifest(stateDir, name, tag string) (*ImageManifest, error) {
	filename := filepath.Join(stateDir, "images", fmt.Sprintf("%s_%s.json", name, tag))
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("image not found: %s:%s", name, tag)
	}

	var manifest ImageManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

// ListManifests returns all manifests in the state directory
func ListManifests(stateDir string) ([]*ImageManifest, error) {
	imagesDir := filepath.Join(stateDir, "images")
	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		return nil, err
	}

	var manifests []*ImageManifest
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(imagesDir, entry.Name()))
		if err != nil {
			continue
		}

		var manifest ImageManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue
		}

		manifests = append(manifests, &manifest)
	}

	return manifests, nil
}

// DeleteManifest removes a manifest and optionally its layers
func DeleteManifest(stateDir, name, tag string) error {
	filename := filepath.Join(stateDir, "images", fmt.Sprintf("%s_%s.json", name, tag))
	return os.Remove(filename)
}

// GetCacheKey looks up a layer digest by cache key
func GetCacheKey(stateDir, cacheKey string) (string, error) {
	cacheFile := filepath.Join(stateDir, "cache", cacheKey)
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return "", nil // Cache miss
	}
	return string(data), nil
}

// SetCacheKey saves a cache key mapping to a layer digest
func SetCacheKey(stateDir, cacheKey, digest string) error {
	cacheFile := filepath.Join(stateDir, "cache", cacheKey)
	return os.WriteFile(cacheFile, []byte(digest), 0644)
}

// LayerExists checks if a layer file exists
func LayerExists(stateDir, digest string) bool {
	layerPath := filepath.Join(stateDir, "layers", digest)
	_, err := os.Stat(layerPath)
	return err == nil
}

// GetLayerSize returns the size of a layer
func GetLayerSize(stateDir, digest string) (int64, error) {
	layerPath := filepath.Join(stateDir, "layers", digest)
	info, err := os.Stat(layerPath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// ComputeFileHash computes the SHA-256 hash of a file's content
func ComputeFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// ComputeDataHash computes the SHA-256 hash of data
func ComputeDataHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
