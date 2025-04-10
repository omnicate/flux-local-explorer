package controller

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	ociclient "github.com/fluxcd/pkg/oci/client"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/rs/zerolog"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/omnicate/flx/fs"
	"github.com/omnicate/flx/loader"
	intres "github.com/omnicate/flx/resource"
)

type OCI struct {
	logger    zerolog.Logger
	cachePath string
	repos     map[string]filesys.FileSystem
}

func NewOCI(logger zerolog.Logger, cachePath string) *OCI {
	return &OCI{cachePath: cachePath, logger: logger, repos: map[string]filesys.FileSystem{}}
}

func (l *OCI) Get(kind, namespace, name string) (any, error) {
	if kind == "OCIRepository" {
		fileSys, ok := l.repos[namespace+"/"+name]
		if !ok {
			return nil, ErrNotFound
		}
		return fileSys, nil
	}
	return nil, ErrNotFound
}

func (l *OCI) Handle(rs *loader.ResultSet) (*loader.ResultSet, error) {
	sort.Slice(rs.GitRepositories, func(i, j int) bool {
		a, b := rs.GitRepositories[i], rs.GitRepositories[j]
		return len(a.Spec.Include) < len(b.Spec.Include)
	})
	for _, r := range rs.OCIRepositories {
		if err := l.handleOCIRepository(r); err != nil {
			r.Error = err
			continue
		}
	}
	return loader.EmptyResultSet(), nil
}

func (l *OCI) handleOCIRepository(
	or *intres.OCIRepository,
) error {
	nn := namespacedName(or)
	if _, ok := l.repos[nn]; ok {
		return nil
	}
	ociRef := ociRepoReference(or)
	if ociRef == "" {
		return fmt.Errorf("OCIRepository reference is missing")
	}
	if !strings.HasPrefix(or.Spec.URL, sourcev1b2.OCIRepositoryPrefix) {
		return fmt.Errorf("OCIRepository with invalid scheme")
	}

	url := strings.TrimPrefix(or.Spec.URL, sourcev1b2.OCIRepositoryPrefix)
	md5h := md5.Sum([]byte(url + "@" + ociRef))
	hash := hex.EncodeToString(md5h[:])
	ociRepoPath := filepath.Join(l.cachePath, or.Namespace+"-"+or.Name+"-"+hash)
	if _, err := os.Stat(ociRepoPath); os.IsNotExist(err) {
		client := ociclient.NewClient([]crane.Option{})
		_, err := client.Pull(context.Background(), url+":"+ociRef, ociRepoPath)
		if err != nil {
			return fmt.Errorf("oci pull: %w", err)
		}
	}

	l.repos[nn] = fs.KrustyFileSystem(fs.Prefix(filesys.MakeFsOnDisk(), ociRepoPath))
	return nil
}
