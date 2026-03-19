package cmd

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/kustomize/api/resource"
	sigyaml "sigs.k8s.io/yaml"

	"github.com/omnicate/flx/loader"
)

type testResource struct {
	Namespace string `yaml:"namespace"`
	Name      string `yaml:"name"`
	Value     string `yaml:"value,omitempty"`
}

func (t testResource) GetNamespace() string { return t.Namespace }
func (t testResource) GetName() string      { return t.Name }

func seqFrom[T any](items []T, err error) loader.ErrSeq[T] {
	return func(yield func(T, error) bool) {
		if err != nil {
			var zero T
			yield(zero, err)
			return
		}
		for _, item := range items {
			if !yield(item, nil) {
				return
			}
		}
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSortResources(t *testing.T) {
	resources := []testResource{
		{Namespace: "z", Name: "b"},
		{Namespace: "a", Name: "c"},
		{Namespace: "a", Name: "b"},
	}

	sortResources(resources)

	got := []testResource(resources)
	want := []testResource{
		{Namespace: "a", Name: "b"},
		{Namespace: "a", Name: "c"},
		{Namespace: "z", Name: "b"},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("resource[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestGetResultsFromSeqFiltersByNamespace(t *testing.T) {
	oldArgs := getArgs
	t.Cleanup(func() { getArgs = oldArgs })
	getArgs = GetFlags{namespace: "ns-a"}

	results, err := getResultsFromSeq(seqFrom([]testResource{
		{Namespace: "ns-b", Name: "b"},
		{Namespace: "ns-a", Name: "a"},
	}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "a" {
		t.Fatalf("results = %+v, want only ns-a/a", results)
	}
}

func TestGetResultsFromSeqFindsNamedResource(t *testing.T) {
	oldArgs := getArgs
	t.Cleanup(func() { getArgs = oldArgs })
	getArgs = GetFlags{namespace: "ns-a", name: "b"}

	results, err := getResultsFromSeq(seqFrom([]testResource{
		{Namespace: "ns-a", Name: "a"},
		{Namespace: "ns-a", Name: "b"},
	}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "b" {
		t.Fatalf("results = %+v, want only ns-a/b", results)
	}
}

func TestGetResultsFromSeqAllNamespacesAndErrors(t *testing.T) {
	oldArgs := getArgs
	t.Cleanup(func() { getArgs = oldArgs })
	getArgs = GetFlags{allNamespaces: true}

	results, err := getResultsFromSeq(seqFrom([]testResource{
		{Namespace: "b", Name: "a"},
		{Namespace: "a", Name: "b"},
	}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Namespace != "a" || results[1].Namespace != "b" {
		t.Fatalf("results = %+v, want sorted cross-namespace list", results)
	}

	_, err = getResultsFromSeq(seqFrom([]testResource(nil), errors.New("boom")))
	if err == nil || err.Error() != "boom" {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestGetResultsFromSeqNoResources(t *testing.T) {
	oldArgs := getArgs
	t.Cleanup(func() { getArgs = oldArgs })
	getArgs = GetFlags{namespace: "missing"}

	_, err := getResultsFromSeq(seqFrom([]testResource{
		{Namespace: "ns-a", Name: "a"},
	}, nil))
	if err == nil || err.Error() != "no resources" {
		t.Fatalf("err = %v, want no resources", err)
	}
}

func TestPrintResultsYAMLAndUnknownFormat(t *testing.T) {
	oldArgs := getArgs
	t.Cleanup(func() { getArgs = oldArgs })
	getArgs = GetFlags{format: "yaml"}

	output := captureStdout(t, func() {
		err := printResults(
			[]testResource{{Namespace: "ns-a", Name: "a", Value: "v"}},
			func() []string { return []string{"Name"} },
			func(item testResource) []string { return []string{item.Name} },
		)
		if err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "---") || !strings.Contains(output, "Namespace: ns-a") {
		t.Fatalf("output = %q, want YAML resource", output)
	}

	getArgs.format = "wat"
	err := printResults(
		[]testResource{{Namespace: "ns-a", Name: "a"}},
		func() []string { return []string{"Name"} },
		func(item testResource) []string { return []string{item.Name} },
	)
	if err == nil || err.Error() != "unknown format: wat" {
		t.Fatalf("err = %v, want unknown format", err)
	}
}

func TestErrOrEmpty(t *testing.T) {
	if got := errOrEmpty(nil); got != "" {
		t.Fatalf("errOrEmpty(nil) = %q, want empty", got)
	}
	if got := errOrEmpty(errors.New("boom")); got != "boom" {
		t.Fatalf("errOrEmpty(err) = %q, want boom", got)
	}
}

func TestFormatHelpersAndRows(t *testing.T) {
	oldArgs := getArgs
	t.Cleanup(func() { getArgs = oldArgs })
	getArgs = GetFlags{allNamespaces: true}

	ks := &loader.Kustomization{
		Kustomization: &kustomizev1.Kustomization{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "ns"},
			Spec: kustomizev1.KustomizationSpec{
				SourceRef: kustomizev1.CrossNamespaceSourceReference{
					Kind: "GitRepository",
					Name: "repo",
				},
			},
		},
		Resources: []*resource.Resource{{}},
	}
	if got := formatSource(ks); got != "git: ns/repo" {
		t.Fatalf("formatSource() = %q", got)
	}
	row := kustomizationRows(ks)
	if len(row) != 5 || row[0] != "ns" || row[2] != "git: ns/repo" {
		t.Fatalf("kustomizationRows() = %v", row)
	}

	ks.Spec.SourceRef = kustomizev1.CrossNamespaceSourceReference{
		Kind:      "OCIRepository",
		Name:      "image",
		Namespace: "shared",
	}
	if got := formatSource(ks); got != "oci: shared/image" {
		t.Fatalf("formatSource() oci = %q", got)
	}

	gr := &loader.GitRepository{
		GitRepository: &sourcev1.GitRepository{
			ObjectMeta: metav1.ObjectMeta{Name: "repo", Namespace: "ns"},
			Spec: sourcev1.GitRepositorySpec{
				URL: "ssh://git@example.com/repo.git",
				Reference: &sourcev1.GitRepositoryRef{
					Commit: "abc123",
				},
			},
		},
		Error: errors.New("oops"),
	}
	var includes sourcev1.GitRepository
	if err := sigyaml.Unmarshal([]byte(`
spec:
  include:
    - repository:
        name: a
    - repository:
        name: b
`), &includes); err != nil {
		t.Fatal(err)
	}
	gr.Spec.Include = includes.Spec.Include
	row = gitRepoRows(gr)
	if len(row) != 6 || row[2] != "ssh://git@example.com/repo.git" || row[3] != "Commit: abc123" || row[4] != "a, b" || row[5] != "oops" {
		t.Fatalf("gitRepoRows() = %v", row)
	}
	if got := formatGitRepoReference(&sourcev1.GitRepositoryRef{Branch: "main"}); got != "Branch: main" {
		t.Fatalf("formatGitRepoReference(branch) = %q", got)
	}
	if got := formatGitRepoReference(&sourcev1.GitRepositoryRef{Tag: "v1"}); got != "Tag: v1" {
		t.Fatalf("formatGitRepoReference(tag) = %q", got)
	}
	if got := formatGitRepoReference(&sourcev1.GitRepositoryRef{}); got != "Unknown" {
		t.Fatalf("formatGitRepoReference(empty) = %q", got)
	}

	or := &loader.OCIRepository{
		OCIRepository: &sourcev1b2.OCIRepository{
			ObjectMeta: metav1.ObjectMeta{Name: "image", Namespace: "ns"},
			Spec: sourcev1b2.OCIRepositorySpec{
				URL: "oci://example.com/repo",
				Reference: &sourcev1b2.OCIRepositoryRef{
					Digest: "sha256:abc",
				},
			},
		},
	}
	ociRow := ociRepoRows(or)
	if len(ociRow) != 5 || ociRow[2] != "oci://example.com/repo" || ociRow[3] != "Digest: sha256:abc" {
		t.Fatalf("ociRepoRows() = %v", ociRow)
	}
	if got := formatOciReference(&sourcev1b2.OCIRepositoryRef{Tag: "latest"}); got != "Tag: latest" {
		t.Fatalf("formatOciReference(tag) = %q", got)
	}
	if got := formatOciReference(&sourcev1b2.OCIRepositoryRef{SemVer: "1.x"}); got != "Version: 1.x" {
		t.Fatalf("formatOciReference(semver) = %q", got)
	}
	if got := formatOciReference(&sourcev1b2.OCIRepositoryRef{}); got != "Unknown" {
		t.Fatalf("formatOciReference(empty) = %q", got)
	}
}

func TestHeaderHelpers(t *testing.T) {
	oldArgs := getArgs
	t.Cleanup(func() { getArgs = oldArgs })

	getArgs = GetFlags{}
	if got := kustomizationHeaders(); len(got) != 4 || got[0] != "Name" {
		t.Fatalf("kustomizationHeaders() = %v", got)
	}
	if got := gitRepoHeaders(); len(got) != 5 || got[0] != "Name" {
		t.Fatalf("gitRepoHeaders() = %v", got)
	}
	if got := ociRepoHeaders(); len(got) != 4 || got[0] != "Name" {
		t.Fatalf("ociRepoHeaders() = %v", got)
	}

	getArgs = GetFlags{allNamespaces: true}
	if got := kustomizationHeaders(); got[0] != "Namespace" {
		t.Fatalf("kustomizationHeaders(all namespaces) = %v", got)
	}
	if got := gitRepoHeaders(); got[0] != "Namespace" {
		t.Fatalf("gitRepoHeaders(all namespaces) = %v", got)
	}
	if got := ociRepoHeaders(); got[0] != "Namespace" {
		t.Fatalf("ociRepoHeaders(all namespaces) = %v", got)
	}
}

func TestRepoGitHelpers(t *testing.T) {
	tmpDir := t.TempDir()
	originPath := filepath.Join(tmpDir, "origin.git")
	workPath := filepath.Join(tmpDir, "work")

	runGit(t, tmpDir, "init", "--bare", originPath)
	runGit(t, tmpDir, "clone", originPath, workPath)
	runGit(t, workPath, "config", "user.name", "Test User")
	runGit(t, workPath, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(workPath, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, workPath, "add", "README.md")
	runGit(t, workPath, "commit", "-m", "initial")
	runGit(t, workPath, "push", "-u", "origin", "master")

	url, err := repoURL(workPath)
	if err != nil {
		t.Fatal(err)
	}
	if url != originPath {
		t.Fatalf("repoURL() = %q, want %q", url, originPath)
	}

	topLevel, err := repoTopLevel(filepath.Join(workPath, "subdir", ".."))
	if err != nil {
		t.Fatal(err)
	}
	wantTopLevel, err := filepath.EvalSymlinks(workPath)
	if err != nil {
		t.Fatal(err)
	}
	if topLevel != wantTopLevel {
		t.Fatalf("repoTopLevel() = %q, want %q", topLevel, wantTopLevel)
	}

	branch, err := repoDefaultBranch(workPath)
	if err != nil {
		t.Fatal(err)
	}
	if branch != "master" {
		t.Fatalf("repoDefaultBranch() = %q, want master", branch)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
}
