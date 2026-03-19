package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/cobra"
	"github.com/omnicate/flx/internal/controller/git"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

func TestImplicitLocalGitRepositoryResources(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "app.yaml"), []byte(`
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: app
  namespace: external-secrets
spec:
  sourceRef:
    kind: GitRepository
    name: cisco-msp-golden-config
    namespace: flux-system
`), 0o644); err != nil {
		t.Fatal(err)
	}

	resources, err := implicitLocalGitRepositoryResources(
		filesys.MakeFsOnDisk(),
		tmpDir,
		[]*git.LocalReplace{{
			Remote: "ssh://git@example.com/repo.git",
			Path:   tmpDir,
			Branch: "main",
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 {
		t.Fatalf("len(resources) = %d, want 1", len(resources))
	}
	if resources[0].GetKind() != "GitRepository" || resources[0].GetNamespace() != "flux-system" || resources[0].GetName() != "cisco-msp-golden-config" {
		t.Fatalf("resource = %s %s/%s", resources[0].GetKind(), resources[0].GetNamespace(), resources[0].GetName())
	}
}

func TestImplicitLocalGitRepositoryResourcesNoGuessing(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "app.yaml"), []byte(`
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: app
  namespace: flux-system
spec:
  sourceRef:
    kind: GitRepository
    name: repo
`), 0o644); err != nil {
		t.Fatal(err)
	}

	resources, err := implicitLocalGitRepositoryResources(
		filesys.MakeFsOnDisk(),
		tmpDir,
		[]*git.LocalReplace{
			{Remote: "ssh://git@example.com/one.git", Path: tmpDir, Branch: "main"},
			{Remote: "ssh://git@example.com/two.git", Path: tmpDir, Branch: "main"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 0 {
		t.Fatalf("len(resources) = %d, want 0", len(resources))
	}
}

func TestCommandControllers(t *testing.T) {
	oldArgs := rootArgs
	t.Cleanup(func() { rootArgs = oldArgs })
	rootArgs.enabledControllers = []string{"ks", "git", "oci", "helm"}

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringSlice("controllers", nil, "")

	got := commandControllers(cmd, []string{"git"})
	if !reflect.DeepEqual(got, []string{"git"}) {
		t.Fatalf("commandControllers(default) = %v, want [git]", got)
	}

	if err := cmd.Flags().Set("controllers", "ks,git"); err != nil {
		t.Fatal(err)
	}
	got = commandControllers(cmd, []string{"git"})
	if !reflect.DeepEqual(got, rootArgs.enabledControllers) {
		t.Fatalf("commandControllers(explicit) = %v, want %v", got, rootArgs.enabledControllers)
	}

	got[0] = "mutated"
	if reflect.DeepEqual(got, rootArgs.enabledControllers) {
		t.Fatalf("commandControllers() returned aliased slice")
	}
}
