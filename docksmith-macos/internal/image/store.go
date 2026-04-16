package image

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"docksmith/internal/storage"
)

type Ref struct {
	Name string
	Tag  string
}

type Store struct {
	storage *storage.ImageStorage
}

func NewStore(storeRoot string) (*Store, error) {
	imgStorage, err := storage.NewImageStorage(storeRoot)
	if err != nil {
		return nil, err
	}
	return &Store{storage: imgStorage}, nil
}

func ParseRef(raw string) (Ref, error) {
	parts := strings.Split(raw, ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Ref{}, fmt.Errorf("invalid image reference %q, expected name:tag", raw)
	}
	return Ref{Name: parts[0], Tag: parts[1]}, nil
}

func (s *Store) Save(manifest Manifest) (Manifest, error) {
	payload, withDigest, err := MarshalWithDigest(manifest)
	if err != nil {
		return Manifest{}, err
	}
	if err := s.storage.SaveManifestJSON(withDigest.Digest, payload); err != nil {
		return Manifest{}, err
	}

	tags, err := s.storage.LoadTags()
	if err != nil {
		return Manifest{}, err
	}
	tags[formatRef(withDigest.Name, withDigest.Tag)] = withDigest.Digest
	if err := s.storage.SaveTags(tags); err != nil {
		return Manifest{}, err
	}
	return withDigest, nil
}

func (s *Store) LoadByRef(raw string) (Manifest, error) {
	tags, err := s.storage.LoadTags()
	if err != nil {
		return Manifest{}, err
	}

	// Try exact match first
	digest, ok := tags[raw]
	if !ok {
		// If no exact match and raw doesn't contain ":", try with :latest suffix
		if !strings.Contains(raw, ":") {
			digest, ok = tags[raw+":latest"]
			if !ok {
				return Manifest{}, fmt.Errorf("image %q not found in local store", raw)
			}
		} else {
			return Manifest{}, fmt.Errorf("image %q not found in local store", raw)
		}
	}

	payload, err := s.storage.LoadManifestJSON(digest)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(payload, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest %q: %w", digest, err)
	}
	return m, nil
}

func (s *Store) List() ([]Manifest, error) {
	tags, err := s.storage.LoadTags()
	if err != nil {
		return nil, err
	}

	refs := make([]string, 0, len(tags))
	for ref := range tags {
		refs = append(refs, ref)
	}
	sort.Strings(refs)

	out := make([]Manifest, 0, len(refs))
	for _, ref := range refs {
		m, err := s.LoadByRef(ref)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (s *Store) Remove(raw string) error {
	tags, err := s.storage.LoadTags()
	if err != nil {
		return err
	}
	digest, ok := tags[raw]
	if !ok {
		return fmt.Errorf("image %q not found", raw)
	}
	delete(tags, raw)
	if err := s.storage.SaveTags(tags); err != nil {
		return err
	}

	for _, d := range tags {
		if d == digest {
			return nil
		}
	}
	return s.storage.DeleteManifest(digest)
}

func formatRef(name, tag string) string {
	return name + ":" + tag
}
