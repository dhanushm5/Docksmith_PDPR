package build

import (
	"fmt"
	"sort"
	"strings"

	"docksmith/internal/storage"
	"docksmith/internal/utils"
)

type Cache struct {
	storage *storage.CacheStorage
	index   map[string]string
}

func NewCache(storeRoot string) (*Cache, error) {
	cacheStorage, err := storage.NewCacheStorage(storeRoot)
	if err != nil {
		return nil, err
	}
	index, err := cacheStorage.LoadIndex()
	if err != nil {
		return nil, err
	}
	return &Cache{storage: cacheStorage, index: index}, nil
}

func (c *Cache) Lookup(key string) (string, bool) {
	digest, ok := c.index[key]
	return digest, ok
}

func (c *Cache) Store(key, layerDigest string) error {
	c.index[key] = layerDigest
	return c.storage.SaveIndex(c.index)
}

func ComputeCacheKey(prevLayerDigest, instructionText, workDir string, env map[string]string, copyFileHashes []string) string {
	envPairs := utils.SortedEnvKV(env)
	copyHashes := append([]string(nil), copyFileHashes...)
	sort.Strings(copyHashes)

	payload := strings.Join([]string{
		"prev=" + prevLayerDigest,
		"ins=" + instructionText,
		"wd=" + workDir,
		"env=" + strings.Join(envPairs, ";"),
		"copy=" + strings.Join(copyHashes, ";"),
	}, "\n")
	return utils.SHA256String(payload)
}

func ParseEnvPairs(env []string) (map[string]string, error) {
	out := map[string]string{}
	for _, pair := range env {
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("invalid env pair %q", pair)
		}
		out[k] = v
	}
	return out, nil
}
