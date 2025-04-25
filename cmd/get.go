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
	"sort"

	"github.com/fluxcd/flux2/v2/pkg/printers"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/omnicate/flx/internal/loader"
)

type GetFlags struct {
	namespace     string
	allNamespaces bool
	format        string
	name          string
}

var (
	getArgs GetFlags
)

// getCmd represents the get command
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Retrieve resources",
}

func init() {
	getCmd.PersistentFlags().BoolVarP(
		&getArgs.allNamespaces,
		"all-namespaces",
		"A",
		false,
		"list the requested object(s) across all namespaces",
	)
	getCmd.PersistentFlags().StringVarP(
		&getArgs.namespace,
		"namespace",
		"n",
		"flux-system",
		"list the requested object(s) in this namespace",
	)
	getCmd.PersistentFlags().StringVarP(
		&getArgs.format,
		"output-format",
		"o",
		"pretty",
		"format the list using one of [table, yaml]",
	)
	rootCmd.AddCommand(getCmd)
}

func sortResources(list []*loader.ResourceNode) {
	sort.Slice(list, func(i, j int) bool {
		{
			a, b := list[i].Resource.GetNamespace(), list[j].Resource.GetNamespace()
			if a != b {
				return a < b
			}
		}
		{
			a, b := list[i].Resource.GetName(), list[j].Resource.GetName()
			if a != b {
				return a < b
			}
		}
		return false
	})
}

func filterResults(
	resources []*loader.ResourceNode,
	filterName string,
	filterNamespace string,
	filterAllNamespaces bool,
) []*loader.ResourceNode {
	var results []*loader.ResourceNode

	for _, res := range resources {
		if filterAllNamespaces {
			results = append(results, res)
			continue
		}
		if ns := filterNamespace; ns != "" && res.Resource.GetNamespace() != ns {
			continue
		}
		if name := filterName; name != "" && res.Resource.GetName() != name {
			continue
		}
		results = append(results, res)
	}
	sortResources(results)
	return results
}

func printResults(
	results []*loader.ResourceNode,
	headerFunc func() []string,
	rowFunc func(node *loader.ResourceNode) []string,
) error {
	if len(results) == 0 {
		return fmt.Errorf("no results")
	}
	switch getArgs.format {
	case "pretty":
		var rows [][]string
		for _, item := range results {
			rows = append(rows, rowFunc(item))
		}
		return printers.TablePrinter(headerFunc()).Print(os.Stdout, rows)
	case "yaml":
		for _, r := range results {
			fmt.Println("---")
			data, _ := yaml.Marshal(r.Resource)
			fmt.Println(string(data))
		}
		return nil
	}
	return fmt.Errorf("unknown format: %s", getArgs.format)
}

func errOrEmpty(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
