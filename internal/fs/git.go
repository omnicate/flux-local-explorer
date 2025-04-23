package fs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

var _ FileSystem = new(gitFileSystem)

// gitFileSystem provides access to files of a local git repository, pinned at
// a specific revision (commit, branch, tag).
type gitFileSystem struct {
	readOnlyFileSystem

	// object.Tree must be protected
	mu sync.Mutex

	repo     *git.Repository
	tree     *object.Tree
	repoPath string
	rev      string
}

// Git returns a new git file system, where repoPath points to a local git repository,
// remoteURL is the remote's URL and rev a valid git revision. Clones the repo from remote URL
// if repoPath doesn't exist. Pulls the repository if a specific revision cannot be resolved.
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
	g.mu.Lock()
	defer g.mu.Unlock()
	entry, ok := g.getTreeEntry(g.tree, path)
	if !ok {
		return false
	}
	return entry.Mode == filemode.Dir
}

func (g *gitFileSystem) ReadDir(path string) ([]string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
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
	g.mu.Lock()
	defer g.mu.Unlock()
	_, ok := g.getTreeEntry(g.tree, path)
	return ok
}

func (g *gitFileSystem) ReadFile(path string) ([]byte, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
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
