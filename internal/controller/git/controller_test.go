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
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	ctrl "github.com/omnicate/flux-local-explorer/internal/controller"
	"github.com/omnicate/flux-local-explorer/internal/loader"
)

func Test_gitHttpsUrl(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "without user",
			input: "ssh://github.com/omnicate/kubeconf",
			want:  "https://github.com/omnicate/kubeconf",
		},
		{
			name:  "with user",
			input: "ssh://git@github.com/omnicate/kubeconf",
			want:  "https://github.com/omnicate/kubeconf",
		},
		{
			name:  "with .git",
			input: "ssh://git@github.com/omnicate/kubeconf.git",
			want:  "https://github.com/omnicate/kubeconf",
		},
		{
			name:  "https",
			input: "https://github.com/omnicate/kubeconf",
			want:  "https://github.com/omnicate/kubeconf",
		},
		{
			name:  "no scheme",
			input: "github.com/omnicate/kubeconf.git",
			want:  "https://github.com/omnicate/kubeconf.git",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gitHttpsURL(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("gitSSHUrl() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("gitSSHUrl() got = %v, want %v", got, tt.want)
			}
		})
	}
}

type stubContext struct {
	attachments map[string]any
}

func (s stubContext) ClientSet() client.Client {
	return nil
}

func (s stubContext) GetAttachment(kind, namespace, name string) (any, bool) {
	v, ok := s.attachments[kind+"/"+namespace+"/"+name]
	return v, ok
}

func (s stubContext) GetResource(kind, namespace, name string) (*ctrl.Resource, bool) {
	return nil, false
}

func makeGitRepoResource(t *testing.T, yaml string) *ctrl.Resource {
	t.Helper()
	resources, err := loader.LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	return ctrl.NewResources(resources)[0]
}

func TestControllerReconcileUsesLocalRepository(t *testing.T) {
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(filepath.Join(repoPath, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "dir", "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	resource := makeGitRepoResource(t, `
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: repo
  namespace: flux-system
spec:
  url: ssh://git@example.com/repo.git
  ref:
    branch: main
`)

	controller := NewController(zerolog.Nop(), Options{
		Local: []*LocalReplace{{
			Remote: "ssh://git@example.com/repo.git",
			Path:   repoPath,
			Branch: "main",
		}},
	})

	result, err := controller.Reconcile(stubContext{}, resource)
	if err != nil {
		t.Fatal(err)
	}
	fs, ok := result.Attachment.(filesys.FileSystem)
	if !ok {
		t.Fatalf("attachment type = %T, want filesys.FileSystem", result.Attachment)
	}
	data, err := fs.ReadFile("dir/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("ReadFile() = %q, want hello", string(data))
	}
}

func TestControllerReconcileMountsIncludes(t *testing.T) {
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "base.txt"), []byte("base"), 0o644); err != nil {
		t.Fatal(err)
	}

	included := filesys.MakeFsInMemory()
	if err := included.MkdirAll("src"); err != nil {
		t.Fatal(err)
	}
	if err := included.WriteFile("src/child.txt", []byte("included")); err != nil {
		t.Fatal(err)
	}

	resource := makeGitRepoResource(t, `
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: repo
  namespace: flux-system
spec:
  url: ssh://git@example.com/repo.git
  ref:
    branch: main
  include:
  - repository:
      name: shared
    fromPath: src
    toPath: mounted
`)

	controller := NewController(zerolog.Nop(), Options{
		Local: []*LocalReplace{{
			Remote: "ssh://git@example.com/repo.git",
			Path:   repoPath,
			Branch: "main",
		}},
	})

	result, err := controller.Reconcile(stubContext{
		attachments: map[string]any{
			"GitRepository/flux-system/shared": included,
		},
	}, resource)
	if err != nil {
		t.Fatal(err)
	}
	fs := result.Attachment.(filesys.FileSystem)
	data, err := fs.ReadFile("mounted/child.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "included" {
		t.Fatalf("ReadFile() = %q, want included", string(data))
	}
}

func TestControllerReconcileErrors(t *testing.T) {
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatal(err)
	}

	controller := NewController(zerolog.Nop(), Options{
		Local: []*LocalReplace{{
			Remote: "ssh://git@example.com/repo.git",
			Path:   repoPath,
			Branch: "main",
		}},
	})

	resource := makeGitRepoResource(t, `
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: repo
  namespace: flux-system
spec: {}
`)

	if _, err := controller.Reconcile(stubContext{}, resource); err == nil || err.Error() != "git repo reference is required" {
		t.Fatalf("Reconcile(no ref) err = %v", err)
	}

	resource = makeGitRepoResource(t, `
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: repo
  namespace: flux-system
spec:
  url: ssh://git@example.com/repo.git
  ref: {}
`)
	if _, err := controller.Reconcile(stubContext{}, resource); err == nil || err.Error() != "git repo must have a reference" {
		t.Fatalf("Reconcile(empty ref) err = %v", err)
	}

	resource = makeGitRepoResource(t, `
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: repo
  namespace: flux-system
spec:
  url: ssh://git@example.com/repo.git
  ref:
    branch: main
  include:
  - repository:
      name: shared
`)
	if _, err := controller.Reconcile(stubContext{}, resource); err == nil || err.Error() != "include flux-system/shared not found" {
		t.Fatalf("Reconcile(missing include) err = %v", err)
	}

	if _, err := controller.Reconcile(stubContext{
		attachments: map[string]any{
			"GitRepository/flux-system/shared": "wrong",
		},
	}, resource); err == nil || err.Error() != "include flux-system/shared has invalid attachment: string" {
		t.Fatalf("Reconcile(invalid include attachment) err = %v", err)
	}
}

func TestLocalReplaceRef(t *testing.T) {
	if got := (&LocalReplace{Commit: "sha", Branch: "main", Tag: "v1"}).Ref(); got != "sha" {
		t.Fatalf("Ref(commit) = %q, want sha", got)
	}
	if got := (&LocalReplace{Branch: "main", Tag: "v1"}).Ref(); got != "main" {
		t.Fatalf("Ref(branch) = %q, want main", got)
	}
	if got := (&LocalReplace{Tag: "v1"}).Ref(); got != "v1" {
		t.Fatalf("Ref(tag) = %q, want v1", got)
	}
}

func Test_gitSSHUrl(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "without user",
			input: "ssh://github.com/omnicate/kubeconf",
			want:  "ssh://git@github.com/omnicate/kubeconf.git",
		},
		{
			name:  "with user",
			input: "ssh://git@github.com/omnicate/kubeconf",
			want:  "ssh://git@github.com/omnicate/kubeconf.git",
		},
		{
			name:  "with .git",
			input: "ssh://git@github.com/omnicate/kubeconf.git",
			want:  "ssh://git@github.com/omnicate/kubeconf.git",
		},
		{
			name:  "https",
			input: "https://github.com/omnicate/kubeconf",
			want:  "ssh://git@github.com/omnicate/kubeconf.git",
		},
		{
			name:  "relative url",
			input: "ssh://git@github.com:omnicate/kubeconf.git",
			want:  "ssh://git@github.com/omnicate/kubeconf.git",
		},
		{
			name:  "no scheme",
			input: "git@github.com:omnicate/kubeconf.git",
			want:  "ssh://git@github.com/omnicate/kubeconf.git",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gitSSHUrl(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("gitSSHUrl() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("gitSSHUrl() got = %v, want %v", got, tt.want)
			}
		})
	}
}
