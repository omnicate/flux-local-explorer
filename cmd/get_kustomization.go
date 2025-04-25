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

package cmd

import (
	"fmt"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"github.com/spf13/cobra"

	"github.com/omnicate/flx/internal/loader"
)

// getKustomizationCmd represents the getKustomization command
var getKustomizationCmd = &cobra.Command{
	Use:     "kustomization",
	Aliases: []string{"ks"},
	Short:   "Flux Kustomization resources (ks)",
	PreRunE: getCmd.PreRunE,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			getArgs.name = args[0]
		}
		mgr, err := newManager(true)
		if err != nil {
			return err
		}
		if err := mgr.Run(); err != nil {
			return err
		}
		results := mgr.ListWithKind(
			"Kustomization",
			getArgs.namespace,
			getArgs.allNamespaces,
		)
		results = filterResults(results, getArgs.name, getArgs.namespace, getArgs.allNamespaces)

		if getArgs.format == "kustomize" {
			for _, re := range results {
				for _, r := range re.GetResources() {
					fmt.Println("---")
					fmt.Println(r.Resource.MustYaml())
				}
			}
			return nil
		}

		return printResults(results, kustomizationHeaders, kustomizationRows)
	},
}

func init() {
	getCmd.AddCommand(getKustomizationCmd)
}

func kustomizationHeaders() []string {
	var headers []string
	if getArgs.allNamespaces {
		headers = append(headers, "Namespace")
	}
	return append(headers, []string{
		"Name",
		"Source",
		"Resources",
		"Error",
	}...)
}

func kustomizationRows(rn *loader.ResourceNode) []string {
	var row []string
	if getArgs.allNamespaces {
		row = append(row, rn.Resource.GetNamespace())
	}
	var ks kustomizev1.Kustomization
	rn.Resource.Unmarshal(&ks)

	return append(row, []string{
		ks.Name,
		formatSource(&ks),
		fmt.Sprintf("%d", len(rn.GetResources())),
		errOrEmpty(rn.Error),
	}...)
}

func formatSource(ks *kustomizev1.Kustomization) string {
	ns := ks.Namespace
	if sourceNs := ks.Spec.SourceRef.Namespace; sourceNs != "" {
		ns = sourceNs
	}
	switch ks.Spec.SourceRef.Kind {
	case "GitRepository":
		return "git: " + ns + "/" + ks.Spec.SourceRef.Name
	case "OCIRepository":
		return "oci: " + ns + "/" + ks.Spec.SourceRef.Name
	}
	panic("unsupported kustomization source " + ks.Spec.SourceRef.Kind)
}
