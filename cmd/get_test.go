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
	"sigs.k8s.io/kustomize/api/resource"
	sigyaml "sigs.k8s.io/yaml"

	"github.com/omnicate/flux-local-explorer/internal/controller"
	"github.com/omnicate/flux-local-explorer/internal/loader"
)

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

func mustNode(t *testing.T, yaml string) *loader.ResourceNode {
	t.Helper()
	resources, err := loader.LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	return &loader.ResourceNode{
		Resource: controller.NewResource(resources[0]),
		Status:   loader.StatusCompleted,
	}
}

func TestSortAndFilterResults(t *testing.T) {
	nodes := []*loader.ResourceNode{
		mustNode(t, `
apiVersion: v1
kind: ConfigMap
metadata:
  name: b
  namespace: z
`),
		mustNode(t, `
apiVersion: v1
kind: ConfigMap
metadata:
  name: c
  namespace: a
`),
		mustNode(t, `
apiVersion: v1
kind: ConfigMap
metadata:
  name: b
  namespace: a
`),
	}

	sortResources(nodes)
	got := []string{
		nodes[0].Resource.GetNamespace() + "/" + nodes[0].Resource.GetName(),
		nodes[1].Resource.GetNamespace() + "/" + nodes[1].Resource.GetName(),
		nodes[2].Resource.GetNamespace() + "/" + nodes[2].Resource.GetName(),
	}
	want := []string{"a/b", "a/c", "z/b"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sorted[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	filtered := filterResults(nodes, "", "a", false)
	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}

	filtered = filterResults(nodes, "b", "a", false)
	if len(filtered) != 1 || filtered[0].Resource.GetName() != "b" {
		t.Fatalf("filtered exact = %+v", filtered)
	}

	filtered = filterResults(nodes, "", "", true)
	if len(filtered) != 3 {
		t.Fatalf("allNamespaces len = %d, want 3", len(filtered))
	}
}

func TestPrintResultsYamlAndUnknownFormat(t *testing.T) {
	oldArgs := getArgs
	t.Cleanup(func() { getArgs = oldArgs })
	getArgs = GetFlags{format: "yaml"}

	node := mustNode(t, `
apiVersion: v1
kind: ConfigMap
metadata:
  name: a
  namespace: ns-a
`)
	output := captureStdout(t, func() {
		err := printResults(
			[]*loader.ResourceNode{node},
			func() []string { return []string{"Name"} },
			func(item *loader.ResourceNode) []string { return []string{item.Resource.GetName()} },
		)
		if err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "---") || !strings.Contains(output, "namespace: ns-a") {
		t.Fatalf("output = %q, want YAML resource", output)
	}

	getArgs.format = "wat"
	err := printResults(
		[]*loader.ResourceNode{node},
		func() []string { return []string{"Name"} },
		func(item *loader.ResourceNode) []string { return []string{item.Resource.GetName()} },
	)
	if err == nil || err.Error() != "unknown format: wat" {
		t.Fatalf("err = %v, want unknown format", err)
	}

	getArgs.format = "pretty"
	err = printResults(nil, func() []string { return nil }, func(item *loader.ResourceNode) []string { return nil })
	if err == nil || err.Error() != "no results" {
		t.Fatalf("err = %v, want no results", err)
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

	ksNode := mustNode(t, `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: app
  namespace: ns
spec:
  sourceRef:
    kind: GitRepository
    name: repo
`)
	ksNode.Children = []*loader.ResourceNode{
		mustNode(t, `
apiVersion: v1
kind: ConfigMap
metadata:
  name: child
  namespace: ns
`),
	}

	var ks kustomizev1.Kustomization
	if err := ksNode.Resource.Unmarshal(&ks); err != nil {
		t.Fatal(err)
	}
	if got := formatSource(&ks); got != "git: ns/repo" {
		t.Fatalf("formatSource() = %q", got)
	}
	row := kustomizationRows(ksNode)
	if len(row) != 5 || row[0] != "ns" || row[2] != "git: ns/repo" || row[3] != "2" {
		t.Fatalf("kustomizationRows() = %v", row)
	}

	ks.Spec.SourceRef = kustomizev1.CrossNamespaceSourceReference{
		Kind:      "OCIRepository",
		Name:      "image",
		Namespace: "shared",
	}
	if got := formatSource(&ks); got != "oci: shared/image" {
		t.Fatalf("formatSource() oci = %q", got)
	}

	grNode := mustNode(t, `
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: repo
  namespace: ns
spec:
  url: ssh://git@example.com/repo.git
  ref:
    commit: abc123
`)
	grNode.Error = errors.New("oops")
	var gr sourcev1.GitRepository
	if err := grNode.Resource.Unmarshal(&gr); err != nil {
		t.Fatal(err)
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
	grNode.Resource = controller.NewResource(&resource.Resource{RNode: *grNode.Resource.Resource.RNode.Copy()})
	data, _ := sigyaml.Marshal(gr)
	resources, err := loader.LoadBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	grNode.Resource = controller.NewResource(resources[0])

	row = gitRepoRows(grNode)
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

	orNode := mustNode(t, `
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: OCIRepository
metadata:
  name: image
  namespace: ns
spec:
  url: oci://example.com/repo
  ref:
    digest: sha256:abc
`)
	ociRow := ociRepoRows(orNode)
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

func TestPrintKustomizationYAML(t *testing.T) {
	ksNode := mustNode(t, `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: app
  namespace: ns
spec:
  sourceRef:
    kind: GitRepository
    name: repo
`)
	ksNode.Children = []*loader.ResourceNode{
		mustNode(t, `
apiVersion: v1
kind: ConfigMap
metadata:
  name: child
  namespace: ns
`),
	}

	output := captureStdout(t, func() {
		if err := printKustomizationYAML([]*loader.ResourceNode{ksNode}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "Resources:") {
		t.Fatalf("output = %q, want Resources", output)
	}
	if strings.Count(output, "- apiVersion:") != 2 {
		t.Fatalf("output = %q, want 2 resources", output)
	}
	if strings.Contains(output, "Error:") {
		t.Fatalf("output = %q, did not want nil Error field", output)
	}

	ksNode.Error = errors.New("boom")
	output = captureStdout(t, func() {
		if err := printKustomizationYAML([]*loader.ResourceNode{ksNode}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "Error: boom") {
		t.Fatalf("output = %q, want error", output)
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
