package affected

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/omnicate/flux-local-explorer/internal/controller"
	"github.com/omnicate/flux-local-explorer/internal/loader"
)

type Options struct {
	RepoRoot string
	Files    []string
}

type Target struct {
	EntryPoint string `json:"entryPoint" yaml:"entryPoint"`
	Namespace  string `json:"namespace" yaml:"namespace"`
	Name       string `json:"name" yaml:"name"`
	Reason     string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

type kustomizationRef struct {
	Namespace string `yaml:"namespace"`
	Kind      string `yaml:"kind"`
	Name      string `yaml:"name"`
}

type substituteRef struct {
	Kind string `yaml:"kind"`
	Name string `yaml:"name"`
}

type postBuildSpec struct {
	SubstituteFrom []substituteRef `yaml:"substituteFrom"`
}

type fluxSpec struct {
	Path      string             `yaml:"path"`
	SourceRef kustomizationRef   `yaml:"sourceRef"`
	PostBuild *postBuildSpec     `yaml:"postBuild"`
	DependsOn []kustomizationRef `yaml:"dependsOn"`
}

type metadata struct {
	Name      string            `yaml:"name"`
	Namespace string            `yaml:"namespace"`
	Labels    map[string]string `yaml:"labels"`
}

type resourceDoc struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   metadata `yaml:"metadata"`
	Spec       fluxSpec `yaml:"spec"`
}

type kustomizeFile struct {
	Namespace             string                  `yaml:"namespace"`
	NamePrefix            string                  `yaml:"namePrefix"`
	Resources             []string                `yaml:"resources"`
	Bases                 []string                `yaml:"bases"`
	Components            []string                `yaml:"components"`
	CRDs                  []string                `yaml:"crds"`
	Configurations        []string                `yaml:"configurations"`
	Generators            []string                `yaml:"generators"`
	Transformers          []string                `yaml:"transformers"`
	Validators            []string                `yaml:"validators"`
	PatchesStrategicMerge []string                `yaml:"patchesStrategicMerge"`
	PatchesJson6902       []struct{ Path string } `yaml:"patchesJson6902"`
	Patches               []struct{ Path string } `yaml:"patches"`
	Replacements          []struct{ Path string } `yaml:"replacements"`
	HelmCharts            []struct {
		ValuesFile            string   `yaml:"valuesFile"`
		AdditionalValuesFiles []string `yaml:"additionalValuesFiles"`
	} `yaml:"helmCharts"`
	ConfigMapGenerator []struct {
		Files []string `yaml:"files"`
		Envs  []string `yaml:"envs"`
	} `yaml:"configMapGenerator"`
	SecretGenerator []struct {
		Files []string `yaml:"files"`
		Envs  []string `yaml:"envs"`
	} `yaml:"secretGenerator"`
	OpenAPI struct {
		Path string `yaml:"path"`
	} `yaml:"openapi"`
}

type rawKustomization struct {
	File string
	Doc  resourceDoc
}

var errRenderedKustomizationNotFound = errors.New("rendered kustomization not found")

func Find(opts Options) ([]Target, error) {
	repoRoot, err := filepath.Abs(opts.RepoRoot)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(opts.Files))
	for _, file := range opts.Files {
		rel, err := normalizeInputPath(repoRoot, file)
		if err != nil {
			return nil, err
		}
		files = append(files, rel)
	}

	raw, err := loadRawKustomizations(repoRoot)
	if err != nil {
		return nil, err
	}

	seen := map[string]Target{}
	for _, changed := range files {
		for _, ks := range raw {
			entrypoints, err := entrypointDirs(repoRoot, ks.File)
			if err != nil {
				return nil, err
			}
			for _, entrypoint := range entrypoints {
				rendered, err := renderedKustomization(repoRoot, entrypoint, ks.Doc)
				if err != nil {
					rendered = ks.Doc
				}
				ok, reason, err := affectedByFile(repoRoot, changed, entrypoint, ks, rendered)
				if err != nil {
					return nil, err
				}
				if !ok {
					continue
				}
				target := Target{
					EntryPoint: filepath.Join(repoRoot, filepath.FromSlash(entrypoint)),
					Namespace:  defaultNamespace(rendered.Metadata.Namespace),
					Name:       rendered.Metadata.Name,
					Reason:     reason,
				}
				key := target.EntryPoint + "\t" + target.Namespace + "\t" + target.Name
				seen[key] = target
			}
		}
	}

	out := make([]Target, 0, len(seen))
	for _, target := range seen {
		out = append(out, target)
	}
	sort.Slice(out, func(i, j int) bool {
		a := out[i].EntryPoint + "\t" + out[i].Namespace + "\t" + out[i].Name
		b := out[j].EntryPoint + "\t" + out[j].Namespace + "\t" + out[j].Name
		return a < b
	})
	return out, nil
}

