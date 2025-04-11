package fs

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"sigs.k8s.io/kustomize/kyaml/filesys"
)

var _ filesys.FileSystem = krustyFileSystem{}

type ReadOnlyFileSystem interface {
}

type FileSystem interface {
	IsDir(path string) bool
	ReadDir(path string) ([]string, error)
	Exists(path string) bool
	ReadFile(path string) ([]byte, error)
	Create(path string) (filesys.File, error)
	MkdirAll(path string) error
	RemoveAll(path string) error
	WriteFile(path string, data []byte) error
}

type krustyFileSystem struct {
	fs   FileSystem
	osFS filesys.FileSystem
}

func KrustyFileSystem(fs FileSystem) filesys.FileSystem {
	return krustyFileSystem{fs: fs, osFS: filesys.MakeFsOnDisk()}
}

func (k krustyFileSystem) Create(path string) (filesys.File, error) {
	if filepath.IsAbs(path) {
		return k.osFS.Create(path)
	}
	return k.fs.Create(path)
}

func (k krustyFileSystem) Mkdir(path string) error {
	if filepath.IsAbs(path) {
		return k.osFS.Mkdir(path)
	}
	return k.MkdirAll(path)
}

func (k krustyFileSystem) MkdirAll(path string) error {
	if filepath.IsAbs(path) {
		return k.osFS.MkdirAll(path)
	}
	return k.fs.MkdirAll(path)
}

func (k krustyFileSystem) RemoveAll(path string) error {
	if filepath.IsAbs(path) {
		return k.osFS.RemoveAll(path)
	}
	return k.fs.RemoveAll(path)
}

func (k krustyFileSystem) Open(path string) (filesys.File, error) {
	if filepath.IsAbs(path) {
		return k.osFS.Open(path)
	}
	return nil, fmt.Errorf("not implemented")
}

func (k krustyFileSystem) IsDir(path string) bool {
	if filepath.IsAbs(path) {
		return k.osFS.IsDir(path)
	}
	return k.fs.IsDir(path)
}

func (k krustyFileSystem) ReadDir(path string) ([]string, error) {
	if filepath.IsAbs(path) {
		return k.osFS.ReadDir(path)
	}
	return k.fs.ReadDir(path)
}

func (k krustyFileSystem) CleanedAbs(path string) (filesys.ConfirmedDir, string, error) {
	if k.IsDir(path) {
		return filesys.ConfirmedDir(path), "", nil
	}
	return filesys.ConfirmedDir(filepath.Dir(path)), filepath.Base(path), nil
}

func (k krustyFileSystem) Exists(path string) bool {
	if filepath.IsAbs(path) {
		return k.osFS.Exists(path)
	}
	return k.fs.Exists(path)
}

func (k krustyFileSystem) Glob(pattern string) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func (k krustyFileSystem) ReadFile(path string) ([]byte, error) {
	if filepath.IsAbs(path) {
		return k.osFS.ReadFile(path)
	}
	return k.fs.ReadFile(path)
}

func (k krustyFileSystem) WriteFile(path string, data []byte) error {
	if filepath.IsAbs(path) {
		return k.osFS.WriteFile(path, data)
	}
	return k.fs.WriteFile(path, data)
}

func (k krustyFileSystem) Walk(path string, walkFn filepath.WalkFunc) error {
	if !k.IsDir(path) {
		return walkFn(path, staticFileInfo{
			name:    filepath.Base(path),
			mode:    fs.ModePerm,
			modTime: time.Now(),
			isDir:   false,
		}, nil)
	}
	if err := walkFn(path, staticFileInfo{
		name:    filepath.Base(path),
		mode:    fs.ModeDir,
		modTime: time.Now(),
		isDir:   true,
	}, nil); err != nil {
		return err
	}
	entries, err := k.ReadDir(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := k.Walk(filepath.Join(path, entry), walkFn); err != nil {
			return err
		}
	}
	return nil
}

var (
	ErrReadOnly = errors.New("read-only fs")
)

type readOnlyFileSystem struct {
}

func (r readOnlyFileSystem) Create(path string) (filesys.File, error) {
	return nil, ErrReadOnly
}

func (r readOnlyFileSystem) MkdirAll(path string) error {
	return ErrReadOnly
}

func (r readOnlyFileSystem) RemoveAll(path string) error {
	return ErrReadOnly
}

func (r readOnlyFileSystem) WriteFile(path string, data []byte) error {
	return ErrReadOnly
}

type staticFileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
	isDir   bool
}

func (g staticFileInfo) Name() string {
	return g.name
}

func (g staticFileInfo) Size() int64 {
	return g.size
}

func (g staticFileInfo) Mode() fs.FileMode {
	return g.mode
}

func (g staticFileInfo) ModTime() time.Time {
	return g.modTime
}

func (g staticFileInfo) IsDir() bool {
	return g.isDir
}

func (g staticFileInfo) Sys() any {
	return nil
}
