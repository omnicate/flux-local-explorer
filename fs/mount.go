package fs

import (
	"path/filepath"
	"sort"
	"strings"
)

var _ FileSystem = new(mountFileSystem)

type MountPoint struct {
	Location string
	Path     string
	FS       FileSystem
}

type mountFileSystem struct {
	FileSystem

	mounts []*MountPoint
}

func Mount(fs FileSystem, mounts ...*MountPoint) FileSystem {
	return &mountFileSystem{
		FileSystem: fs,
		mounts:     mounts,
	}
}

func (m mountFileSystem) mountedPath(path string) (FileSystem, string) {
	for _, mp := range m.mounts {
		relPath, err := filepath.Rel(mp.Location, path)
		if err == nil && !strings.HasPrefix(relPath, "..") {
			return mp.FS, filepath.Join(mp.Path, relPath)
		}
	}
	return m.FileSystem, path
}

func (m mountFileSystem) IsDir(path string) bool {
	fs, p := m.mountedPath(path)
	return fs.IsDir(p)
}

func (m mountFileSystem) ReadDir(path string) ([]string, error) {
	fs, p := m.mountedPath(path)
	entries, err := fs.ReadDir(p)
	if err != nil {
		return nil, err
	}
	for _, mp := range m.mounts {
		relPath, err := filepath.Rel(mp.Location, path)
		if err == nil && (relPath == "../." || relPath == "..") {
			entries = append(entries, filepath.Base(mp.Location)+"/")
		}
	}
	sort.Strings(entries)
	return entries, nil
}

func (m mountFileSystem) Exists(path string) bool {
	fs, p := m.mountedPath(path)
	return fs.Exists(p)
}

func (m mountFileSystem) ReadFile(path string) ([]byte, error) {
	fs, p := m.mountedPath(path)
	return fs.ReadFile(p)
}
