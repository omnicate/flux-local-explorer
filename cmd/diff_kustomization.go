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
	"os"

	"github.com/spf13/cobra"

	"github.com/omnicate/flx/internal/diff"
)

// diffKustomizationCmd compares two kustomizations
var diffKustomizationCmd = &cobra.Command{
	Use:     "kustomization",
	Aliases: []string{"ks"},
	Short:   "Flux Kustomization resources (ks)",
	PreRunE: getCmd.PreRunE,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			diffArgs.name = args[0]
		}

		d := diff.New(os.TempDir(), diffArgs.command)

		{
			logger.Debug().Msg("Getting base version")
			mgr, err := newManager(false, rootArgs.enabledControllers)
			if err != nil {
				return err
			}
			if err := mgr.Run(); err != nil {
				return err
			}
			results := mgr.ListWithKind(
				"Kustomization",
				diffArgs.namespace,
				diffArgs.allNamespaces,
			)
			results = filterResults(results, diffArgs.name, diffArgs.namespace, diffArgs.allNamespaces)
			for _, result := range results {
				for _, res := range result.GetResources() {
					d.AddBase(res.Resource)
				}
			}
		}

		{
			logger.Debug().Msg("Getting current version")
			mgr, err := newManager(true, rootArgs.enabledControllers)
			if err != nil {
				return err
			}
			if err := mgr.Run(); err != nil {
				return err
			}
			results := mgr.ListWithKind(
				"Kustomization",
				diffArgs.namespace,
				diffArgs.allNamespaces,
			)
			results = filterResults(results, diffArgs.name, diffArgs.namespace, diffArgs.allNamespaces)
			for _, result := range results {
				for _, res := range result.GetResources() {
					d.AddAgainst(res.Resource)
				}
			}
		}

		diffOutput, err := d.PrettyDiffAll()
		if err != nil {
			return err
		}

		fmt.Println(diffOutput)
		return nil
	},
}

func init() {
	diffCmd.AddCommand(diffKustomizationCmd)
}
