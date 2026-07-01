package affected

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func writeFile(t *testing.T, root, name, data string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func targetStrings(targets []Target) []string {
	out := make([]string, 0, len(targets))
	for _, target := range targets {
		out = append(out, filepath.ToSlash(target.EntryPoint)+"\t"+target.Namespace+"\t"+target.Name)
	}
	sort.Strings(out)
	return out
}

func requireTargets(t *testing.T, got []Target, want ...string) {
	t.Helper()
	gotStrings := targetStrings(got)
	sort.Strings(want)
	if len(gotStrings) != len(want) {
		t.Fatalf("targets = %v, want %v", gotStrings, want)
	}
	for i := range want {
		if gotStrings[i] != want[i] {
			t.Fatalf("targets = %v, want %v", gotStrings, want)
		}
	}
}

func TestFindIncludesAllExternalEntrypoints(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - clusters/prod
  - regions/eu/stage
`)
	writeFile(t, root, "clusters/prod/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: gitops
namePrefix: prod-
resources:
  - ../../apps/demo/flux
`)
	writeFile(t, root, "regions/eu/stage/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: gitops
namePrefix: stage-
resources:
  - ../../../apps/demo/flux
`)
	writeFile(t, root, "apps/demo/flux/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - service.yaml
`)
	writeFile(t, root, "apps/demo/flux/service.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: shared-service
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/demo/service
`)
	writeFile(t, root, "apps/demo/service/deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
`)

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"apps/demo/service/deployment.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets,
		filepath.Join(root, "clusters/prod")+"\tgitops\tprod-shared-service",
		filepath.Join(root, "regions/eu/stage")+"\tgitops\tstage-shared-service",
	)
}

func TestFindUsesNearestAncestorEntrypoint(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - clusters/prod
`)
	writeFile(t, root, "clusters/prod/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: gitops
namePrefix: prod-
resources:
  - service.yaml
`)
	writeFile(t, root, "clusters/prod/service.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: service
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/demo/service
`)
	writeFile(t, root, "apps/demo/service/deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
`)

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"apps/demo/service/deployment.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets,
		filepath.Join(root, "clusters/prod")+"\tgitops\tprod-service",
	)
}

func TestFindRendersAlternateEntrypointKustomizationFilenames(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clusters/prod/kustomization.yml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: gitops
namePrefix: prod-
resources:
  - service.yaml
`)
	writeFile(t, root, "clusters/prod/service.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: service
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/demo/service
`)
	writeFile(t, root, "apps/demo/service/deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
`)

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"apps/demo/service/deployment.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets,
		filepath.Join(root, "clusters/prod")+"\tgitops\tprod-service",
	)
}

func TestFindTracksEntrypointYamlBuildInputs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clusters/prod/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - service.yaml
patches:
  - path: labels.yaml
`)
	writeFile(t, root, "clusters/prod/service.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: entrypoint-service
  namespace: demo
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/demo/service
`)
	writeFile(t, root, "clusters/prod/labels.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: entrypoint-service
  namespace: demo
  labels:
    patched: "true"
`)

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"clusters/prod/labels.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets,
		filepath.Join(root, "clusters/prod")+"\tdemo\tentrypoint-service",
	)
}

func TestFindMatchesRenderedSpecPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clusters/prod/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: gitops
namePrefix: prod-
resources:
  - service.yaml
patches:
  - path: path-patch.yaml
`)
	writeFile(t, root, "clusters/prod/service.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: service
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/raw/service
`)
	writeFile(t, root, "clusters/prod/path-patch.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: service
spec:
  path: ./apps/rendered/service
`)
	writeFile(t, root, "apps/rendered/service/deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
`)

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"apps/rendered/service/deployment.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets,
		filepath.Join(root, "clusters/prod")+"\tgitops\tprod-service",
	)
}

