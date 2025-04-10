package loader

import (
	"sigs.k8s.io/kustomize/api/resource"

	intres "github.com/omnicate/flx/resource"
)

type ResultSet struct {
	Kustomizations   []*intres.Kustomization
	GitRepositories  []*intres.GitRepository
	OCIRepositories  []*intres.OCIRepository
	HelmReleases     []*intres.HelmRelease
	HelmRepositories []*intres.HelmRepository
	Resources        []*resource.Resource
}

func EmptyResultSet() *ResultSet {
	return &ResultSet{}
}

func (rs *ResultSet) Merge(other *ResultSet) {
	for _, v := range other.Kustomizations {
		rs.Kustomizations = append(rs.Kustomizations, v)
	}
	for _, v := range other.GitRepositories {
		rs.GitRepositories = append(rs.GitRepositories, v)
	}
	for _, v := range other.OCIRepositories {
		rs.OCIRepositories = append(rs.OCIRepositories, v)
	}
	for _, v := range other.HelmReleases {
		rs.HelmReleases = append(rs.HelmReleases, v)
	}
	for _, v := range other.HelmRepositories {
		rs.HelmRepositories = append(rs.HelmRepositories, v)
	}
	for _, v := range other.Resources {
		rs.Resources = append(rs.Resources, v)
	}
}

func NewResultSet(
	resources []*resource.Resource,
	defaultNamespace string,
) (
	*ResultSet,
	error,
) {
	result := EmptyResultSet()
	for _, res := range resources {
		if res.GetNamespace() == "" {
			_ = res.SetNamespace(defaultNamespace)
		}
		switch res.GetKind() {
		case intres.OCIRepositoryKind:
			hr, err := intres.NewOCIRepository(res)
			if err != nil {
				return nil, err
			}
			result.OCIRepositories = append(result.OCIRepositories, hr)
		case intres.GitRepositoryKind:
			hr, err := intres.NewGitRepository(res)
			if err != nil {
				return nil, err
			}
			result.GitRepositories = append(result.GitRepositories, hr)
		case intres.KustomizationKind:
			hr, err := intres.NewKustomization(res)
			if err != nil {
				return nil, err
			}
			result.Kustomizations = append(result.Kustomizations, hr)
		case intres.HelmRepositoryKind:
			hr, err := intres.NewHelmRepository(res)
			if err != nil {
				return nil, err
			}
			result.HelmRepositories = append(result.HelmRepositories, hr)
		case intres.HelmReleaseKind:
			hr, err := intres.NewHelmRelease(res)
			if err != nil {
				return nil, err
			}
			result.HelmReleases = append(result.HelmReleases, hr)
		}

		// All resources land in .Resources
		result.Resources = append(result.Resources, res)
	}
	return result, nil
}
