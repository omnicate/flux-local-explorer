package fs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitFileSystemReadsRepositoryContent(t *testing.T) {
	tmpDir := t.TempDir()
	originPath := filepath.Join(tmpDir, "origin.git")
	workPath := filepath.Join(tmpDir, "work")
	cachePath := filepath.Join(tmpDir, "cache")

	runGit(t, tmpDir, "init", "--bare", originPath)
	runGit(t, tmpDir, "clone", originPath, workPath)
	runGit(t, workPath, "config", "user.name", "Test User")
	runGit(t, workPath, "config", "user.email", "test@example.com")
	if err := os.MkdirAll(filepath.Join(workPath, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workPath, "dir", "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, workPath, "add", ".")
	runGit(t, workPath, "commit", "-m", "initial")
	runGit(t, workPath, "push", "-u", "origin", "master")

	gfs, err := Git(cachePath, originPath, "master")
	if err != nil {
		t.Fatal(err)
	}
	if !gfs.IsDir(".") || !gfs.IsDir("dir") {
		t.Fatal("expected directories to exist")
	}
	if !gfs.Exists("dir/file.txt") || gfs.Exists("missing.txt") {
		t.Fatal("unexpected Exists result")
	}
	entries, err := gfs.ReadDir("dir")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0] != "file.txt" {
		t.Fatalf("ReadDir() = %v", entries)
	}
	data, err := gfs.ReadFile("dir/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("ReadFile() = %q, want hello", string(data))
	}
	if _, err := gfs.ReadDir("dir/file.txt"); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("ReadDir(file) err = %v", err)
	}
	if _, err := gfs.ReadFile("dir"); err == nil || !strings.Contains(err.Error(), "not a file") {
		t.Fatalf("ReadFile(dir) err = %v", err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
}
