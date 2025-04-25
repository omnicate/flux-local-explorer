// Copyright 2025 Cisco Systems, Inc. and its affiliates
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package git

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/rs/zerolog"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	ctrl "github.com/omnicate/flx/internal/controller"
	"github.com/omnicate/flx/internal/fs"
)

func init() {
	_ = sourcev1.AddToScheme(ctrl.Scheme)
}

var _ ctrl.Controller = new(Controller)

type Controller struct {
	logger zerolog.Logger
	opts   Options
	mu     sync.Mutex
}

func NewController(logger zerolog.Logger, opts Options) *Controller {
	return &Controller{logger: logger, opts: opts}
}

func (g *Controller) Kinds() []string {
	return []string{"GitRepository"}
}

func (g *Controller) Reconcile(ctx ctrl.Context, req *ctrl.Resource) (*ctrl.Result, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	var gr sourcev1.GitRepository
	if err := req.Unmarshal(&gr); err != nil {
		return nil, err
	}

	if gr.Spec.Reference == nil {
		return nil, fmt.Errorf("git repo reference is required")
	}

	var remoteURL string
	var err error
	if g.opts.UseHTTPS {
		remoteURL, err = gitHttpsURL(gr.Spec.URL)
	} else {
		remoteURL, err = gitSSHUrl(gr.Spec.URL)
	}
	if err != nil {
		return nil, fmt.Errorf("git url: %w", err)
	}

	g.logger.Debug().
		Str("url", remoteURL).
		Str("ref", gitRepoReference(gr)).
		Msg("loading git repository")

	ref := gr.Spec.Reference
	if ref == nil {
		return nil, fmt.Errorf("git repository without reference")
	}
	if ref.Commit == "" && ref.Branch == "" && ref.Tag == "" {
		return nil, fmt.Errorf("git repo must have a reference")
	}

	var repoFS fs.FileSystem
	gitRef := gitRepoReference(gr)

	var isLocal bool
	for _, lgr := range g.opts.Local {
		equals, err := gitURLEquals(lgr.Remote, remoteURL)
		if err != nil {
			return nil, fmt.Errorf("git url equals: %w", err)
		}
		if equals && lgr.Ref() == gitRef {
			repoFS = fs.Prefix(
				filesys.MakeFsOnDisk(),
				lgr.Path,
			)
			isLocal = true
			g.logger.Debug().
				Str("url", gr.Spec.URL).
				Str("local", lgr.Path).
				Msg("using local file system")
		}
	}

	// Use git filesystems for non-local repos:
	if !isLocal {
		var err error
		var gitRepoPath string
		md5h := md5.Sum([]byte(remoteURL))
		hash := hex.EncodeToString(md5h[:])
		gitRepoPath = filepath.Join(g.opts.CachePath, hash)
		repoFS, err = fs.Git(
			gitRepoPath,
			remoteURL,
			gitRef,
		)
		if err != nil {
			return nil, err
		}
	}

	// Handle includes:
	mountPoints := make([]*fs.MountPoint, 0)
	for _, include := range gr.Spec.Include {
		includedAttachment, ok := ctx.GetAttachment(
			"GitRepository",
			gr.Namespace,
			include.GitRepositoryRef.Name,
		)
		if !ok {
			return nil, fmt.Errorf(
				"include %s/%s not found",
				gr.Namespace,
				include.GitRepositoryRef.Name,
			)
		}
		includedFS, ok := includedAttachment.(filesys.FileSystem)
		if !ok {
			return nil, fmt.Errorf(
				"include %s/%s has invalid attachment: %T",
				gr.Namespace,
				include.GitRepositoryRef.Name,
				includedAttachment,
			)
		}
		mountPoints = append(mountPoints, &fs.MountPoint{
			Location: ctrl.Any(include.ToPath, include.GitRepositoryRef.Name),
			Path:     include.FromPath,
			FS:       includedFS,
		})
	}
	if len(mountPoints) > 0 {
		repoFS = fs.Mount(repoFS, mountPoints...)
	}

	attachment := fs.KrustyFileSystem(repoFS)
	return &ctrl.Result{Attachment: attachment}, nil
}

// gitHttpsURL returns a URL that clones via https protocol.
func gitHttpsURL(repoURL string) (string, error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "https":
		return u.String(), nil
	case "ssh":
		u.Scheme = "https"
		u.User = nil
		if strings.HasSuffix(u.Path, ".git") {
			u.Path = strings.TrimSuffix(u.Path, ".git")
		}
		return u.String(), nil
	default:
		u.Scheme = "https"
		return gitHttpsURL(u.String())
	}
}

// gitSSHUrl returns a URL that will clone via SSH protocol.
func gitSSHUrl(repoURL string) (string, error) {
	// TODO: hack to parse relative git repos, e.g. ssh://git@github.com:a/b.
	//  otherwise errors "first path segment in URL cannot contain colon".
	//  Consider https://github.com/gitsight/go-vcsurl or some other parsing library.
	repoURL = strings.Replace(repoURL, "github.com:", "github.com/", 1)

	u, err := url.Parse(repoURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "ssh"
		return gitSSHUrl(u.String())
	case "ssh":
		if u.User == nil {
			u.User = url.User("git")
		}
		// TODO: is it better to always return without .git ?
		if !strings.HasSuffix(u.Path, ".git") {
			u.Path = u.Path + ".git"
		}
		return u.String(), nil
	default:
		u.Scheme = "ssh"
		return gitSSHUrl(u.String())
	}
}

// gitURLEquals returns true if two remotes have the same representation
// when converted to SSH urls.
func gitURLEquals(a, b string) (bool, error) {
	au, err := gitSSHUrl(a)
	if err != nil {
		return false, err
	}
	bu, err := gitSSHUrl(b)
	if err != nil {
		return false, err
	}
	return au == bu, nil
}

func gitRepoReference(gr sourcev1.GitRepository) string {
	ref := gr.Spec.Reference
	return ctrl.Any(ref.Commit, ref.Branch, ref.Tag)
}
