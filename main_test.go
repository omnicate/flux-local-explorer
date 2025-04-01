package main

import (
	"fmt"
	"os"
	"testing"

	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/omnicate/flx/loader"
)

func Test_Loader(t *testing.T) {
	var (
		path = os.Getenv("FLX_DIR")
	)
	if path == "" {
		t.Skip("missing env: FLX_DIR")
	}
	l := loader.NewLoader()
	diskFS := filesys.MakeFsOnDisk()
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
