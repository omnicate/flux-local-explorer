package loader

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	ociclient "github.com/fluxcd/pkg/oci/client"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/rs/zerolog"

	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/omnicate/flx/fs"
	intres "github.com/omnicate/flx/resource"
)

func (l *Loader) handleOCIRepository(or *intres.OCIRepository) (*intres.OCIRepository, error) {
	nn := "OCIRepository/" + namespacedName(or)
	if _, ok := l.repos[nn]; ok {
		return nil, ErrSkip
	}
	ociRef := ociRepoReference(or)
	if ociRef == "" {
		return nil, fmt.Errorf("OCIRepository reference is missing")
	}
	if !strings.HasPrefix(or.Spec.URL, sourcev1b2.OCIRepositoryPrefix) {
		return nil, fmt.Errorf("OCIRepository with invalid scheme")
	}

	url := strings.TrimPrefix(or.Spec.URL, sourcev1b2.OCIRepositoryPrefix)
	md5h := md5.Sum([]byte(url + "@" + ociRef))
	hash := hex.EncodeToString(md5h[:])
	ociRepoPath := filepath.Join(l.repoCachePath, or.Namespace+"-"+or.Name+"-"+hash)
	if _, err := os.Stat(ociRepoPath); os.IsNotExist(err) {
		client := ociclient.NewClient([]crane.Option{})
		_, err := client.Pull(context.Background(), url+":"+ociRef, ociRepoPath)
		if err != nil {
			return nil, fmt.Errorf("oci pull: %w", err)
		}
	}

	l.repos[nn] = fs.KrustyFileSystem(fs.Prefix(filesys.MakeFsOnDisk(), ociRepoPath))
	return or, nil
}

func (l *Loader) handleGitRepository(
	logger zerolog.Logger,
	gr *intres.GitRepository,
) (*intres.GitRepository, error) {
	nn := "GitRepository/" + namespacedName(gr)
	if _, ok := l.repos[nn]; ok {
		return nil, ErrSkip
	}

	var remoteURL string
	var err error
	if l.gitViaHTTPS {
		remoteURL, err = gitHttpsURL(gr.Spec.URL)
	} else {
		remoteURL, err = gitSSHUrl(gr.Spec.URL)
	}
	if err != nil {
		return nil, fmt.Errorf("git url: %w", err)
	}

	logger.Debug().
		Str("url", remoteURL).
		Str("ref", gitRepoReference(gr)).
		Msg("loading git repository")

	ref := gr.Spec.Reference
	if ref == nil {
		return nil, fmt.Errorf("git repository without reference")
	}
	if ref.Commit == "" && ref.Branch == "" && ref.Tag == "" {
		return nil, fmt.Errorf("git repo must have a reference")
	}

	var repoFS fs.FileSystem
	gitRef := gitRepoReference(gr)

	var isLocal bool
	for _, lgr := range l.repoReplace {
		equals, err := gitURLEquals(lgr.Remote, remoteURL)
		if err != nil {
			return nil, fmt.Errorf("git url equals: %w", err)
		}
		if equals && lgr.Ref() == gitRef {
			repoFS = fs.Prefix(
				filesys.MakeFsOnDisk(),
				lgr.Path,
			)
			isLocal = true
			logger.Debug().
				Str("url", gr.Spec.URL).
				Str("local", lgr.Path).
				Msg("using local file system")
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
			return nil, err
		}
	}

	// Handle includes:
	mountPoints := make([]*fs.MountPoint, 0)
	for _, include := range gr.Spec.Include {
		repoName := fmt.Sprintf(
			"%s/%s/%s",
			"GitRepository",
			gr.Namespace,
			include.GitRepositoryRef.Name,
		)
		includedRepo, ok := l.repos[repoName]
		if !ok {
			return nil, fmt.Errorf("include %s not found", repoName)
		}
		mountPoints = append(mountPoints, &fs.MountPoint{
			Location: orDefault(include.ToPath, include.GitRepositoryRef.Name),
			Path:     include.FromPath,
			FS:       includedRepo,
		})
	}
	if len(mountPoints) > 0 {
		repoFS = fs.Mount(repoFS, mountPoints...)
	}

	// Enable caching:
	//{
	//	md5h := md5.Sum([]byte(remoteURL + gitRef))
	//	hash := hex.EncodeToString(md5h[:])
	//	repoFS = fs.Cache(repoFS, filepath.Join(l.repoCachePath, gr.Namespace+"-"+gr.Name+"-"+hash))
	//}

	l.repos[nn] = fs.KrustyFileSystem(repoFS)
	return gr, nil
}

// gitHttpsURL returns a URL that clones via https protocol.
func gitHttpsURL(repoURL string) (string, error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "https":
		return u.String(), nil
	case "ssh":
		u.Scheme = "https"
		u.User = nil
		if strings.HasSuffix(u.Path, ".git") {
			u.Path = strings.TrimSuffix(u.Path, ".git")
		}
		return u.String(), nil
	default:
		u.Scheme = "https"
		return gitHttpsURL(u.String())
	}
}

// gitSSHUrl returns a URL that will clone via SSH protocol.
func gitSSHUrl(repoURL string) (string, error) {
	// TODO: hack to parse relative git repos, e.g. ssh://git@github.com:a/b.
	//  otherwise errors "first path segment in URL cannot contain colon".
	//  Consider https://github.com/gitsight/go-vcsurl or some other parsing library.
	repoURL = strings.Replace(repoURL, "github.com:", "github.com/", 1)

	u, err := url.Parse(repoURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "ssh"
		return gitSSHUrl(u.String())
	case "ssh":
		if u.User == nil {
			u.User = url.User("git")
		}
		// TODO: is it better to always return without .git ?
		if !strings.HasSuffix(u.Path, ".git") {
			u.Path = u.Path + ".git"
		}
		return u.String(), nil
	default:
		u.Scheme = "ssh"
		return gitSSHUrl(u.String())
	}
}

// gitURLEquals returns true if two remotes have the same representation
// when converted to SSH urls.
func gitURLEquals(a, b string) (bool, error) {
	au, err := gitSSHUrl(a)
	if err != nil {
		return false, err
	}
	bu, err := gitSSHUrl(b)
	if err != nil {
		return false, err
	}
	return au == bu, nil
}

func gitRepoReference(gr *intres.GitRepository) string {
	ref := gr.Spec.Reference
	return orDefault(orDefault(ref.Commit, ref.Branch), ref.Tag)
}

func ociRepoReference(gr *intres.OCIRepository) string {
	ref := gr.Spec.Reference
	return orDefault(orDefault(ref.Digest, ref.SemVer), ref.Tag)
}