func List(opts Options) ([]Target, error) {
	repoRoot, err := filepath.Abs(opts.RepoRoot)
	if err != nil {
		return nil, err
	}

	raw, err := loadRawKustomizations(repoRoot)
	if err != nil {
		return nil, err
	}
	kustomizations, err := findKustomizationFiles(repoRoot)
	if err != nil {
		return nil, err
	}
	fluxEntrypoints := fluxEntrypointDirs(repoRoot, raw, kustomizations)

	seen := map[string]Target{}
	for _, ks := range raw {
		entrypoints, err := listedEntrypointDirsFromFiles(repoRoot, ks.File, ks.Doc, kustomizations, fluxEntrypoints)
		if err != nil {
			return nil, err
		}
		for _, entrypoint := range entrypoints {
			rendered, err := renderedKustomization(repoRoot, entrypoint, ks.Doc)
			if err != nil {
				if errors.Is(err, errRenderedKustomizationNotFound) {
					continue
				}
				return nil, err
			}
			target := Target{
				EntryPoint: filepath.Join(repoRoot, filepath.FromSlash(entrypoint)),
				Namespace:  defaultNamespace(rendered.Metadata.Namespace),
				Name:       rendered.Metadata.Name,
			}
			key := target.EntryPoint + "\t" + target.Namespace + "\t" + target.Name
			seen[key] = target
		}
	}

	return sortedTargets(seen), nil
}

func normalizeInputPath(repoRoot, file string) (string, error) {
	if filepath.IsAbs(file) {
		rel, err := filepath.Rel(repoRoot, file)
		if err != nil {
			return "", err
		}
		return filepath.ToSlash(filepath.Clean(rel)), nil
	}
	return filepath.ToSlash(filepath.Clean(file)), nil
}

func loadRawKustomizations(repoRoot string) ([]rawKustomization, error) {
	var out []rawKustomization
	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isYAML(path) {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		docs, err := loadDocs(path)
		if err != nil {
			return nil
		}
		for _, doc := range docs {
			if isFluxKustomization(doc) {
				out = append(out, rawKustomization{
					File: filepath.ToSlash(rel),
					Doc:  doc,
				})
			}
		}
		return nil
	})
	return out, err
}

