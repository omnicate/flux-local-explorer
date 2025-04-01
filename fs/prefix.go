package fs

import (
	"path/filepath"
)

var _ FileSystem = new(prefixFileSystem)

type prefixFileSystem struct {
	readOnlyFileSystem

	base   FileSystem
	prefix string
}

func Prefix(base FileSystem, prefix string) FileSystem {
	return &prefixFileSystem{
		base:   base,
		prefix: prefix,
	}
}

func (p prefixFileSystem) IsDir(path string) bool {
	path = filepath.Join(p.prefix, path)
	return p.base.IsDir(path)
}

func (p prefixFileSystem) ReadDir(path string) ([]string, error) {
	path = filepath.Join(p.prefix, path)
	return p.base.ReadDir(path)
}

func (p prefixFileSystem) Exists(path string) bool {
	path = filepath.Join(p.prefix, path)
	return p.base.Exists(path)
}

func (p prefixFileSystem) ReadFile(path string) ([]byte, error) {
	path = filepath.Join(p.prefix, path)
	return p.base.ReadFile(path)
}
