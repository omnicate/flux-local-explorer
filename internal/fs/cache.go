package fs

import (
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/kustomize/kyaml/filesys"
)

var _ FileSystem = new(cacheFileSystem)

type cacheFileSystem struct {
	FileSystem

	path  string
	cache FileSystem
}

func Cache(base FileSystem, path string) FileSystem {
	return &cacheFileSystem{
		FileSystem: base,
		path:       path,
		cache:      filesys.MakeFsOnDisk(),
	}
}

func (c cacheFileSystem) cachePath(path string) string {
	return filepath.Join(c.path, path)
}

func (c cacheFileSystem) ReadFile(path string) ([]byte, error) {
	if !c.FileSystem.Exists(path) {
		return nil, os.ErrNotExist
	}
	cp := c.cachePath(path)
	data, err := c.cache.ReadFile(cp)
	if err == nil {
		return data, nil
	}
	data, err = c.FileSystem.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := c.cache.MkdirAll(filepath.Dir(cp)); err != nil {
		return nil, fmt.Errorf("failed to make cache dir: %w", err)
	}
	if err := c.cache.WriteFile(cp, data); err != nil {
		return nil, fmt.Errorf("failed to write cache file: %w", err)
	}
	return data, err
}
