package image

import (
	"encoding/json"
	"fmt"
	"time"

	"docksmith/internal/utils"
)

type Config struct {
	WorkingDir string   `json:"workingDir"`
	Env        []string `json:"env"`
	Cmd        []string `json:"cmd"`
}

type Manifest struct {
	Name    string   `json:"name"`
	Tag     string   `json:"tag"`
	Digest  string   `json:"digest"`
	Created string   `json:"created"`
	Config  Config   `json:"config"`
	Layers  []string `json:"layers"`
}

func NewManifest(name, tag string, cfg Config, layers []string) Manifest {
	return Manifest{
		Name:    name,
		Tag:     tag,
		Digest:  "",
		Created: time.Now().UTC().Format(time.RFC3339),
		Config:  cfg,
		Layers:  append([]string(nil), layers...),
	}
}

func NewManifestWithCreated(name, tag string, cfg Config, layers []string, created string) Manifest {
	return Manifest{
		Name:    name,
		Tag:     tag,
		Digest:  "",
		Created: created,
		Config:  cfg,
		Layers:  append([]string(nil), layers...),
	}
}

func ComputeDigest(m Manifest) (string, error) {
	clone := m
	clone.Digest = ""
	payload, err := json.Marshal(clone)
	if err != nil {
		return "", fmt.Errorf("marshal manifest for digest: %w", err)
	}
	return utils.SHA256Bytes(payload), nil
}

func MarshalWithDigest(m Manifest) ([]byte, Manifest, error) {
	digest, err := ComputeDigest(m)
	if err != nil {
		return nil, Manifest{}, err
	}
	m.Digest = digest
	payload, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, Manifest{}, fmt.Errorf("marshal manifest json: %w", err)
	}
	return payload, m, nil
}
