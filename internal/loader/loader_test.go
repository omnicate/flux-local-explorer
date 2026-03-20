package loader

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/omnicate/flux-local-explorer/internal/controller"
)

func makeFs() filesys.FileSystem {
	return filesys.MakeFsOnDisk()
}

func mustNode(t *testing.T, yaml string) *ResourceNode {
	t.Helper()
	resources, err := LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	return &ResourceNode{
		Resource: controller.NewResource(resources[0]),
		Status:   StatusCompleted,
	}
}

func TestLoadBytesParsesMultiDocumentYAML(t *testing.T) {
	resources, err := LoadBytes([]byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: first
---
apiVersion: v1
kind: Secret
metadata:
  name: second
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 2 {
		t.Fatalf("len(resources) = %d, want 2", len(resources))
	}
	if resources[0].GetKind() != "ConfigMap" || resources[1].GetKind() != "Secret" {
		t.Fatalf("kinds = %s, %s", resources[0].GetKind(), resources[1].GetKind())
	}
}

func TestLoadPathSupportsFileDirectoryAndKustomization(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "cm.yaml")
	if err := os.WriteFile(filePath, []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: file-cm
`), 0o644); err != nil {
		t.Fatal(err)
	}
	resources, err := LoadPath(makeFs(), filePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 || resources[0].GetName() != "file-cm" {
		t.Fatalf("file resources = %+v", resources)
	}

	dirPath := filepath.Join(tmpDir, "dir")
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirPath, "one.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: one
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirPath, "skip.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	resources, err = LoadPath(makeFs(), dirPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 || resources[0].GetName() != "one" {
		t.Fatalf("dir resources = %+v", resources)
	}

	kustPath := filepath.Join(tmpDir, "kust")
	if err := os.MkdirAll(kustPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kustPath, "kustomization.yaml"), []byte(`
resources:
  - cm.yaml
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kustPath, "cm.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: kust-cm
`), 0o644); err != nil {
		t.Fatal(err)
	}
	resources, err = LoadPath(makeFs(), kustPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 || resources[0].GetName() != "kust-cm" {
		t.Fatalf("kustomization resources = %+v", resources)
	}
}

func TestLoadPathMissingYamlFails(t *testing.T) {
	_, err := LoadPath(makeFs(), filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil || !strings.Contains(err.Error(), "no such file or directory") {
		t.Fatalf("err = %v, want missing file error", err)
	}
}

func TestResourceNodeHelpers(t *testing.T) {
	root := &ResourceNode{}
	ks := mustNode(t, `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: app
  namespace: ns
`)
	cm := mustNode(t, `
apiVersion: v1
kind: ConfigMap
metadata:
  name: settings
  namespace: ns
`)
	secret := mustNode(t, `
apiVersion: v1
kind: Secret
metadata:
  name: token
  namespace: other
`)
	secret.Status = StatusError
	ks.Children = []*ResourceNode{cm}
	root.Children = []*ResourceNode{ks, secret}

	if flat := root.Flat(); len(flat) != 3 {
		t.Fatalf("len(flat) = %d, want 3", len(flat))
	}
	if got := root.FlatByStatus(StatusError); len(got) != 1 || got[0].Resource.GetName() != "token" {
		t.Fatalf("FlatByStatus(StatusError) = %+v", got)
	}
	if got := root.Flat().FilterByKind("ConfigMap"); len(got) != 1 || got[0].Resource.GetName() != "settings" {
		t.Fatalf("FilterByKind(ConfigMap) = %+v", got)
	}
	if got := root.Flat().FilterByNamespace("ns"); len(got) != 2 {
		t.Fatalf("FilterByNamespace(ns) len = %d, want 2", len(got))
	}
	if got := ks.GetResources(); len(got) != 2 {
		t.Fatalf("GetResources() len = %d, want 2", len(got))
	}
	found, ok := root.Find("Secret", "other", "token")
	if !ok || found.Resource.GetName() != "token" {
		t.Fatalf("Find() = %+v, %v", found, ok)
	}
	if !strings.Contains(root.String(), "ConfigMap ns/settings") {
		t.Fatalf("String() = %q", root.String())
	}
}

func TestWalkAndAddResources(t *testing.T) {
	root := &ResourceNode{}
	resources, err := LoadBytes([]byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: one
  namespace: ns
---
apiVersion: v1
kind: Secret
metadata:
  name: two
  namespace: ns
`))
	if err != nil {
		t.Fatal(err)
	}
	root.AddResources(controller.NewResources(resources))

	var names []string
	err = root.Walk(func(node *ResourceNode) error {
		if node.Resource != nil {
			names = append(names, node.Resource.GetName())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "one" || names[1] != "two" {
		t.Fatalf("names = %v", names)
	}
}

func TestContextAccessors(t *testing.T) {
	res := mustNode(t, `
apiVersion: v1
kind: ConfigMap
metadata:
  name: one
  namespace: ns
`)
	res.Attachment = "attached"
	ctx := &Context{tree: &ResourceNode{Children: []*ResourceNode{res}}}

	attachment, ok := ctx.GetAttachment("ConfigMap", "ns", "one")
	if !ok || attachment != "attached" {
		t.Fatalf("GetAttachment() = %v, %v", attachment, ok)
	}
	resource, ok := ctx.GetResource("ConfigMap", "ns", "one")
	if !ok || resource.GetName() != "one" {
		t.Fatalf("GetResource() = %+v, %v", resource, ok)
	}
	if _, ok := ctx.GetAttachment("ConfigMap", "ns", "missing"); ok {
		t.Fatal("unexpected attachment for missing resource")
	}
}

type stubController struct {
	kinds     []string
	reconcile func(controller.Context, *controller.Resource) (*controller.Result, error)
}

func (s stubController) Kinds() []string {
	return s.kinds
}

func (s stubController) Reconcile(ctx controller.Context, resource *controller.Resource) (*controller.Result, error) {
	return s.reconcile(ctx, resource)
}

func TestManagerRunAddsResourcesAndDeduplicates(t *testing.T) {
	root := &ResourceNode{}
	initial := mustNode(t, `
apiVersion: v1
kind: ConfigMap
metadata:
  name: source
  namespace: ns
`)
	initial.Status = StatusUnknown
	root.Children = []*ResourceNode{initial}

	secretResource := mustNode(t, `
apiVersion: v1
kind: Secret
metadata:
  name: generated
  namespace: ns
`).Resource

	mgr := NewManager(zerolog.Nop(), []controller.Controller{
		stubController{
			kinds: []string{"ConfigMap"},
			reconcile: func(_ controller.Context, _ *controller.Resource) (*controller.Result, error) {
				return &controller.Result{Resources: []*controller.Resource{secretResource, secretResource}}, nil
			},
		},
	})
	mgr.root = root

	if err := mgr.Run(); err != nil {
		t.Fatal(err)
	}

	nodes := mgr.AllNodes()
	if len(nodes) != 2 {
		t.Fatalf("len(nodes) = %d, want 2", len(nodes))
	}
	if nodes[0].Resource.GetName() != "source" || nodes[1].Resource.GetName() != "generated" {
		t.Fatalf("nodes = %s, %s", nodes[0].Resource.GetName(), nodes[1].Resource.GetName())
	}
}

func TestManagerProcessNodeRetriesAndMarksError(t *testing.T) {
	node := mustNode(t, `
apiVersion: v1
kind: ConfigMap
metadata:
  name: source
  namespace: ns
`)
	node.Status = StatusUnknown

	mgr := NewManager(zerolog.Nop(), []controller.Controller{
		stubController{
			kinds: []string{"ConfigMap"},
			reconcile: func(_ controller.Context, _ *controller.Resource) (*controller.Result, error) {
				return nil, errors.New("boom")
			},
		},
	})
	mgr.root = &ResourceNode{Children: []*ResourceNode{node}}

	for i := 0; i < 6; i++ {
		if ok := mgr.processNode(node); !ok {
			t.Fatalf("processNode() = false on attempt %d", i)
		}
	}

	if node.Status != StatusError {
		t.Fatalf("node.Status = %v, want StatusError", node.Status)
	}
	if node.Error == nil || node.Error.Error() != "boom" {
		t.Fatalf("node.Error = %v, want boom", node.Error)
	}
	if node.Attempts != 6 {
		t.Fatalf("node.Attempts = %d, want 6", node.Attempts)
	}
}

func TestManagerProcessNodeWithoutControllerCompletes(t *testing.T) {
	node := mustNode(t, `
apiVersion: v1
kind: ConfigMap
metadata:
  name: source
  namespace: ns
`)
	node.Status = StatusUnknown

	mgr := NewManager(zerolog.Nop(), nil)
	mgr.root = &ResourceNode{Children: []*ResourceNode{node}}

	if ok := mgr.processNode(node); !ok {
		t.Fatal("processNode() = false, want true")
	}
	if node.Status != StatusCompleted {
		t.Fatalf("node.Status = %v, want StatusCompleted", node.Status)
	}
}

func TestManagerInitializeAddResourcesAndListWithKind(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "cm.yaml"), []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: one
`), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(zerolog.Nop(), nil)
	if err := mgr.Initialize(makeFs(), tmpDir, "flux-system"); err != nil {
		t.Fatal(err)
	}
	results := mgr.ListWithKind("ConfigMap", "flux-system", false)
	if len(results) != 1 || results[0].Resource.GetNamespace() != "flux-system" {
		t.Fatalf("results = %+v", results)
	}

	mgr.AddResources([]*controller.Resource{mustNode(t, `
apiVersion: v1
kind: Secret
metadata:
  name: extra
  namespace: other
`).Resource})
	if got := mgr.ListWithKind("Secret", "", true); len(got) != 1 || got[0].Resource.GetName() != "extra" {
		t.Fatalf("ListWithKind(Secret) = %+v", got)
	}
}

func TestClientSetGetAndGroupVersionKindFor(t *testing.T) {
	cmNode := mustNode(t, `
apiVersion: v1
kind: ConfigMap
metadata:
  name: one
  namespace: ns
`)
	clientset := NewClientSet(controller.Scheme, &ResourceNode{Children: []*ResourceNode{cmNode}})

	var cm corev1.ConfigMap
	if err := clientset.Get(context.Background(), client.ObjectKey{Name: "one", Namespace: "ns"}, &cm); err != nil {
		t.Fatal(err)
	}
	if cm.Name != "one" {
		t.Fatalf("cm.Name = %q, want one", cm.Name)
	}

	gvk, err := clientset.GroupVersionKindFor(&corev1.ConfigMap{})
	if err != nil {
		t.Fatal(err)
	}
	if gvk.Kind != "ConfigMap" {
		t.Fatalf("gvk.Kind = %q, want ConfigMap", gvk.Kind)
	}

	if err := clientset.Get(context.Background(), client.ObjectKey{Name: "missing", Namespace: "ns"}, &corev1.ConfigMap{}); err == nil || !strings.Contains(err.Error(), "object not found") {
		t.Fatalf("missing Get() err = %v, want object not found", err)
	}
}

func TestResourceNodeFindMissing(t *testing.T) {
	root := &ResourceNode{Children: []*ResourceNode{mustNode(t, `
apiVersion: v1
kind: ConfigMap
metadata:
  name: one
  namespace: ns
`)}}
	if _, ok := root.Find("Secret", "ns", "one"); ok {
		t.Fatal("unexpected match")
	}
}
