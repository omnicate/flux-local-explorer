package fs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

var _ FileSystem = new(gitFileSystem)

type gitFileSystem struct {
	readOnlyFileSystem

	repo     *git.Repository
	tree     *object.Tree
	repoPath string
	rev      string
}

func Git(repoPath, remoteURL, rev string) (FileSystem, error) {
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		cmd := exec.Command("git", "clone", remoteURL, repoPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("git clone failed: %v", err)
		}
	}
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, err
	}
	hash, err := repo.ResolveRevision(plumbing.Revision(rev))
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		cmd := exec.Command("git", "-C", repoPath, "pull")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("git pull failed: %v", err)
		}
		hash, err = repo.ResolveRevision(plumbing.Revision(rev))
		if err != nil {
			return nil, fmt.Errorf("invalid revision after pulling: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to resolve revision %q: %v", rev, err)
	}
	commit, err := repo.CommitObject(*hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %v", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get tree: %v", err)
	}
	return &gitFileSystem{
		repoPath: repoPath,
		rev:      rev,
		tree:     tree,
		repo:     repo,
	}, nil
}

func (g *gitFileSystem) getTreeEntry(tree *object.Tree, path string) (object.TreeEntry, bool) {
	path = filepath.Clean(path)
	if path == "." || path == "/" {
		return object.TreeEntry{
			Name: path,
			Mode: filemode.Dir,
			Hash: tree.Hash,
		}, true
	}
	dir, nextDir, _ := strings.Cut(path, string(os.PathSeparator))
	for _, entry := range tree.Entries {
		if entry.Name == path {
			return entry, true
		}
		if entry.Name == dir {
			if entry.Mode == filemode.Dir {
				subTree, err := g.repo.TreeObject(entry.Hash)
				if err != nil {
					return object.TreeEntry{}, false
				}
				return g.getTreeEntry(subTree, nextDir)
			}
		}
	}
	return object.TreeEntry{}, false
}

func (g *gitFileSystem) IsDir(path string) bool {
	entry, ok := g.getTreeEntry(g.tree, path)
	if !ok {
		return false
	}
	return entry.Mode == filemode.Dir
}

func (g *gitFileSystem) ReadDir(path string) ([]string, error) {
	entry, ok := g.getTreeEntry(g.tree, path)
	if !ok {
		return nil, fmt.Errorf("dir does not exist: %s", path)
	}
	if entry.Mode != filemode.Dir {
		return nil, fmt.Errorf("not a directory: %s", path)
	}
	tree, err := g.repo.TreeObject(entry.Hash)
	if err != nil {
		return nil, err
	}
	entries := make([]string, len(tree.Entries))
	for i, child := range tree.Entries {
		entries[i] = child.Name
	}
	return entries, nil
}

func (g *gitFileSystem) Exists(path string) bool {
	_, ok := g.getTreeEntry(g.tree, path)
	return ok
}

func (g *gitFileSystem) ReadFile(path string) ([]byte, error) {
	entry, ok := g.getTreeEntry(g.tree, path)
	if !ok {
		return nil, fmt.Errorf("file does not exist: %s", path)
	}
	if entry.Mode == filemode.Dir {
		return nil, fmt.Errorf("not a file: %s", path)
	}
	obj, err := g.repo.Object(plumbing.AnyObject, entry.Hash)
	if err != nil {
		return nil, err
	}
	if obj.Type() != plumbing.BlobObject {
		return nil, fmt.Errorf("not a blob: %s", path)
	}
	blob, _ := obj.(*object.Blob)
	reader, err := blob.Reader()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return content, nil
}

type gitFileNode struct {
	name     string
	hash     string
	isDir    bool
	children []*gitFileNode
}

func (n *gitFileNode) insert(path, hash string) {
	path = filepath.Clean(path)
	dir, nextDir, ok := strings.Cut(path, string(os.PathSeparator))
	if !ok {
		n.children = append(n.children, &gitFileNode{
			name:  dir,
			hash:  hash,
			isDir: false,
		})
		return
	}
	// Try to insert into children:
	for _, c := range n.children {
		if c.name == dir {
			c.insert(nextDir, hash)
			return
		}
	}
	// Could not find place to insert:
	child := &gitFileNode{
		name:     dir,
		isDir:    true,
		children: nil,
	}
	child.insert(nextDir, hash)
	n.children = append(n.children, child)
}

func (n *gitFileNode) get(path string) (*gitFileNode, bool) {
	path = filepath.Clean(path)
	dir, nextDir, ok := strings.Cut(path, string(os.PathSeparator))
	for _, c := range n.children {
		if c.name == dir {
			if !ok {
				return c, true
			}
			return c.get(nextDir)
		}
	}
	return nil, false
}
