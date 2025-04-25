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

package fs

import (
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/kustomize/kyaml/filesys"
)

var _ FileSystem = new(cacheFileSystem)

type cacheFileSystem struct {
	FileSystem

	path  string
	cache FileSystem
}

func Cache(base FileSystem, path string) FileSystem {
	return &cacheFileSystem{
		FileSystem: base,
		path:       path,
		cache:      filesys.MakeFsOnDisk(),
	}
}

func (c cacheFileSystem) cachePath(path string) string {
	return filepath.Join(c.path, path)
}

func (c cacheFileSystem) ReadFile(path string) ([]byte, error) {
	if !c.FileSystem.Exists(path) {
		return nil, os.ErrNotExist
	}
	cp := c.cachePath(path)
	data, err := c.cache.ReadFile(cp)
	if err == nil {
		return data, nil
	}
	data, err = c.FileSystem.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := c.cache.MkdirAll(filepath.Dir(cp)); err != nil {
		return nil, fmt.Errorf("failed to make cache dir: %w", err)
	}
	if err := c.cache.WriteFile(cp, data); err != nil {
		return nil, fmt.Errorf("failed to write cache file: %w", err)
	}
	return data, err
}
