package loader

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ociclient "github.com/fluxcd/pkg/oci/client"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/google/go-containerregistry/pkg/crane"

	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/omnicate/flx/fs"
)

func (l *Loader) handleOCIRepository(or *sourcev1b2.OCIRepository) error {
	nn := namespacedName(or)
	if _, ok := l.ociRepositories[nn]; ok {
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
	ociRepoPath := filepath.Join(l.repoCachePath, or.Namespace+"-"+or.Name+"-"+hash)
	if _, err := os.Stat(ociRepoPath); os.IsNotExist(err) {
		client := ociclient.NewClient([]crane.Option{})
		_, err := client.Pull(context.Background(), url+":"+ociRef, ociRepoPath)
		if err != nil {
			return fmt.Errorf("oci pull: %w", err)
		}
	}

	l.ociRepositories[nn] = &OCIRepository{
		OCIRepository: or,
		FS:            fs.KrustyFileSystem(fs.Prefix(filesys.MakeFsOnDisk(), ociRepoPath)),
	}
	return nil
}

func (l *Loader) handleGitRepository(gr *sourcev1.GitRepository) error {
	nn := namespacedName(gr)
	if _, ok := l.gitRepositories[nn]; ok {
		return nil
	}
	ref := gr.Spec.Reference
	if ref == nil {
		return fmt.Errorf("git repository without reference")
	}
	if ref.Commit == "" && ref.Branch == "" && ref.Tag == "" {
		return fmt.Errorf("git repo must have a reference")
	}

	var repoFS fs.FileSystem
	remoteURL := gitHttpsURL(gr.Spec.URL)
	gitRef := gitRepoReference(gr)

	var isLocal bool
	for _, lgr := range l.repoReplace {
		if gitHttpsURL(lgr.Remote) == remoteURL && lgr.Ref() == gitRef {
			repoFS = fs.Prefix(
				filesys.MakeFsOnDisk(),
				lgr.Path,
			)
			isLocal = true
		}
	}

	// Use git filesystems for non-local repos:
	if !isLocal {
		var err error
		var gitRepoPath string
		md5h := md5.Sum([]byte(remoteURL))
		hash := hex.EncodeToString(md5h[:])
		gitRepoPath = filepath.Join(l.repoCachePath, hash)
		repoFS, err = fs.Git(
			gitRepoPath,
			remoteURL,
			gitRef,
		)
		if err != nil {
			return err
		}
	}

	// Handle includes:
	mountPoints := make([]*fs.MountPoint, 0)
	for _, include := range gr.Spec.Include {
		repoName := fmt.Sprintf(
			"%s/%s",
			gr.Namespace,
			include.GitRepositoryRef.Name,
		)
		includedRepo, ok := l.gitRepositories[repoName]
		if !ok {
			return fmt.Errorf("include %s not found", repoName)
		}
		mountPoints = append(mountPoints, &fs.MountPoint{
			Location: orDefault(include.ToPath, include.GitRepositoryRef.Name),
			Path:     include.FromPath,
			FS:       includedRepo.FS,
		})
	}
	if len(mountPoints) > 0 {
		repoFS = fs.Mount(repoFS, mountPoints...)
	}

	// Enable caching:
	{
		md5h := md5.Sum([]byte(remoteURL + gitRef))
		hash := hex.EncodeToString(md5h[:])
		repoFS = fs.Cache(repoFS, filepath.Join(l.repoCachePath, gr.Namespace+"-"+gr.Name+"-"+hash))
	}

	l.gitRepositories[nn] = &GitRepository{
		GitRepository: gr,
		FS:            fs.KrustyFileSystem(repoFS),
	}

	return nil
}

func gitHttpsURL(u string) string {
	if strings.HasPrefix(u, "https://") {
		return u
	}
	u = strings.TrimPrefix(u, "ssh://")
	if idx := strings.Index(u, "@"); idx > -1 {
		u = u[idx+1:]
	}
	return "https://" + strings.TrimSuffix(u, ".git")
}

func gitRepoReference(gr *sourcev1.GitRepository) string {
	ref := gr.Spec.Reference
	return orDefault(orDefault(ref.Commit, ref.Branch), ref.Tag)
}

func ociRepoReference(gr *sourcev1b2.OCIRepository) string {
	ref := gr.Spec.Reference
	return orDefault(orDefault(ref.Digest, ref.SemVer), ref.Tag)
}
