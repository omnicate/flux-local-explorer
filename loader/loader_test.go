package loader

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

func TestLoadBytesParsesMultiDocumentYAML(t *testing.T) {
	l := NewLoader()
	resources, err := l.loadBytes([]byte(`
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
	l := NewLoader()
	fs := filesys.MakeFsOnDisk()

	filePath := filepath.Join(tmpDir, "cm.yaml")
	if err := os.WriteFile(filePath, []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: file-cm
`), 0o644); err != nil {
		t.Fatal(err)
	}
	resources, err := l.loadPath(fs, filePath)
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
	resources, err = l.loadPath(fs, dirPath)
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
	resources, err = l.loadPath(fs, kustPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 || resources[0].GetName() != "kust-cm" {
		t.Fatalf("kustomization resources = %+v", resources)
	}
}

func TestHandleResourceQueuesFluxKindsAndCreatesObjects(t *testing.T) {
	l := NewLoader()
	queue := new(Queue)

	makeRes := func(yaml string) *resource.Resource {
		resources, err := l.loadBytes([]byte(yaml))
		if err != nil {
			t.Fatal(err)
		}
		return resources[0]
	}

	if err := l.handleResource(queue, makeRes(`
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: app
`), "flux-system"); err != nil {
		t.Fatal(err)
	}
	if err := l.handleResource(queue, makeRes(`
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: repo
spec:
  url: ssh://git@example.com/repo.git
  ref:
    branch: main
`), "flux-system"); err != nil {
		t.Fatal(err)
	}
	if err := l.handleResource(queue, makeRes(`
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: OCIRepository
metadata:
  name: image
spec:
  url: oci://example.com/repo
  ref:
    tag: latest
`), "flux-system"); err != nil {
		t.Fatal(err)
	}
	if err := l.handleResource(queue, makeRes(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: settings
data:
  key: value
`), "custom-ns"); err != nil {
		t.Fatal(err)
	}
	if err := l.handleResource(queue, makeRes(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ignored
`), "flux-system"); err != nil {
		t.Fatal(err)
	}

	if len(queue.items) != 3 {
		t.Fatalf("len(queue.items) = %d, want 3 flux objects", len(queue.items))
	}
	var cm unstructured.Unstructured
	cm.SetAPIVersion("v1")
	cm.SetKind("ConfigMap")
	err := l.cs.Get(context.Background(), types.NamespacedName{Namespace: "custom-ns", Name: "settings"}, &cm)
	if err != nil {
		t.Fatal(err)
	}
	if cm.GetNamespace() != "custom-ns" {
		t.Fatalf("configmap namespace = %q, want custom-ns", cm.GetNamespace())
	}
}

func TestHandleResourceInvalidYamlAndAlreadyExists(t *testing.T) {
	l := NewLoader()
	queue := new(Queue)

	resources, err := l.loadBytes([]byte("not: [valid"))
	if err == nil || resources != nil {
		t.Fatalf("loadBytes invalid YAML err = %v resources = %v", err, resources)
	}

	cmRes, err := l.loadBytes([]byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: dupe
  namespace: ns
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := l.handleResource(queue, cmRes[0], "ns"); err != nil {
		t.Fatal(err)
	}
	if err := l.handleResource(queue, cmRes[0], "ns"); err != nil {
		t.Fatalf("duplicate create err = %v, want nil", err)
	}
}

func TestIterYieldsGitRepositoryResources(t *testing.T) {
	tmpDir := t.TempDir()
	fs := filesys.MakeFsOnDisk()
	if err := os.WriteFile(filepath.Join(tmpDir, "repo.yaml"), []byte(`
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: repo
  namespace: flux-system
spec:
  url: ssh://git@example.com/repo.git
  ref:
    branch: main
`), 0o644); err != nil {
		t.Fatal(err)
	}

	var got *GitRepository
	for item, err := range NewLoader().GitRepositories(fs, tmpDir, "flux-system") {
		if err != nil {
			t.Fatal(err)
		}
		got = item
	}
	if got == nil || got.Name != "repo" {
		t.Fatalf("got = %+v, want repo", got)
	}
}

func TestTypedIterAndFindHelpers(t *testing.T) {
	seq := ErrSeq[NamedResource](func(yield func(NamedResource, error) bool) {
		yield(&GitRepository{GitRepository: &sourcev1.GitRepository{
			ObjectMeta: metav1.ObjectMeta{Name: "repo", Namespace: "flux-system"},
		}}, nil)
	})

	results, err := typedIter[*GitRepository](seq).Collect()
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "repo" {
		t.Fatalf("typedIter results = %+v", results)
	}
}

func TestQueueHelpers(t *testing.T) {
	cmResources, err := NewLoader().loadBytes([]byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm
  namespace: ns
`))
	if err != nil {
		t.Fatal(err)
	}
	q := new(Queue)
	q.Push(&QueueItem{Value: &kustomizev1.Kustomization{ObjectMeta: metav1.ObjectMeta{Name: "ks", Namespace: "ns"}}})
	q.Push(&QueueItem{Value: &sourcev1.GitRepository{ObjectMeta: metav1.ObjectMeta{Name: "gr", Namespace: "ns"}}})
	q.Push(&QueueItem{Value: &sourcev1b2.OCIRepository{ObjectMeta: metav1.ObjectMeta{Name: "or", Namespace: "ns"}}})
	q.Push(&QueueItem{Value: cmResources[0]})

	item, ok := q.Pop()
	if !ok || item.Kind() != "Kustomization" || item.NamespacedName() != "ns/ks" {
		t.Fatalf("first pop = %+v, ok=%v", item, ok)
	}
	item, _ = q.Pop()
	if item.Kind() != "GitRepository" {
		t.Fatalf("kind = %s, want GitRepository", item.Kind())
	}
	item, _ = q.Pop()
	if item.Kind() != "OCIRepository" {
		t.Fatalf("kind = %s, want OCIRepository", item.Kind())
	}
	item, _ = q.Pop()
	if item.Kind() != "Resource" {
		t.Fatalf("kind = %s, want Resource", item.Kind())
	}

	retry := &QueueItem{Value: &kustomizev1.Kustomization{ObjectMeta: metav1.ObjectMeta{Name: "retry", Namespace: "ns"}}}
	q.Retry(retry, errors.New("boom"))
	item, _ = q.Pop()
	if item.Attempt != 1 || item.Err == nil || item.Err.Error() != "boom" {
		t.Fatalf("retry item = %+v", item)
	}
}

func TestOrDefault(t *testing.T) {
	if got := orDefault("", "fallback"); got != "fallback" {
		t.Fatalf("orDefault string = %q", got)
	}
	if got := orDefault(1, 2); got != 1 {
		t.Fatalf("orDefault int = %d", got)
	}
}

func TestHandleResourceCreatesUnstructuredObject(t *testing.T) {
	l := NewLoader()
	queue := new(Queue)
	resources, err := l.loadBytes([]byte(`
apiVersion: v1
kind: Secret
metadata:
  name: secret
`))
	if err != nil {
		t.Fatal(err)
	}
	if err := l.handleResource(queue, resources[0], "ns"); err != nil {
		t.Fatal(err)
	}
	var secret unstructured.Unstructured
	secret.SetAPIVersion("v1")
	secret.SetKind("Secret")
	err = l.cs.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: "secret"}, &secret)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLoadPathMissingDirectoryFails(t *testing.T) {
	_, err := NewLoader().loadPath(filesys.MakeFsOnDisk(), filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("expected missing path error")
	}
	if !os.IsNotExist(err) && !strings.Contains(err.Error(), "no such file or directory") {
		t.Fatalf("unexpected err: %v", err)
	}
}