func TestFindKeepsKustomizationsWithRenderedSourceRef(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clusters/prod/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: gitops
namePrefix: prod-
resources:
  - service.yaml
patches:
  - target:
      group: kustomize.toolkit.fluxcd.io
      version: v1
      kind: Kustomization
      name: service
    patch: |-
      - op: add
        path: /spec/sourceRef
        value:
          kind: GitRepository
          name: app-source
`)
	writeFile(t, root, "clusters/prod/service.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: service
spec:
  path: ./apps/demo/service
`)
	writeFile(t, root, "apps/demo/service/deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
`)

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"apps/demo/service/deployment.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets,
		filepath.Join(root, "clusters/prod")+"\tgitops\tprod-service",
	)
}

func TestFindTracksDirectEntrypointResourceFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clusters/prod/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: gitops
namePrefix: prod-
resources:
  - service.yaml
  - sidecar.yaml
`)
	writeFile(t, root, "clusters/prod/service.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: service
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/demo/service
`)
	writeFile(t, root, "clusters/prod/sidecar.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sidecar
spec:
  selector:
    matchLabels:
      app: sidecar
  template:
    metadata:
      labels:
        app: sidecar
    spec:
      containers:
        - name: sidecar
          image: example/sidecar:latest
`)

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"clusters/prod/sidecar.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets,
		filepath.Join(root, "clusters/prod")+"\tgitops\tprod-service",
	)
}

func TestFindTracksPlainConfigMapEntrypointResourceFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clusters/prod/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: gitops
namePrefix: prod-
resources:
  - service.yaml
  - config.yaml
`)
	writeFile(t, root, "clusters/prod/service.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: service
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/demo/service
`)
	writeFile(t, root, "clusters/prod/config.yaml", `
apiVersion: v1
kind: ConfigMap
metadata:
  name: plain-config
data:
  enabled: "true"
`)

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"clusters/prod/config.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets,
		filepath.Join(root, "clusters/prod")+"\tgitops\tprod-service",
	)
}

