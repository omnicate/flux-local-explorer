package kustomize

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	ctrl "github.com/omnicate/flux-local-explorer/internal/controller"
	fluxfs "github.com/omnicate/flux-local-explorer/internal/fs"
	"github.com/omnicate/flux-local-explorer/internal/loader"
)

type stubContext struct {
	attachments map[string]any
	client      client.Client
}

func (s stubContext) ClientSet() client.Client {
	return s.client
}

func (s stubContext) GetAttachment(kind, namespace, name string) (any, bool) {
	value, ok := s.attachments[kind+"/"+namespace+"/"+name]
	return value, ok
}

func (s stubContext) GetResource(kind, namespace, name string) (*ctrl.Resource, bool) {
	return nil, false
}

func TestReconcileSupportsRepoRootAbsolutePaths(t *testing.T) {
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")
	appDir := filepath.Join(repoDir, "workloads", "demo", "app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "kustomization.yaml"), []byte(`
resources:
  - cm.yaml
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "cm.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: demo
  namespace: ns
`), 0o644); err != nil {
		t.Fatal(err)
	}

	resources, err := loader.LoadBytes([]byte(`
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: demo
  namespace: ns
spec:
  path: /workloads/demo/app
  sourceRef:
    kind: GitRepository
    name: repo
    namespace: flux-system
`))
	if err != nil {
		t.Fatal(err)
	}

	repoFS := fluxfs.KrustyFileSystem(fluxfs.Prefix(filesys.MakeFsOnDisk(), repoDir))
	ctx := stubContext{
		attachments: map[string]any{
			"GitRepository/flux-system/repo": repoFS,
		},
		client: loader.NewClientSet(ctrl.Scheme, &loader.ResourceNode{}),
	}

	result, err := NewController(zerolog.Nop()).Reconcile(ctx, ctrl.NewResource(resources[0]))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Resources) != 1 || result.Resources[0].GetKind() != "ConfigMap" || result.Resources[0].GetName() != "demo" {
		t.Fatalf("result.Resources = %+v, want rendered configmap", result.Resources)
	}
}
