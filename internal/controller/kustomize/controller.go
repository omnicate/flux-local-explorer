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

package kustomize

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"github.com/fluxcd/pkg/kustomize"
	"github.com/rs/zerolog"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	ctrl "github.com/omnicate/flux-local-explorer/internal/controller"
	"github.com/omnicate/flux-local-explorer/internal/loader"
)

func init() {
	_ = kustomizev1.AddToScheme(ctrl.Scheme)
}

var _ ctrl.Controller = new(Controller)

type Controller struct {
	logger zerolog.Logger
}

func NewController(logger zerolog.Logger) *Controller {
	return &Controller{logger: logger}
}

func (r Controller) Kinds() []string {
	return []string{"Kustomization"}
}

func (r Controller) Reconcile(ctx ctrl.Context, req *ctrl.Resource) (*ctrl.Result, error) {
	var ks kustomizev1.Kustomization
	if err := req.Unmarshal(&ks); err != nil {
		return nil, err
	}

	logger := r.logger.With().
		Str("namespace", ks.Namespace).
		Str("name", ks.Name).
		Logger()

	startTime := time.Now()

	// Get source repository:
	sourceAttachment, ok := ctx.GetAttachment(
		ks.Spec.SourceRef.Kind,
		ctrl.Any(ks.Spec.SourceRef.Namespace, ks.Namespace),
		ks.Spec.SourceRef.Name,
	)
	if !ok {
		return nil, fmt.Errorf(
			"failed to find source repository %s/%s",
			ctrl.Any(ks.Spec.SourceRef.Namespace, ks.Namespace),
			ks.Spec.SourceRef.Name,
		)
	}
	repoFS, ok := sourceAttachment.(filesys.FileSystem)
	if !ok {
		return nil, fmt.Errorf(
			"source repo attachment of invalid type: %T",
			sourceAttachment,
		)
	}

	// Load resources:
	resources, err := loader.LoadPath(repoFS, normalizeRepoPath(ks.Spec.Path))
	if err != nil {
		return nil, fmt.Errorf("failed to load path: %v", err)
	}

	targetNamespace := ctrl.Any(ks.Spec.TargetNamespace, ks.Namespace)

	// Variable substitution:
	if pb := ks.Spec.PostBuild; pb != nil && (pb.SubstituteFrom != nil || pb.Substitute != nil) {
		obj, err := req.Unstructured()
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal Kustomization: %v", err)
		}
		clientSet := ctx.ClientSet()
		for i, res := range resources {
			newRes, err := kustomize.SubstituteVariables(
				context.Background(),
				clientSet,
				*obj,
				res,
				kustomize.SubstituteWithStrict(true),
			)
			if err != nil {
				return nil, fmt.Errorf(
					"%s/%s/%s: %w",
					res.GetKind(),
					ctrl.Any(res.GetNamespace(), targetNamespace),
					res.GetName(),
					err,
				)
			}
			resources[i] = newRes
		}
	}

	// Parse the resulting resources:
	for _, res := range resources {
		if res.GetNamespace() == "" {
			_ = res.SetNamespace(targetNamespace)
		}
	}

	logger.Debug().
		Str("elapsed", time.Since(startTime).String()).
		Msg("loaded")

	// TODO: add load functions to directly use Resource.
	//  Only the KS controller should need to load resources. Ever.
	return &ctrl.Result{Resources: ctrl.NewResources(resources)}, nil
}

func normalizeRepoPath(path string) string {
	cleaned := filepath.Clean(path)
	if filepath.IsAbs(cleaned) {
		cleaned = strings.TrimLeft(cleaned, string(filepath.Separator))
		if cleaned == "" {
			return "."
		}
	}
	return cleaned
}