func loadDocs(path string) ([]resourceDoc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	resources, err := loader.LoadBytes(data)
	if err != nil {
		return nil, err
	}
	out := make([]resourceDoc, 0, len(resources))
	for _, res := range resources {
		var doc resourceDoc
		if err := controller.NewResource(res).Unmarshal(&doc); err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	return out, nil
}

func isFluxKustomization(doc resourceDoc) bool {
	group, _, _ := strings.Cut(doc.APIVersion, "/")
	return doc.Kind == "Kustomization" && group == "kustomize.toolkit.fluxcd.io"
}

func entrypointDirs(repoRoot, manifest string) ([]string, error) {
	kustomizations, err := findKustomizationFiles(repoRoot)
	if err != nil {
		return nil, err
	}
	return entrypointDirsFromFiles(repoRoot, manifest, kustomizations)
}

func entrypointDirsFromFiles(repoRoot, manifest string, kustomizations []string) ([]string, error) {
	entrypoints, referenced, err := referencedEntrypointDirsFromFiles(repoRoot, manifest, kustomizations)
	if err != nil {
		return nil, err
	}
	if referenced {
		return entrypoints, nil
	}

	manifestDir := filepath.ToSlash(filepath.Dir(manifest))
	if manifestDir == "." {
		manifestDir = ""
	}
	return []string{manifestDir}, nil
}

func listedEntrypointDirsFromFiles(repoRoot, manifest string, doc resourceDoc, kustomizations []string, fluxEntrypoints map[string]bool) ([]string, error) {
	entrypoints, ancestors, referenced, err := listedEntrypointCandidatesFromFiles(repoRoot, manifest, kustomizations)
	if err != nil {
		return nil, err
	}
	if referenced {
		if ancestor, ok := preferredFluxOwnedAncestor(repoRoot, ancestors, fluxEntrypoints); ok {
			out := []string{ancestor}
			out = append(out, fluxOwnedEntrypointDirs(repoRoot, externalEntrypointDirs(manifest, entrypoints), fluxEntrypoints)...)
			return out, nil
		}
		owned := fluxOwnedEntrypointDirs(repoRoot, entrypoints, fluxEntrypoints)
		if len(owned) > 0 {
			return owned, nil
		}
		if len(fluxEntrypoints) == 0 {
			return entrypoints, nil
		}
		manifestDir := filepath.ToSlash(filepath.Dir(manifest))
		if manifestDir == "." {
			manifestDir = ""
		}
		if !fluxEntrypointOwnsFile(repoRoot, manifest, fluxEntrypoints) {
			return entrypoints, nil
		}
		if len(ancestors) > 0 {
			return []string{deepestEntrypointAncestor(ancestors)}, nil
		}
		if fluxEntrypointOwnsDir(repoRoot, manifestDir, fluxEntrypoints) || (fluxEntrypoints[""] && len(ancestors) == 0) {
			return []string{manifestDir}, nil
		}
		return nil, nil
	}
	manifestDir := filepath.ToSlash(filepath.Dir(manifest))
	if manifestDir == "." {
		manifestDir = ""
	}
	if !fluxEntrypointOwnsDir(repoRoot, manifestDir, fluxEntrypoints) && !fluxEntrypoints[""] {
		if !standaloneFluxManifestRendersSpecPath(repoRoot, doc) {
			return nil, nil
		}
	}
	entrypoints = []string{manifestDir}
	return entrypoints, nil
}

func standaloneFluxManifestRendersSpecPath(repoRoot string, doc resourceDoc) bool {
	if doc.Spec.Path == "" {
		return false
	}
	specPath := normalizeSpecPath(doc.Spec.Path)
	path := filepath.Join(repoRoot, filepath.FromSlash(specPath))
	if _, err := os.Stat(path); err != nil {
		return false
	}
	resources, err := loader.LoadPath(filesys.MakeFsOnDisk(), path)
	if err != nil || len(resources) == 0 {
		return false
	}
	return true
}

func externalEntrypointDirs(manifest string, entrypoints []string) []string {
	out := make([]string, 0, len(entrypoints))
	for _, entrypoint := range entrypoints {
		if entrypoint == "" || hasPathPrefix(manifest, entrypoint) {
			continue
		}
		out = append(out, entrypoint)
	}
	return out
}

func listedEntrypointCandidatesFromFiles(repoRoot, manifest string, kustomizations []string) ([]string, []string, bool, error) {
	var external []string
	var ancestors []string

	for _, file := range kustomizations {
		dir := filepath.ToSlash(filepath.Dir(file))
		if dir == "." {
			dir = ""
		}
		ok, err := kustomizationReferencesInput(repoRoot, dir, manifest, map[string]bool{}, true)
		if err != nil {
			return nil, nil, false, err
		}
		if !ok {
			continue
		}
		if dir == "" || hasPathPrefix(manifest, dir) {
			ancestors = append(ancestors, dir)
			continue
		}
		external = append(external, dir)
	}
	sort.Slice(ancestors, func(i, j int) bool {
		return pathDepth(ancestors[i]) < pathDepth(ancestors[j])
	})
	if len(external) > 0 {
		sort.Strings(external)
		return external, ancestors, true, nil
	}
	if len(ancestors) > 0 {
		return []string{ancestors[len(ancestors)-1]}, ancestors, true, nil
	}
	return nil, nil, false, nil
}

func deepestEntrypointAncestor(ancestors []string) string {
	deepest := ancestors[0]
	for _, ancestor := range ancestors[1:] {
		if pathDepth(ancestor) > pathDepth(deepest) {
			deepest = ancestor
		}
	}
	return deepest
}

func preferredFluxOwnedAncestor(repoRoot string, ancestors []string, fluxEntrypoints map[string]bool) (string, bool) {
	for entrypoint := range fluxEntrypoints {
		if _, hasKustomization := findKustomizationFile(repoRoot, entrypoint); hasKustomization {
			for _, ancestor := range ancestors {
				if ancestor == entrypoint && kustomizationHasLocalTransform(repoRoot, entrypoint) {
					return ancestor, true
				}
			}
			for i := len(ancestors) - 1; i >= 0; i-- {
				ancestor := ancestors[i]
				if ancestor != entrypoint && fluxEntrypointOwnsDir(repoRoot, ancestor, map[string]bool{entrypoint: true}) && kustomizationHasLocalTransform(repoRoot, ancestor) {
					return ancestor, true
				}
			}
			for _, ancestor := range ancestors {
				if ancestor == entrypoint {
					return ancestor, true
				}
			}
			continue
		}
		for i := len(ancestors) - 1; i >= 0; i-- {
			if fluxEntrypointOwnsDir(repoRoot, ancestors[i], map[string]bool{entrypoint: true}) {
				return ancestors[i], true
			}
		}
	}
	return "", false
}

func kustomizationHasLocalTransform(repoRoot, dir string) bool {
	kfile, ok := findKustomizationFile(repoRoot, dir)
	if !ok {
		return false
	}
	k, ok, err := loadKustomizeFile(filepath.Join(repoRoot, filepath.FromSlash(kfile)))
	if err != nil || !ok {
		return false
	}
	return k.Namespace != "" || k.NamePrefix != ""
}

func fluxOwnedEntrypointDirs(repoRoot string, entrypoints []string, fluxEntrypoints map[string]bool) []string {
	out := make([]string, 0, len(entrypoints))
	for _, entrypoint := range entrypoints {
		if fluxEntrypointOwnsDir(repoRoot, entrypoint, fluxEntrypoints) {
			out = append(out, entrypoint)
		}
	}
	return out
}

func referencedEntrypointDirsFromFiles(repoRoot, manifest string, kustomizations []string) ([]string, bool, error) {
	var external []string
	ancestor := ""
	ancestorDepth := -1

	for _, file := range kustomizations {
		dir := filepath.ToSlash(filepath.Dir(file))
		if dir == "." {
			dir = ""
		}
		ok, err := kustomizationReferencesInput(repoRoot, dir, manifest, map[string]bool{}, true)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			continue
		}
		depth := pathDepth(dir)
		if dir == "" || hasPathPrefix(manifest, dir) {
			if depth > ancestorDepth {
				ancestorDepth = depth
				ancestor = dir
			}
			continue
		}
		external = append(external, dir)
	}
	if len(external) > 0 {
		sort.Strings(external)
		return external, true, nil
	}
	if ancestorDepth >= 0 {
		return []string{ancestor}, true, nil
	}
	return nil, false, nil
}