func TestFindDoesNotSuppressDirectResourceMatchForUnrelatedTargets(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clusters/prod/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: gitops
namePrefix: prod-
resources:
  - dependent.yaml
  - unrelated.yaml
  - app-vars.yaml
`)
	writeFile(t, root, "clusters/prod/dependent.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: dependent
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  postBuild:
    substituteFrom:
      - kind: ConfigMap
        name: app-vars
  path: ./apps/dependent
`)
	writeFile(t, root, "clusters/prod/unrelated.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: unrelated
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/unrelated
`)
	writeFile(t, root, "clusters/prod/app-vars.yaml", `
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-vars
data:
  IMAGE_TAG: stable
`)

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"clusters/prod/app-vars.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets,
		filepath.Join(root, "clusters/prod")+"\tgitops\tprod-dependent",
		filepath.Join(root, "clusters/prod")+"\tgitops\tprod-unrelated",
	)
}

func TestFindDoesNotSuppressDirectResourceMatchForDifferentNamespaceDependency(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clusters/prod/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namePrefix: prod-
resources:
  - service.yaml
  - team-b-vars.yaml
`)
	writeFile(t, root, "clusters/prod/service.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: service
  namespace: team-a
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  postBuild:
    substituteFrom:
      - kind: ConfigMap
        name: app-vars
  path: ./apps/service
`)
	writeFile(t, root, "clusters/prod/team-b-vars.yaml", `
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-vars
  namespace: team-b
data:
  IMAGE_TAG: stable
`)

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"clusters/prod/team-b-vars.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets,
		filepath.Join(root, "clusters/prod")+"\tteam-a\tprod-service",
	)
}

func TestFindTracksNestedEntrypointBuildInputs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clusters/prod/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: gitops
namePrefix: prod-
resources:
  - ../../apps/foo/overlay
`)
	writeFile(t, root, "apps/foo/overlay/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - service.yaml
patches:
  - path: patch.yaml
`)
	writeFile(t, root, "apps/foo/overlay/service.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: foo-service
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/foo/source
`)
	writeFile(t, root, "apps/foo/overlay/patch.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: foo-service
  labels:
    patched: "true"
`)

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"apps/foo/overlay/patch.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets,
		filepath.Join(root, "clusters/prod")+"\tgitops\tprod-foo-service",
	)
}

func TestFindTracksTargetManifestEditsReferencedByEntrypoint(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clusters/prod/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: gitops
namePrefix: prod-
resources:
  - service.yaml
`)
	writeFile(t, root, "clusters/prod/service.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: service
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/demo/service
`)

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"clusters/prod/service.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets,
		filepath.Join(root, "clusters/prod")+"\tgitops\tprod-service",
	)
}

func TestFindMatchesRenderedKustomizationsByOwnName(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clusters/prod/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: gitops
namePrefix: prod-
resources:
  - alpha.yaml
  - beta.yaml
`)
	writeFile(t, root, "clusters/prod/alpha.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: alpha-service
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/demo/service
`)
	writeFile(t, root, "clusters/prod/beta.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: beta-service
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/demo/service
`)
	writeFile(t, root, "apps/demo/service/deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
`)

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"apps/demo/service/deployment.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets,
		filepath.Join(root, "clusters/prod")+"\tgitops\tprod-alpha-service",
		filepath.Join(root, "clusters/prod")+"\tgitops\tprod-beta-service",
	)
}

func TestFindDoesNotMatchRenderedKustomizationsBySubstringName(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clusters/prod/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: gitops
namePrefix: prod-
resources:
  - app.yaml
  - my-app.yaml
`)
	writeFile(t, root, "clusters/prod/app.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: app
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/demo/service
`)
	writeFile(t, root, "clusters/prod/my-app.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: my-app
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/demo/service
`)
	writeFile(t, root, "apps/demo/service/deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
`)

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"apps/demo/service/deployment.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets,
		filepath.Join(root, "clusters/prod")+"\tgitops\tprod-app",
		filepath.Join(root, "clusters/prod")+"\tgitops\tprod-my-app",
	)
}

func TestFindMatchesRenderedKustomizationsByNamespace(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clusters/prod/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namePrefix: prod-
resources:
  - team-a.yaml
  - team-b.yaml
`)
	writeFile(t, root, "clusters/prod/team-a.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: svc
  namespace: team-a
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/demo/service
`)
	writeFile(t, root, "clusters/prod/team-b.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: svc
  namespace: team-b
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/demo/service
`)
	writeFile(t, root, "apps/demo/service/deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
`)

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"apps/demo/service/deployment.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets,
		filepath.Join(root, "clusters/prod")+"\tteam-a\tprod-svc",
		filepath.Join(root, "clusters/prod")+"\tteam-b\tprod-svc",
	)
}

func TestFindMatchesRenderedKustomizationNamespaceTransforms(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clusters/prod/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: gitops
namePrefix: prod-
resources:
  - service.yaml
`)
	writeFile(t, root, "clusters/prod/service.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: svc
  namespace: stale
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/demo/service
`)
	writeFile(t, root, "apps/demo/service/deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
`)

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"apps/demo/service/deployment.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets,
		filepath.Join(root, "clusters/prod")+"\tgitops\tprod-svc",
	)
}

func TestFindMatchesRenderedDependencyNameTransforms(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clusters/prod/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: demo
namePrefix: prod-
resources:
  - service.yaml
  - app-vars.yaml
`)
	writeFile(t, root, "clusters/prod/service.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: rendered-dependency-service
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  postBuild:
    substituteFrom:
      - kind: ConfigMap
        name: app-vars
  path: ./apps/demo/service
`)
	writeFile(t, root, "clusters/prod/app-vars.yaml", `
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-vars
data:
  IMAGE_TAG: stable
`)

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"clusters/prod/app-vars.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets,
		filepath.Join(root, "clusters/prod")+"\tdemo\tprod-rendered-dependency-service",
	)
}

