package loader

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/fluxcd/pkg/chartutil"
	"github.com/go-logr/zerologr"
	sigyaml "sigs.k8s.io/yaml"

	intres "github.com/omnicate/flx/resource"
)

func (l *Loader) handleHelmRepository(hr *intres.HelmRepository) (
	*intres.HelmRepository,
	error,
) {
	l.helmRepos = append(l.helmRepos, hr)
	return hr, nil
}

func (l *Loader) renderHelmRelease(
	hr *intres.HelmRelease,
) (
	*ResultSet,
	error,
) {
	if hr.SourceRef.Kind != intres.HelmRepositoryKind {
		return nil, fmt.Errorf("helm release source only supports HelmRepository")
	}
	if hr.Chart == "" {
		return nil, fmt.Errorf("HelmRelease \"chart\" is required")
	}

	var found *intres.HelmRepository
	for _, repo := range l.helmRepos {
		if repo.Name == hr.SourceRef.Name &&
			repo.Namespace == orDefault(hr.SourceRef.Namespace, hr.Namespace) {
			found = repo
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("HelmRepository %s not found", hr.SourceRef.Name)
	}

	if found.URL == "" {
		return nil, fmt.Errorf("HelmRepository needs a URL")
	}

	md5h := md5.Sum([]byte(found.URL + "@" + hr.Version))
	hash := hex.EncodeToString(md5h[:])
	cache := filepath.Join(l.repoCachePath, hr.Chart+"-"+hr.Version+"-"+hash)

	// Run `helm pull` command.
	if _, err := os.Stat(cache); os.IsNotExist(err) {
		var cmd *exec.Cmd
		if found.Type == "oci" {
			cmd = exec.Command(
				"helm",
				"pull",
				"--version", hr.Version,
				"--untar",
				"--untardir", cache,
				found.URL+"/"+hr.Chart,
			)
		} else {
			cmd = exec.Command(
				"helm",
				"pull",
				"--repo", found.URL,
				"--version", hr.Version,
				"--untar",
				"--untardir", cache,
				hr.Chart,
			)
		}
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stderr

		l.logger.Debug().
			Str("path", cache).
			Str("name", hr.Name).
			Str("cmd", cmd.String()).
			Str("namespace", hr.Namespace).
			Msg("downloading helm chart")

		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("helm pull download failed: %w", err)
		}
	}

	values, err := chartutil.ChartValuesFromReferences(
		context.Background(),
		zerologr.New(&l.logger),
		l.cs,
		hr.Namespace,
		hr.Values,
		hr.ValuesFrom...,
	)
	if err != nil {
		return nil, fmt.Errorf("values from references: %w", err)
	}

	// Write values to some file.
	data, err := sigyaml.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("marshalling values.yaml: %w", err)
	}
	valuesMd5h := md5.Sum(data)
	valuesHash := hex.EncodeToString(valuesMd5h[:])
	valuesFile := filepath.Join(cache, "values-"+valuesHash+".yaml")
	if _, err := os.Stat(valuesFile); os.IsNotExist(err) {
		if err := os.WriteFile(valuesFile, data, 0600); err != nil {
			return nil, fmt.Errorf("writing values.yaml: %w", err)
		}
	}

	// Run `helm template` command.
	var out bytes.Buffer
	cmd := exec.Command(
		"helm",
		"template",
		filepath.Join(cache, filepath.Base(hr.Chart)),
		"-f", valuesFile,
	)
	l.logger.Debug().
		Str("path", cache).
		Str("name", hr.Name).
		Str("cmd", cmd.String()).
		Str("namespace", hr.Namespace).
		Msg("templating helm chart")

	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	cmd.Env = []string{}
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("helm template: %w", err)
	}

	resources, err := l.loadBytes(out.Bytes())
	if err != nil {
		return nil, fmt.Errorf("loading resources from helm: %w", err)
	}

	return l.buildResultSet(resources, hr.Namespace)
}