func fluxEntrypointDirs(repoRoot string, raw []rawKustomization, kustomizations []string) map[string]bool {
	out := map[string]bool{}
	for _, ks := range raw {
		if ks.Doc.Spec.Path == "" {
			continue
		}
		specPath := normalizeSpecPath(ks.Doc.Spec.Path)
		if !fluxEntrypointPathExists(repoRoot, specPath, raw) {
			continue
		}
		out[specPath] = true
	}
	for _, kfile := range kustomizations {
		dir := filepath.ToSlash(filepath.Dir(kfile))
		if dir == "." {
			dir = ""
		}
		resources, err := loader.LoadPath(filesys.MakeFsOnDisk(), filepath.Join(repoRoot, filepath.FromSlash(dir)))
		if err != nil {
			continue
		}
		for _, res := range resources {
			var doc resourceDoc
			if err := controller.NewResource(res).Unmarshal(&doc); err != nil || !isFluxKustomization(doc) || doc.Spec.Path == "" {
				continue
			}
			specPath := normalizeSpecPath(doc.Spec.Path)
			if !fluxEntrypointPathExists(repoRoot, specPath, raw) {
				continue
			}
			out[specPath] = true
		}
	}
	return out
}

func fluxEntrypointPathExists(repoRoot, specPath string, raw []rawKustomization) bool {
	resources, err := loader.LoadPath(filesys.MakeFsOnDisk(), filepath.Join(repoRoot, filepath.FromSlash(specPath)))
	if err == nil {
		for _, res := range resources {
			var doc resourceDoc
			if err := controller.NewResource(res).Unmarshal(&doc); err == nil && isFluxKustomization(doc) {
				return true
			}
		}
	}
	for _, ks := range raw {
		if pathMatches(ks.File, specPath) {
			return true
		}
	}
	return false
}

func fluxEntrypointOwnsDir(repoRoot, dir string, fluxEntrypoints map[string]bool) bool {
	for entrypoint := range fluxEntrypoints {
		if entrypoint == "" {
			if dir == "" {
				return true
			}
			continue
		}
		if dir == entrypoint {
			return true
		}
		if _, hasKustomization := findKustomizationFile(repoRoot, entrypoint); !hasKustomization && hasPathPrefix(dir, entrypoint) {
			return true
		}
		ok, err := kustomizationReferencesInput(repoRoot, entrypoint, dir, map[string]bool{}, true)
		if err == nil && ok {
			return true
		}
	}
	return false
}

