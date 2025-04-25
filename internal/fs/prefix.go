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
	"path/filepath"
)

var _ FileSystem = new(prefixFileSystem)

type prefixFileSystem struct {
	readOnlyFileSystem

	base   FileSystem
	prefix string
}

func Prefix(base FileSystem, prefix string) FileSystem {
	return &prefixFileSystem{
		base:   base,
		prefix: prefix,
	}
}

func (p prefixFileSystem) IsDir(path string) bool {
	path = filepath.Join(p.prefix, path)
	return p.base.IsDir(path)
}

func (p prefixFileSystem) ReadDir(path string) ([]string, error) {
	path = filepath.Join(p.prefix, path)
	return p.base.ReadDir(path)
}

func (p prefixFileSystem) Exists(path string) bool {
	path = filepath.Join(p.prefix, path)
	return p.base.Exists(path)
}

func (p prefixFileSystem) ReadFile(path string) ([]byte, error) {
	path = filepath.Join(p.prefix, path)
	return p.base.ReadFile(path)
}
