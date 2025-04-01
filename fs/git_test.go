package fs

import (
	"testing"
)

func TestGitFileNode(t *testing.T) {
	files := []string{
		"a/b/c",
		"a/b/c.json",
	}
	g := new(gitFileNode)
	for _, f := range files {
		g.insert(f)
	}
	if _, ok := g.get("a"); !ok {
		t.Fatalf("expect a")
	}
	if _, ok := g.get("a/"); !ok {
		t.Fatalf("expect a/")
	}
	if _, ok := g.get("a/b"); !ok {
		t.Fatalf("expect a/b")
	}
	if _, ok := g.get("a/b/"); !ok {
		t.Fatalf("expect a/b/")
	}
	if dir, ok := g.get("a/b/c"); !ok || dir.isDir {
		t.Fatalf("expect a/b/c to be file")
	}
	if dir, ok := g.get("a/b/c.json"); !ok || dir.isDir {
		t.Fatalf("expect a/b/c.json to be a file")
	}
}
