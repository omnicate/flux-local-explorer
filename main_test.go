package main

import (
	"fmt"
	"testing"

	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/omnicate/flx/loader"
)

func Test_Loader(t *testing.T) {
	l := loader.NewLoader()
	diskFS := filesys.MakeFsOnDisk()
	path := "../../omnicate/kubeconf/dub.dev.wgtwo.com/flux/flux-system/"
	if err := l.Load(diskFS, path, "flux-system", func(ks *loader.Kustomization, gr *loader.GitRepository) bool {
		return false
	}); err != nil {
		t.Fatal(err)
	}
	for _, ks := range l.Kustomizations() {
		fmt.Println(ks.Namespace, ks.Name)
	}
	fmt.Println(len(l.Kustomizations()))
}
