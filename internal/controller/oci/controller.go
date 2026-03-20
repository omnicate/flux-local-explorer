// Copyright 2025 Cisco Systems, Inc. and its affiliates
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package oci

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	ociclient "github.com/fluxcd/pkg/oci/client"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/rs/zerolog"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	ctrl "github.com/omnicate/flux-local-explorer/internal/controller"
	"github.com/omnicate/flux-local-explorer/internal/fs"
)

func init() {
	_ = sourcev1b2.AddToScheme(ctrl.Scheme)
}

var _ ctrl.Controller = new(Controller)

type Controller struct {
	logger zerolog.Logger
	opts   Options
	mu     sync.Mutex
}

func NewController(logger zerolog.Logger, opts Options) *Controller {
	return &Controller{logger: logger, opts: opts}
}

func (l *Controller) Kinds() []string {
	return []string{"OCIRepository"}
}

func (l *Controller) Reconcile(ctx ctrl.Context, req *ctrl.Resource) (*ctrl.Result, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	var or sourcev1b2.OCIRepository
	if err := req.Unmarshal(&or); err != nil {
		return nil, err
	}

	ociRef := ociRepoReference(&or)
	if ociRef == "" {
		return nil, fmt.Errorf("OCIRepository reference is missing")
	}
	if !strings.HasPrefix(or.Spec.URL, sourcev1b2.OCIRepositoryPrefix) {
		return nil, fmt.Errorf("OCIRepository with invalid scheme")
	}

	url := strings.TrimPrefix(or.Spec.URL, sourcev1b2.OCIRepositoryPrefix)
	md5h := md5.Sum([]byte(url + "@" + ociRef))
	hash := hex.EncodeToString(md5h[:])
	ociRepoPath := filepath.Join(l.opts.CachePath, or.Namespace+"-"+or.Name+"-"+hash)
	if _, err := os.Stat(ociRepoPath); os.IsNotExist(err) {
		client := ociclient.NewClient([]crane.Option{})
		_, err := client.Pull(context.Background(), url+":"+ociRef, ociRepoPath)
		if err != nil {
			return nil, fmt.Errorf("oci pull: %w", err)
		}
	}

	return &ctrl.Result{
		Attachment: fs.KrustyFileSystem(fs.Prefix(filesys.MakeFsOnDisk(), ociRepoPath)),
	}, nil
}

func ociRepoReference(gr *sourcev1b2.OCIRepository) string {
	ref := gr.Spec.Reference
	return ctrl.Any(ref.Digest, ref.SemVer, ref.Tag)
}