func TestFindMatchesRenderedDependencyNamespaceTransforms(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clusters/prod/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: gitops
namePrefix: prod-
resources:
  - service.yaml
  - app-vars.yaml
`)
	writeFile(t, root, "clusters/prod/service.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: service
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  postBuild:
    substituteFrom:
      - kind: ConfigMap
        name: app-vars
  path: ./apps/demo/service
`)
	writeFile(t, root, "clusters/prod/app-vars.yaml", `
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-vars
  namespace: stale
data:
  IMAGE_TAG: stable
`)

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"clusters/prod/app-vars.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets,
		filepath.Join(root, "clusters/prod")+"\tgitops\tprod-service",
	)
}

func TestFindPreservesNamespaceWhenMatchingRenderedDependencies(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clusters/prod/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namePrefix: prod-
resources:
  - team-a-service.yaml
  - team-b-service.yaml
  - team-a-vars.yaml
  - team-b-vars.yaml
`)
	writeFile(t, root, "clusters/prod/team-a-service.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: service
  namespace: team-a
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  postBuild:
    substituteFrom:
      - kind: ConfigMap
        name: app-vars
  path: ./apps/team-a/service
`)
	writeFile(t, root, "clusters/prod/team-b-service.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: service
  namespace: team-b
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  postBuild:
    substituteFrom:
      - kind: ConfigMap
        name: app-vars
  path: ./apps/team-b/service
`)
	writeFile(t, root, "clusters/prod/team-a-vars.yaml", `
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-vars
  namespace: team-a
data:
  IMAGE_TAG: stable
`)
	writeFile(t, root, "clusters/prod/team-b-vars.yaml", `
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-vars
  namespace: team-b
data:
  IMAGE_TAG: stable
`)

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"clusters/prod/team-a-vars.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets,
		filepath.Join(root, "clusters/prod")+"\tteam-a\tprod-service",
		filepath.Join(root, "clusters/prod")+"\tteam-b\tprod-service",
	)
}

func TestFindMatchesFluxDependsOnReferences(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clusters/prod/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: gitops
namePrefix: prod-
resources:
  - infra.yaml
  - app.yaml
`)
	writeFile(t, root, "clusters/prod/infra.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: infra
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/infra
`)
	writeFile(t, root, "clusters/prod/app.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: app
spec:
  dependsOn:
    - name: infra
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/app
`)

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"clusters/prod/infra.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets,
		filepath.Join(root, "clusters/prod")+"\tgitops\tprod-app",
		filepath.Join(root, "clusters/prod")+"\tgitops\tprod-infra",
	)
}

func TestFindDoesNotTreatMissingOpenAPIPathAsDirectoryInput(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clusters/prod/kustomization.yaml", `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - service.yaml
`)
	writeFile(t, root, "clusters/prod/service.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: service
  namespace: demo
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/demo/service
`)
	writeFile(t, root, "clusters/prod/README.md", "notes\n")

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"clusters/prod/README.md"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets)
}

func TestFindIgnoresNonManifestChangedFilesWithoutDependencyMetadata(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clusters/prod/service.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: service
  namespace: demo
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/demo/service
`)
	writeFile(t, root, "scripts/update.sh", "#!/bin/sh\n")

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"scripts/update.sh", "deleted.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets)
}

func TestFindIgnoresNonResourceYAMLDuringDiscovery(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "clusters/prod/service.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: service
  namespace: demo
spec:
  sourceRef:
    kind: GitRepository
    name: app-source
  path: ./apps/demo/service
`)
	writeFile(t, root, "charts/demo/values.yaml", `
- image:
    tag: latest
`)
	writeFile(t, root, "apps/demo/service/deployment.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: demo
`)

	targets, err := Find(Options{
		RepoRoot: root,
		Files:    []string{"apps/demo/service/deployment.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	requireTargets(t, targets,
		filepath.Join(root, "clusters/prod")+"\tdemo\tservice",
	)
}
