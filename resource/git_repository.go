package resource

import (
	"fmt"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"sigs.k8s.io/kustomize/api/resource"
)

const GitRepositoryKind = sourcev1.GitRepositoryKind

type GitRepository struct {
	*sourcev1.GitRepository

	Error error
}

type GitRepoInclude struct {
	Name      string
	Namespace string
	FromPath  string
	ToPath    string
}

func NewGitRepository(res *resource.Resource) (*GitRepository, error) {
	if res.GetKind() != GitRepositoryKind {
		return nil, fmt.Errorf("kind != GitRepository")
	}
	result := &GitRepository{}
	switch res.GetApiVersion() {
	case sourcev1.GroupVersion.String():
		if err := unmarshalInto(res, &result.GitRepository); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported git repository version %s", res.GetApiVersion())
	}
	return result, nil
}
