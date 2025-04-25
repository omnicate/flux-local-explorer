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

package controller

import (
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Scheme to use for building k8s ClientSets.
var Scheme = runtime.NewScheme()

func init() {
	_ = clientgoscheme.AddToScheme(Scheme)
}

type Context interface {
	// ClientSet for a subset of resource kinds. Must implement Get and List methods.
	ClientSet() client.Client
	// GetAttachment for a specific resource that any controller added during the Reconcile
	// invocation.
	GetAttachment(kind, namespace, name string) (any, bool)
	// GetResource with specific kind, namespace and name. Use this over ClientSet().Get(...) if the
	// ApiVersion of a resource is unknown or ambiguous. Returns the first resource matched in a resource tree.
	GetResource(kind, namespace, name string) (*Resource, bool)
}

// Result of a single reconciliation.
type Result struct {
	// Resources that are created based on the handled resource, e.g. ExternalSecret creates a Secret.
	Resources []*Resource
	// Attachment that should be stored alongside this resource node, or nil. Attachments can be retrieved
	// using Context.GetAttachment.
	Attachment any
}

// Controller defines a reconciliation loop.
type Controller interface {
	// Kinds this controller will accept. Only resources of this API kind will be handled, e.g. "Secret",
	// "Kustomization". Scope a controller as narrow as possible.
	Kinds() []string
	// Reconcile is invoked for each resource. Returns a list of resources to create
	// and an error. If error is ErrSkip, the controller is skipped for this invocation.
	Reconcile(Context, *Resource) (*Result, error)
}

// Any returns the first non-zero value, or zero.
func Any[T comparable](items ...T) T {
	var zero T
	for _, item := range items {
		if item != zero {
			return item
		}
	}
	return zero
}
