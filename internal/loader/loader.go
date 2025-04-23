package loader

import (
	"path/filepath"
	"strings"

	"sigs.k8s.io/kustomize/api/konfig"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/api/resource"
	kustypes "sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	"sigs.k8s.io/kustomize/kyaml/kio"
)

var kustomizer = krusty.MakeKustomizer(&krusty.Options{
	LoadRestrictions: kustypes.LoadRestrictionsNone,
	PluginConfig:     kustypes.DisabledPluginConfig(),
})

// LoadPath loads all resources under path from fs recursively.
func LoadPath(
	fs filesys.FileSystem,
	path string,
) (
	[]*resource.Resource,
	error,
) {

	// Kustomization:
	if fs.Exists(filepath.Join(path, konfig.DefaultKustomizationFileName())) {
		resMap, err := kustomizer.Run(fs, path)
		if err != nil {
			return nil, err
		}
		return resMap.Resources(), nil
	}

	// Folder:
	if fs.IsDir(path) {
		entries, err := fs.ReadDir(path)
		if err != nil {
			return nil, err
		}

		// Regular Folder:
		resources := make([]*resource.Resource, 0, len(entries))
		for i := range entries {
			res, err := LoadPath(fs, filepath.Join(path, entries[i]))
			if err != nil {
				return nil, err
			}
			resources = append(resources, res...)
		}
		return resources, nil
	}

	// Skip non YAML files:
	if !(strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")) {
		return nil, nil
	}

	// YAML file:
	data, err := fs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadBytes(data)
}

func LoadBytes(data []byte) ([]*resource.Resource, error) {
	docs, err := kio.FromBytes(data)
	if err != nil {
		return nil, err
	}
	resources := make([]*resource.Resource, len(docs))
	for i, doc := range docs {
		resources[i] = &resource.Resource{
			RNode: *doc,
		}
	}
	return resources, nil
}
