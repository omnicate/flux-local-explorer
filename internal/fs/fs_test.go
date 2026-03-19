package fs

import (
	"os"
	"path/filepath"
	"testing"

	"sigs.k8s.io/kustomize/kyaml/filesys"
)

type countingFS struct {
	filesys.FileSystem
	readCount int
}

func (c *countingFS) ReadFile(path string) ([]byte, error) {
	c.readCount++
	return c.FileSystem.ReadFile(path)
}

func TestPrefixFileSystem(t *testing.T) {
	mem := filesys.MakeFsInMemory()
	if err := mem.MkdirAll("root/dir"); err != nil {
		t.Fatal(err)
	}
	if err := mem.WriteFile("root/dir/file.txt", []byte("hello")); err != nil {
		t.Fatal(err)
	}

	prefixed := Prefix(mem, "root")

	if !prefixed.Exists("dir/file.txt") {
		t.Fatal("prefixed file should exist")
	}
	data, err := prefixed.ReadFile("dir/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("ReadFile() = %q, want hello", string(data))
	}
	entries, err := prefixed.ReadDir("dir")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0] != "file.txt" {
		t.Fatalf("ReadDir() = %v, want [file.txt]", entries)
	}
}

func TestMountFileSystem(t *testing.T) {
	base := filesys.MakeFsInMemory()
	if err := base.WriteFile("base.txt", []byte("base")); err != nil {
		t.Fatal(err)
	}
	mountedBase := filesys.MakeFsInMemory()
	if err := mountedBase.MkdirAll("src"); err != nil {
		t.Fatal(err)
	}
	if err := mountedBase.WriteFile("src/child.txt", []byte("mounted")); err != nil {
		t.Fatal(err)
	}

	mounted := Mount(base, &MountPoint{
		Location: "mnt",
		Path:     "src",
		FS:       mountedBase,
	})

	rootEntries, err := mounted.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(rootEntries) != 2 || rootEntries[0] != "base.txt" || rootEntries[1] != "mnt/" {
		t.Fatalf("ReadDir(.) = %v, want [base.txt mnt/]", rootEntries)
	}
	data, err := mounted.ReadFile("mnt/child.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "mounted" {
		t.Fatalf("ReadFile() = %q, want mounted", string(data))
	}
}

func TestCacheFileSystemCachesReads(t *testing.T) {
	mem := filesys.MakeFsInMemory()
	if err := mem.WriteFile("file.txt", []byte("cached")); err != nil {
		t.Fatal(err)
	}
	base := &countingFS{FileSystem: mem}

	cached := Cache(base, t.TempDir())
	for i := 0; i < 2; i++ {
		data, err := cached.ReadFile("file.txt")
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "cached" {
			t.Fatalf("ReadFile() = %q, want cached", string(data))
		}
	}
	if base.readCount != 1 {
		t.Fatalf("base.readCount = %d, want 1", base.readCount)
	}
}

func TestKrustyFileSystemUsesBaseForRelativeAndOSForAbsolute(t *testing.T) {
	mem := filesys.MakeFsInMemory()
	if err := mem.WriteFile("relative.txt", []byte("mem")); err != nil {
		t.Fatal(err)
	}
	krusty := KrustyFileSystem(mem)

	data, err := krusty.ReadFile("relative.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "mem" {
		t.Fatalf("relative ReadFile() = %q, want mem", string(data))
	}

	absPath := filepath.Join(t.TempDir(), "absolute.txt")
	if err := os.WriteFile(absPath, []byte("disk"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err = krusty.ReadFile(absPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "disk" {
		t.Fatalf("absolute ReadFile() = %q, want disk", string(data))
	}
}

func TestKrustyFileSystemWalkAndReadOnlyFileSystem(t *testing.T) {
	mem := filesys.MakeFsInMemory()
	if err := mem.MkdirAll("dir"); err != nil {
		t.Fatal(err)
	}
	if err := mem.WriteFile("dir/file.txt", []byte("ok")); err != nil {
		t.Fatal(err)
	}
	krusty := KrustyFileSystem(mem)

	var visited []string
	err := krusty.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		visited = append(visited, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(visited) != 3 || visited[0] != "." || visited[1] != "dir" || visited[2] != filepath.Join("dir", "file.txt") {
		t.Fatalf("visited = %v, want [., dir, dir/file.txt]", visited)
	}

	var ro readOnlyFileSystem
	if _, err := ro.Create("x"); err != ErrReadOnly {
		t.Fatalf("Create() err = %v, want ErrReadOnly", err)
	}
	if err := ro.MkdirAll("x"); err != ErrReadOnly {
		t.Fatalf("MkdirAll() err = %v, want ErrReadOnly", err)
	}
	if err := ro.RemoveAll("x"); err != ErrReadOnly {
		t.Fatalf("RemoveAll() err = %v, want ErrReadOnly", err)
	}
	if err := ro.WriteFile("x", []byte("y")); err != ErrReadOnly {
		t.Fatalf("WriteFile() err = %v, want ErrReadOnly", err)
	}
}
