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
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

type DiffFlags struct {
	name          string
	namespace     string
	allNamespaces bool

	command string
	short   bool
}

var diffArgs DiffFlags

// getCmd represents the get command
var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Diff two flux clusters",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		bin, _, ok := strings.Cut(diffArgs.command, " ")
		if !ok {
			return fmt.Errorf("not a valid diff command: %s", diffArgs.command)
		}
		_, err := exec.LookPath(bin)
		return err
	},
}

func init() {
	diffCmd.PersistentFlags().BoolVarP(
		&diffArgs.allNamespaces,
		"all-namespaces",
		"A",
		false,
		"diff the requested object(s) across all namespaces",
	)
	diffCmd.PersistentFlags().StringVarP(
		&diffArgs.namespace,
		"namespace",
		"n",
		"flux-system",
		"diff the requested object(s) in this namespace",
	)
	diffCmd.PersistentFlags().BoolVarP(
		&diffArgs.short,
		"short",
		"",
		false,
		"only print summary",
	)
	diffCmd.PersistentFlags().StringVarP(
		&diffArgs.command,
		"diff-tool",
		"",
		"dyff --color on between -b -i ${base} ${against}",
		"only print summary",
	)
	rootCmd.AddCommand(diffCmd)
}
