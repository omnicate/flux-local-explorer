package loader

import (
	"os"
	"path/filepath"
	"testing"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHandleKustomizationFallsBackToLocalRepoForMissingGitSource(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, "kustomization.yaml"), []byte(`
resources:
  - configmap.yaml
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "configmap.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: example
`), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(WithLocalRepoRef(&LocalGitRepository{
		Path:   tmpDir,
		Remote: "https://example.com/repo.git",
		Branch: "main",
	}))

	ks := &kustomizev1.Kustomization{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "flux-system",
		},
		Spec: kustomizev1.KustomizationSpec{
			Path: "./",
			SourceRef: kustomizev1.CrossNamespaceSourceReference{
				Kind: "GitRepository",
				Name: "repo",
			},
		},
	}

	result, err := loader.handleKustomization(new(Queue), ks)
	if err != nil {
		t.Fatalf("handleKustomization() error = %v", err)
	}
	if got := len(result.Resources); got != 1 {
		t.Fatalf("len(result.Resources) = %d, want 1", got)
	}
	if _, ok := loader.repos["GitRepository/flux-system/repo"]; !ok {
		t.Fatalf("missing cached local fallback repository")
	}
}

func TestHandleKustomizationMissingGitSourceWithoutLocalRepoFails(t *testing.T) {
	loader := NewLoader()
	ks := &kustomizev1.Kustomization{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "flux-system",
		},
		Spec: kustomizev1.KustomizationSpec{
			Path: "./",
			SourceRef: kustomizev1.CrossNamespaceSourceReference{
				Kind: "GitRepository",
				Name: "repo",
			},
		},
	}

	_, err := loader.handleKustomization(new(Queue), ks)
	if err == nil || err.Error() != "could not find source: GitRepository/flux-system/repo" {
		t.Fatalf("handleKustomization() error = %v, want missing source error", err)
	}
}

func TestHandleKustomizationWithMultipleLocalReposDoesNotGuessFallback(t *testing.T) {
	tmpDir := t.TempDir()
	loader := NewLoader(WithLocalRepoRef(
		&LocalGitRepository{Path: tmpDir, Remote: "https://example.com/one.git", Branch: "main"},
		&LocalGitRepository{Path: tmpDir, Remote: "https://example.com/two.git", Branch: "main"},
	))
	ks := &kustomizev1.Kustomization{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "flux-system",
		},
		Spec: kustomizev1.KustomizationSpec{
			Path: "./",
			SourceRef: kustomizev1.CrossNamespaceSourceReference{
				Kind: "GitRepository",
				Name: "repo",
			},
		},
	}

	_, err := loader.handleKustomization(new(Queue), ks)
	if err == nil || err.Error() != "could not find source: GitRepository/flux-system/repo" {
		t.Fatalf("handleKustomization() error = %v, want missing source error", err)
	}
}

func TestFallbackLocalGitRepositoryReusesCachedRepo(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "marker.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	loader := NewLoader(WithLocalRepoRef(&LocalGitRepository{
		Path:   tmpDir,
		Remote: "https://example.com/repo.git",
		Branch: "main",
	}))

	first, ok := loader.fallbackLocalGitRepository("GitRepository/flux-system/repo")
	if !ok {
		t.Fatal("first fallback did not resolve")
	}
	second, ok := loader.fallbackLocalGitRepository("GitRepository/flux-system/repo")
	if !ok {
		t.Fatal("second fallback did not resolve")
	}
	if first != second {
		t.Fatal("fallback did not reuse cached filesystem")
	}
	if !first.Exists("marker.txt") {
		t.Fatal("fallback filesystem does not point at local repository")
	}
}
