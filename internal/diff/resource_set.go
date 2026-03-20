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

package diff

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	ctrl "github.com/omnicate/flux-local-explorer/internal/controller"
)

type Diff struct {
	cacheDir string
	command  string
	a, b     map[string]*ctrl.Resource
}

func New(cacheDir, cmd string) *Diff {
	return &Diff{
		cacheDir: cacheDir,
		command:  cmd,
		a:        map[string]*ctrl.Resource{},
		b:        map[string]*ctrl.Resource{},
	}
}

func (d Diff) AddBase(r *ctrl.Resource) {
	d.a[resourceKey(r)] = r
}

func (d Diff) AddAgainst(r *ctrl.Resource) {
	d.b[resourceKey(r)] = r
}

func (d Diff) PrettyDiffAll() (string, error) {
	var out bytes.Buffer

	added := make([]*ctrl.Resource, 0)
	removed := make([]*ctrl.Resource, 0)
	changed := map[*ctrl.Resource]string{}

	for _, res := range d.a {
		otherRes, ok := d.b[resourceKey(res)]
		if !ok {
			removed = append(removed, res)
			continue
		}

		diff, err := d.diffResources(res, otherRes)
		if err != nil {
			return "", err
		}
		if diff == "" {
			continue
		}
		changed[res] = diff
	}

	for _, otherRes := range d.b {
		_, ok := d.a[resourceKey(otherRes)]
		if !ok {
			added = append(added, otherRes)
			continue
		}
	}

	for _, res := range added {
		fmt.Fprintln(&out, "# added", resourceKey(res))
		fmt.Fprintln(&out, res.MustYaml())
		fmt.Fprintln(&out)
	}

	for _, res := range removed {
		fmt.Fprintln(&out, "# removed", resourceKey(res))
		fmt.Fprintln(&out, res.MustYaml())
		fmt.Fprintln(&out)
	}

	for res, diff := range changed {
		fmt.Fprintln(&out, "# changed ", resourceKey(res))
		fmt.Fprintln(&out, diff)
	}

	return out.String(), nil
}

func (d Diff) diffResources(base, now *ctrl.Resource) (string, error) {
	tempDir := filepath.Join(
		d.cacheDir,
		fmt.Sprintf("%d", time.Now().UnixNano()),
	)
	defer os.RemoveAll(tempDir)

	_ = os.MkdirAll(tempDir, 0755)

	baseFile := filepath.Join(tempDir, "base.yaml")
	againstFile := filepath.Join(tempDir, "against.yaml")

	a, err := os.Create(baseFile)
	if err != nil {
		return "", err
	}
	_, _ = a.WriteString(base.MustYaml())
	_ = a.Close()

	b, err := os.Create(againstFile)
	if err != nil {
		return "", err
	}
	_, _ = b.WriteString(now.MustYaml())
	_ = b.Close()

	extDiff := d.command
	extDiff = strings.Replace(
		extDiff, ""+
			"${base}",
		baseFile,
		-1,
	)
	extDiff = strings.Replace(
		extDiff, ""+
			"${against}",
		againstFile,
		-1,
	)

	segments := strings.Fields(extDiff)
	if len(segments) < 2 {
		return "", fmt.Errorf("bad diff command")
	}
	cmd := exec.Command(segments[0], segments[1:]...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

func resourceKey(r *ctrl.Resource) string {
	return strings.Join([]string{
		r.GetApiVersion(),
		r.GetKind(),
		r.GetNamespace(),
		r.GetName(),
	}, "/")
}