func fluxEntrypointOwnsFile(repoRoot, file string, fluxEntrypoints map[string]bool) bool {
	for entrypoint := range fluxEntrypoints {
		if entrypoint == "" {
			ok, err := kustomizationReferencesInput(repoRoot, entrypoint, file, map[string]bool{}, true)
			if err == nil && ok {
				return true
			}
			continue
		}
		if pathMatches(file, entrypoint) {
			return true
		}
		ok, err := kustomizationReferencesInput(repoRoot, entrypoint, file, map[string]bool{}, true)
		if err == nil && ok {
			return true
		}
	}
	return false
}

func sortedTargets(seen map[string]Target) []Target {
	out := make([]Target, 0, len(seen))
	for _, target := range seen {
		out = append(out, target)
	}
	sort.Slice(out, func(i, j int) bool {
		a := out[i].EntryPoint + "\t" + out[i].Namespace + "\t" + out[i].Name
		b := out[j].EntryPoint + "\t" + out[j].Namespace + "\t" + out[j].Name
		return a < b
	})
	return out
}

func findKustomizationFiles(repoRoot string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if name != "kustomization.yaml" && name != "kustomization.yml" && name != "Kustomization" {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out, err
}

func affectedByFile(repoRoot, changed, entrypoint string, raw rawKustomization, rendered resourceDoc) (bool, string, error) {
	var ok bool
	var err error
	if rendered.Spec.Path != "" {
		specPath := normalizeSpecPath(rendered.Spec.Path)
		if pathMatches(changed, specPath) {
			return true, "spec.path", nil
		}
		ok, err = kustomizationReferencesInput(repoRoot, specPath, changed, map[string]bool{}, true)
		if err != nil {
			return false, "", err
		}
		if ok {
			return true, "spec.path-input", nil
		}
	}
	if pathMatches(changed, raw.File) {
		return true, "manifest", nil
	}
	ok, err = kustomizationBuildInputReferencesInput(repoRoot, entrypoint, changed, raw.Doc, rendered)
	if err != nil {
		return false, "", err
	}
	if ok {
		return true, "entrypoint-input", nil
	}
	ok, err = dependencyMatchesInput(repoRoot, changed, entrypoint, raw.Doc, rendered)
	if err != nil {
		return false, "", err
	}
	if ok {
		return true, "dependency", nil
	}
	return false, "", nil
}

func renderedKustomization(repoRoot, entrypoint string, raw resourceDoc) (resourceDoc, error) {
	resources, err := loader.LoadPath(filesys.MakeFsOnDisk(), filepath.Join(repoRoot, filepath.FromSlash(entrypoint)))
	if err != nil {
		return resourceDoc{}, err
	}
	var matches []resourceDoc
	for _, res := range resources {
		var doc resourceDoc
		if err := controller.NewResource(res).Unmarshal(&doc); err != nil {
			return resourceDoc{}, err
		}
		if !isFluxKustomization(doc) {
			continue
		}
		if doc.Spec.SourceRef.Name == "" {
			continue
		}
		if raw.Spec.SourceRef.Kind != "" && doc.Spec.SourceRef.Kind != raw.Spec.SourceRef.Kind {
			continue
		}
		if raw.Spec.SourceRef.Namespace != "" && doc.Spec.SourceRef.Namespace != raw.Spec.SourceRef.Namespace {
			continue
		}
		if !renderedNameMatchesRawTransform(doc.Metadata.Name, raw.Metadata.Name, doc.Spec.SourceRef.Name, raw.Spec.SourceRef.Name) {
			continue
		}
		matches = append(matches, doc)
	}
	namespaceMatches := matches[:0]
	for _, match := range matches {
		if defaultNamespace(match.Metadata.Namespace) == defaultNamespace(raw.Metadata.Namespace) {
			namespaceMatches = append(namespaceMatches, match)
		}
	}
	if len(namespaceMatches) > 0 {
		matches = namespaceMatches
	}
	if len(matches) > 1 {
		sort.Slice(matches, func(i, j int) bool {
			return len(matches[i].Metadata.Name) < len(matches[j].Metadata.Name)
		})
		if len(matches[0].Metadata.Name) < len(matches[1].Metadata.Name) {
			return matches[0], nil
		}
	}
	if len(matches) != 1 {
		if len(matches) == 0 {
			return resourceDoc{}, errRenderedKustomizationNotFound
		}
		return resourceDoc{}, fmt.Errorf("rendered kustomization match count = %d", len(matches))
	}
	return matches[0], nil
}

func dependencyMatchesInput(repoRoot, changed, entrypoint string, raw, rendered resourceDoc) (bool, error) {
	changedPath := filepath.Join(repoRoot, filepath.FromSlash(changed))
	if !isYAML(changedPath) {
		return false, nil
	}
	changedDocs, err := loadDocs(changedPath)
	if err != nil {
		return false, nil
	}
	rawResources := make([]resourceDoc, 0, len(changedDocs))
	for _, doc := range changedDocs {
		if doc.Kind != "" && doc.Metadata.Name != "" {
			rawResources = append(rawResources, doc)
		}
	}
	if len(rawResources) == 0 {
		return false, nil
	}

	renderedResources, _ := loader.LoadPath(filesys.MakeFsOnDisk(), filepath.Join(repoRoot, filepath.FromSlash(entrypoint)))
	for _, res := range renderedResources {
		var doc resourceDoc
		if err := controller.NewResource(res).Unmarshal(&doc); err != nil {
			return false, err
		}
		for _, rawResource := range rawResources {
			if doc.Kind != rawResource.Kind {
				continue
			}
			if !renderedDependencyNamespaceMatches(rawResource.Metadata.Namespace, doc.Metadata.Namespace, raw.Metadata.Namespace, rendered.Metadata.Namespace) {
				continue
			}
			transformed := renderNameWithRawTransform(rendered.Metadata.Name, raw.Metadata.Name, rawResource.Metadata.Name)
			if doc.Metadata.Name == rawResource.Metadata.Name || doc.Metadata.Name == transformed {
				rawResources = append(rawResources, doc)
				break
			}
		}
	}

	deps := dependencies(raw)
	for _, dep := range deps {
		if dep.Namespace == "__KUSTOMIZATION_NAMESPACE__" {
			dep.Namespace = defaultNamespace(rendered.Metadata.Namespace)
		}
		transformed := renderNameWithRawTransform(rendered.Metadata.Name, raw.Metadata.Name, dep.Name)
		for _, res := range rawResources {
			ns := defaultNamespace(res.Metadata.Namespace)
			if dep.Namespace == ns && dep.Kind == res.Kind && (dep.Name == res.Metadata.Name || transformed == res.Metadata.Name) {
				return true, nil
			}
		}
	}
	return false, nil
}

func renderedDependencyNamespaceMatches(rawResourceNamespace, renderedResourceNamespace, rawKustomizationNamespace, renderedKustomizationNamespace string) bool {
	if rawResourceNamespace == "" || renderedResourceNamespace == rawResourceNamespace {
		return true
	}
	if rawKustomizationNamespace == renderedKustomizationNamespace {
		return false
	}
	return renderedResourceNamespace == renderedKustomizationNamespace
}

func dependencies(doc resourceDoc) []kustomizationRef {
	var out []kustomizationRef
	if doc.Spec.SourceRef.Kind != "" && doc.Spec.SourceRef.Name != "" {
		ns := doc.Spec.SourceRef.Namespace
		if ns == "" {
			ns = "__KUSTOMIZATION_NAMESPACE__"
		}
		out = append(out, kustomizationRef{Namespace: ns, Kind: doc.Spec.SourceRef.Kind, Name: doc.Spec.SourceRef.Name})
	}
	if doc.Spec.PostBuild != nil {
		for _, ref := range doc.Spec.PostBuild.SubstituteFrom {
			kind := ref.Kind
			if kind == "" {
				kind = "ConfigMap"
			}
			out = append(out, kustomizationRef{Namespace: "__KUSTOMIZATION_NAMESPACE__", Kind: kind, Name: ref.Name})
		}
	}
	for _, ref := range doc.Spec.DependsOn {
		if ref.Name == "" {
			continue
		}
		ns := ref.Namespace
		if ns == "" {
			ns = "__KUSTOMIZATION_NAMESPACE__"
		}
		out = append(out, kustomizationRef{Namespace: ns, Kind: "Kustomization", Name: ref.Name})
	}
	return out
}

func kustomizationReferencesInput(repoRoot, dir, input string, seen map[string]bool, includeResources bool) (bool, error) {
	kfile, ok := findKustomizationFile(repoRoot, dir)
	if !ok {
		return false, nil
	}
	if seen[kfile] {
		return false, nil
	}
	seen[kfile] = true
	if pathMatches(input, kfile) {
		return true, nil
	}
	refs, err := kustomizeRefs(filepath.Join(repoRoot, filepath.FromSlash(kfile)), includeResources)
	if err != nil {
		return false, err
	}
	for _, ref := range refs {
		if isRemoteRef(ref) {
			continue
		}
		resolved := resolveRef(repoRoot, dir, ref)
		if pathMatches(input, resolved) {
			return true, nil
		}
		if info, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(resolved))); err == nil && info.IsDir() {
			ok, err := kustomizationReferencesInput(repoRoot, resolved, input, seen, includeResources)
			if err != nil || ok {
				return ok, err
			}
		}
	}
	return false, nil
}

