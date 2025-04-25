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
	"sort"

	"github.com/spf13/cobra"

	"github.com/omnicate/flx/internal/loader"
)

var statCmd = &cobra.Command{
	Use:     "stat",
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
		results := mgr.AllNodes()

		sort.Slice(results, func(i, j int) bool {
			return results[i].Duration < results[j].Duration
		})

		return printResults(results, statHeaders, statRows)
	},
}

func init() {
	rootCmd.AddCommand(statCmd)
}

func statHeaders() []string {
	var headers []string
	if getArgs.allNamespaces {
		headers = append(headers, "Namespace")
	}
	return append(headers, []string{
		"Name",
		"Kind",
		"Duration",
	}...)
}

func statRows(rn *loader.ResourceNode) []string {
	var row []string
	if getArgs.allNamespaces {
		row = append(row, rn.Resource.GetNamespace())
	}

	return append(row, []string{
		rn.Resource.GetName(),
		rn.Resource.GetKind(),
		rn.Duration.String(),
	}...)
}
