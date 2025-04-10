package loader

import (
	"context"
	"fmt"

	"github.com/fluxcd/pkg/kustomize"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	sigyaml "sigs.k8s.io/yaml"

	intres "github.com/omnicate/flx/resource"
)

func (l *Loader) loadKustomization(
	ks *intres.Kustomization,
) (
	*ResultSet,
	error,
) {

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

	// Parse the resulting resources:
	targetNamespace := orDefault(ks.Spec.TargetNamespace, ks.Namespace)
	resultSet, err := l.buildResultSet(resources, targetNamespace)
	if err != nil {
		return nil, err
	}

	// Handle them:
	if err := l.handleResultSet(resultSet); err != nil {
		return nil, err
	}

	ks.Resources = resultSet.Resources
	return resultSet, nil
}