func kustomizationBuildInputReferencesInput(repoRoot, dir, input string, raw, rendered resourceDoc) (bool, error) {
	return kustomizationBuildInputReferencesInputRecursive(repoRoot, dir, input, raw, rendered, map[string]bool{})
}

func kustomizationBuildInputReferencesInputRecursive(repoRoot, dir, input string, raw, rendered resourceDoc, seen map[string]bool) (bool, error) {
	kfile, ok := findKustomizationFile(repoRoot, dir)
	if !ok {
		return false, nil
	}
	if seen[kfile] {
		return false, nil
	}
	seen[kfile] = true
	if pathMatches(input, kfile) {
		return true, nil
	}
	refs, err := kustomizeRefs(filepath.Join(repoRoot, filepath.FromSlash(kfile)), false)
	if err != nil {
		return false, err
	}
	for _, ref := range refs {
		if isRemoteRef(ref) {
			continue
		}
		resolved := resolveRef(repoRoot, dir, ref)
		if pathMatches(input, resolved) {
			return true, nil
		}
		if info, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(resolved))); err == nil && info.IsDir() {
			ok, err := kustomizationBuildInputReferencesInputRecursive(repoRoot, resolved, input, raw, rendered, seen)
			if err != nil || ok {
				return ok, err
			}
		}
	}
	resourceRefs, err := kustomizeResourceRefs(filepath.Join(repoRoot, filepath.FromSlash(kfile)))
	if err != nil {
		return false, err
	}
	for _, ref := range resourceRefs {
		if isRemoteRef(ref) {
			continue
		}
		resolved := resolveRef(repoRoot, dir, ref)
		info, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(resolved)))
		if err == nil {
			if info.IsDir() {
				ok, err := kustomizationBuildInputReferencesInputRecursive(repoRoot, resolved, input, raw, rendered, seen)
				if err != nil || ok {
					return ok, err
				}
				continue
			}
			if pathMatches(input, resolved) && !isDependencyMetadataForKustomization(repoRoot, resolved, raw, rendered) {
				return true, nil
			}
			continue
		}
		if normalizeSpecPath(input) == normalizeSpecPath(resolved) {
			return true, nil
		}
	}
	return false, nil
}

