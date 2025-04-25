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
	"strings"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/spf13/cobra"

	"github.com/omnicate/flx/internal/loader"
)

var getGitRepoCmd = &cobra.Command{
	Use:     "git-repo",
	Aliases: []string{"gr", "git"},
	Short:   "Flux GitRepository resources (gr)",
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
			"GitRepository",
			getArgs.namespace,
			getArgs.allNamespaces,
		)
		results = filterResults(results, getArgs.name, getArgs.namespace, getArgs.allNamespaces)
		return printResults(results, gitRepoHeaders, gitRepoRows)
	},
}

func init() {
	getCmd.AddCommand(getGitRepoCmd)
}

func gitRepoHeaders() []string {
	var headers []string
	if getArgs.allNamespaces {
		headers = append(headers, "Namespace")
	}
	return append(headers, []string{
		"Name",
		"URL",
		"Reference",
		"Includes",
		"Error",
	}...)
}

func gitRepoRows(rn *loader.ResourceNode) []string {
	var gr sourcev1.GitRepository
	rn.Resource.Unmarshal(&gr)
	var row []string
	if getArgs.allNamespaces {
		row = append(row, gr.Namespace)
	}
	return append(row, []string{
		gr.Name,
		gr.Spec.URL,
		formatGitRepoReference(gr.Spec.Reference),
		formatIncludes(gr.Spec.Include),
		errOrEmpty(rn.Error),
	}...)
}

func formatGitRepoReference(ref *sourcev1.GitRepositoryRef) string {
	if ref == nil {
		return ""
	}
	if ref.Commit != "" {
		return "Commit: " + ref.Commit
	}
	if ref.Branch != "" {
		return "Branch: " + ref.Branch
	}
	if ref.Tag != "" {
		return "Tag: " + ref.Tag
	}
	return "Unknown"
}

func formatIncludes(incls []sourcev1.GitRepositoryInclude) string {
	var repos []string
	for _, incl := range incls {
		repos = append(repos, incl.GitRepositoryRef.Name)
	}
	return strings.Join(repos, ", ")
}
