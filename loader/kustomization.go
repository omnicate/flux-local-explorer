package loader

import (
	"context"
	"fmt"
	"sort"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"github.com/fluxcd/pkg/kustomize"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	sigyaml "sigs.k8s.io/yaml"
)

func (l *Loader) handleKustomization(
	queue *Queue,
	ks *kustomizev1.Kustomization,
) (*Kustomization, error) {

	// Load the root exactly once:
	if l.root == nil {
		l.root = ks
	} else if ks.Namespace == l.root.Namespace && ks.Name == l.root.Name {
		return nil, ErrSkip
	}

	repoName := fmt.Sprintf(
		"%s/%s/%s",
		ks.Spec.SourceRef.Kind,
		orDefault(ks.Spec.SourceRef.Namespace, ks.Namespace),
		ks.Spec.SourceRef.Name,
	)
	repoFS, ok := l.repos[repoName]
	if !ok {
		return nil, fmt.Errorf("could not find source: %s", repoName)
	}

	resources, err := l.loadPath(repoFS, ks.Spec.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to load path: %v", err)
	}

	// Variable substitution:
	if pb := ks.Spec.PostBuild; pb != nil && (pb.SubstituteFrom != nil || pb.Substitute != nil) {
		ksYaml, err := sigyaml.Marshal(ks)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal Kustomization: %v", err)
		}
		var ksObj unstructured.Unstructured
		if err := sigyaml.Unmarshal(ksYaml, &ksObj); err != nil {
			return nil, fmt.Errorf("failed to unmarshal Kustomization: %v", err)
		}

		for i, res := range resources {
			newRes, err := kustomize.SubstituteVariables(
				context.Background(),
				l.cs,
				ksObj,
				res,
				kustomize.SubstituteWithStrict(true),
			)
			if err != nil {
				return nil, err
			}
			resources[i] = newRes
		}
	}

	// Optimization: recurse repositories and kustomizations first:
	sort.Slice(resources, func(i, _ int) bool {
		res := resources[i]
		kind := res.GetKind()
		return kind == "GitRepository" || kind == "OCIRepository" || kind == "Kustomization"
	})

	targetNamespace := orDefault(ks.Spec.TargetNamespace, ks.Namespace)
	for _, res := range resources {
		if res.GetNamespace() == "" {
			_ = res.SetNamespace(targetNamespace)
		}
		queue.Push(&QueueItem{Value: res})
	}

	kustomization := &Kustomization{
		Kustomization: ks,
		Resources:     resources,
	}
	return kustomization, nil
}