func isDependencyMetadataForKustomization(repoRoot, path string, raw, rendered resourceDoc) bool {
	if !isYAML(path) {
		return false
	}
	docs, err := loadDocs(filepath.Join(repoRoot, filepath.FromSlash(path)))
	if err != nil {
		return false
	}
	for _, doc := range docs {
		if !isDependencyMetadata(doc) {
			continue
		}
		for _, dep := range dependencies(raw) {
			ns := dep.Namespace
			if ns == "__KUSTOMIZATION_NAMESPACE__" {
				ns = defaultNamespace(rendered.Metadata.Namespace)
			}
			if ns == defaultNamespace(doc.Metadata.Namespace) && dep.Kind == doc.Kind && dep.Name == doc.Metadata.Name {
				return true
			}
		}
	}
	return false
}

func isDependencyMetadata(doc resourceDoc) bool {
	if doc.Kind == "ConfigMap" || doc.Kind == "Secret" || isFluxKustomization(doc) {
		return true
	}
	return strings.HasSuffix(doc.Kind, "Repository")
}

func kustomizeRefs(path string, includeResources bool) ([]string, error) {
	k, ok, err := loadKustomizeFile(path)
	if err != nil || !ok {
		return nil, err
	}
	var refs []string
	if includeResources {
		refs = append(refs, kustomizeResourceRefsFromFile(k)...)
	}
	refs = append(refs, k.Configurations...)
	refs = append(refs, k.Generators...)
	refs = append(refs, k.Transformers...)
	refs = append(refs, k.Validators...)
	refs = append(refs, k.PatchesStrategicMerge...)
	for _, patch := range k.PatchesJson6902 {
		refs = append(refs, patch.Path)
	}
	for _, patch := range k.Patches {
		refs = append(refs, patch.Path)
	}
	for _, replacement := range k.Replacements {
		refs = append(refs, replacement.Path)
	}
	for _, chart := range k.HelmCharts {
		refs = append(refs, chart.ValuesFile)
		refs = append(refs, chart.AdditionalValuesFiles...)
	}
	for _, gen := range k.ConfigMapGenerator {
		refs = append(refs, gen.Files...)
		refs = append(refs, gen.Envs...)
	}
	for _, gen := range k.SecretGenerator {
		refs = append(refs, gen.Files...)
		refs = append(refs, gen.Envs...)
	}
	refs = append(refs, k.OpenAPI.Path)
	return filterEmptyRefs(refs), nil
}

func kustomizeResourceRefs(path string) ([]string, error) {
	k, ok, err := loadKustomizeFile(path)
	if err != nil || !ok {
		return nil, err
	}
	return filterEmptyRefs(kustomizeResourceRefsFromFile(k)), nil
}

func loadKustomizeFile(path string) (kustomizeFile, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return kustomizeFile{}, false, err
	}
	resources, err := loader.LoadBytes(data)
	if err != nil {
		return kustomizeFile{}, false, err
	}
	if len(resources) == 0 {
		return kustomizeFile{}, false, nil
	}
	var k kustomizeFile
	if err := controller.NewResource(resources[0]).Unmarshal(&k); err != nil {
		return kustomizeFile{}, false, err
	}
	return k, true, nil
}

func kustomizeResourceRefsFromFile(k kustomizeFile) []string {
	var refs []string
	refs = append(refs, k.Resources...)
	refs = append(refs, k.Bases...)
	refs = append(refs, k.Components...)
	refs = append(refs, k.CRDs...)
	return refs
}

func filterEmptyRefs(refs []string) []string {
	filtered := refs[:0]
	for _, ref := range refs {
		if ref != "" {
			filtered = append(filtered, ref)
		}
	}
	return filtered
}

func findKustomizationFile(repoRoot, dir string) (string, bool) {
	for _, name := range []string{"kustomization.yaml", "kustomization.yml", "Kustomization"} {
		path := filepath.ToSlash(filepath.Join(dir, name))
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(path))); err == nil {
			return path, true
		}
	}
	return "", false
}

func resolveRef(repoRoot, baseDir, ref string) string {
	if strings.Contains(ref, "=") {
		_, value, ok := strings.Cut(ref, "=")
		if ok {
			ref = value
		}
	}
	var path string
	if filepath.IsAbs(ref) {
		path = strings.TrimLeft(filepath.ToSlash(ref), "/")
	} else {
		path = filepath.ToSlash(filepath.Join(baseDir, filepath.FromSlash(ref)))
	}
	full := filepath.Join(repoRoot, filepath.FromSlash(path))
	if _, err := os.Stat(full); err == nil {
		if rel, relErr := filepath.Rel(repoRoot, full); relErr == nil {
			return filepath.ToSlash(filepath.Clean(rel))
		}
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func isRemoteRef(ref string) bool {
	return strings.Contains(ref, "://") || strings.HasPrefix(ref, "git::") || strings.HasPrefix(ref, "github.com/")
}

func normalizeSpecPath(path string) string {
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimLeft(path, "/")
	path = strings.TrimSuffix(path, "/")
	path = filepath.ToSlash(filepath.Clean(path))
	if path == "." {
		return ""
	}
	return path
}

func pathMatches(input, match string) bool {
	match = normalizeSpecPath(match)
	input = filepath.ToSlash(filepath.Clean(input))
	return match == "" || input == match || strings.HasPrefix(input, match+"/")
}

func hasPathPrefix(path, prefix string) bool {
	return prefix == "" || path == prefix || strings.HasPrefix(path, prefix+"/")
}

func pathDepth(path string) int {
	if path == "" {
		return 0
	}
	return strings.Count(path, "/") + 1
}

func defaultNamespace(ns string) string {
	if ns == "" {
		return "flux-system"
	}
	return ns
}

func isYAML(path string) bool {
	return strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")
}

func renderedNameMatchesRawTransform(renderedName, rawName, renderedPeerName, rawPeerName string) bool {
	if rawName == "" {
		return false
	}
	if renderedName == rawName {
		return true
	}
	if rawPeerName != "" && renderedPeerName != rawPeerName {
		if !strings.Contains(renderedPeerName, rawPeerName) {
			return false
		}
		prefix, suffix := splitRenderedName(renderedPeerName, rawPeerName)
		return renderedName == prefix+rawName+suffix
	}
	return strings.Contains(renderedName, rawName)
}

func renderNameWithRawTransform(renderedName, rawName, rawPeerName string) string {
	if rawName == "" || renderedName == rawName || !strings.Contains(renderedName, rawName) {
		return rawPeerName
	}
	prefix, suffix := splitRenderedName(renderedName, rawName)
	return prefix + rawPeerName + suffix
}

func splitRenderedName(renderedName, rawName string) (string, string) {
	before, after, _ := strings.Cut(renderedName, rawName)
	return before, after
}
