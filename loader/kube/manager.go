package kube

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/omnicate/flx/loader"
	"github.com/omnicate/flx/loader/controller"
)

const maxIterations = 4096

type Manager struct {
	controllers map[string][]controller.Controller
	root        *ResourceNode
	clientSet   client.Client
}

func NewManager(
	controllers []controller.Controller,
) *Manager {
	controllerMap := make(map[string][]controller.Controller)
	for _, ctrl := range controllers {
		kinds := ctrl.Kinds()
		for _, kind := range kinds {
			controllerMap[kind] = append(controllerMap[kind], ctrl)
		}
	}
	return &Manager{
		controllers: controllerMap,
		clientSet: fake.
			NewClientBuilder().
			WithScheme(controller.Scheme).
			Build(),
	}
}

func (m *Manager) Initialize(
	fs filesys.FileSystem,
	path string,
	defaultNamespace string,
) error {
	resources, err := loader.LoadPath(fs, path)
	if err != nil {
		return fmt.Errorf("loading %s: %w", path, err)
	}

	m.root = &ResourceNode{}
	for _, res := range resources {
		if res.GetNamespace() == "" {
			_ = res.SetNamespace(defaultNamespace)
		}
	}
	m.root.AddResources(controller.NewResources(resources))
	return nil
}

func (m *Manager) Run() error {
	for range maxIterations {
		n, err := m.runOnce()
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
	}
	return fmt.Errorf("max iterations reached")
}

func (m *Manager) runOnce() (int, error) {
	var n int

	nodeChan := make(chan *ResourceNode, 16)
	go func() {
		_ = m.root.Walk(func(node *ResourceNode) error {
			nodeChan <- node
			return nil
		})
		close(nodeChan)
	}()

	// TODO: nothing here is thread safe.
	//  This will crash eventually.
	var threads = runtime.NumCPU() / 2
	var wg sync.WaitGroup
	for range threads {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for node := range nodeChan {
				if node.Resource == nil {
					continue
				}
				if node.Status == StatusCompleted {
					continue
				}
				if node.Status == StatusError {
					continue
				}
				var wasHandled bool
				for _, ctrl := range m.controllers[node.Resource.GetKind()] {

					// A resource was handled. We have to keep reconciling.
					n += 1
					wasHandled = true

					// Reconcile it:
					result, err := ctrl.Reconcile(m, node.Resource)

					// Error during reconciliation, try again.
					if err != nil {
						node.Error = err
						node.Attempts += 1
						if node.Attempts > 5 {
							node.Status = StatusError
						}
						continue
					}

					// Reset node:
					node.Error = nil
					node.Status = StatusCompleted
					node.Attachment = result.Attachment

					// Never add a resource twice, even if a controler
					// instructs us to.
					for _, res := range result.Resources {
						if _, ok := m.root.Find(
							res.GetKind(),
							res.GetNamespace(),
							res.GetName(),
						); ok {
							continue
						}
						node.AddResources([]*controller.Resource{res})
					}
				}

				// A resource which was not handled by any of our controllers
				// is treated as "Completed". This is a leaf node.
				if !wasHandled {
					node.Status = StatusCompleted
				}

				// Completed nodes are added to the client set
				if node.Status == StatusCompleted {
					kind := node.Resource.GetKind()

					// We're only tracking configMaps and secrets. All
					// other information is already part of the resource tree.
					if kind == "ConfigMap" || kind == "Secret" {
						obj, err := node.Resource.Unstructured()
						if err == nil {
							_ = m.clientSet.Create(context.Background(), obj)
						}
					}
				}
			}
		}()
	}
	wg.Wait()

	// TODO: Uncomment this if above keeps crashing:
	//_ = m.root.Walk(func(node *ResourceNode) error {
	//	if node.Resource == nil {
	//		return nil
	//	}
	//	if node.Status == StatusCompleted {
	//		return nil
	//	}
	//	if node.Status == StatusError {
	//		return nil
	//	}
	//
	//	var wasHandled bool
	//	for _, ctrl := range m.controllers[node.Resource.GetKind()] {
	//
	//		result, err := ctrl.Reconcile(m, node.Resource)
	//		if errors.Is(err, controller.ErrSkip) {
	//			continue
	//		}
	//
	//		// A resource was handled. We have to keep reconciling.
	//		n += 1
	//		wasHandled = true
	//
	//		// Error during reconciliation, try again.
	//		if err != nil {
	//			node.Error = err
	//			node.Attempts += 1
	//			if node.Attempts > 5 {
	//				node.Status = StatusError
	//			}
	//			continue
	//		}
	//
	//		// Reset node:
	//		node.Error = nil
	//		node.Status = StatusCompleted
	//		node.Attachment = result.Attachment
	//
	//		// Never add a resource twice, even if a controler
	//		// instructs us to.
	//		for _, res := range result.Resources {
	//			if _, ok := m.root.Find(
	//				res.GetKind(),
	//				res.GetNamespace(),
	//				res.GetName(),
	//			); ok {
	//				continue
	//			}
	//			node.AddResources([]*controller.Resource{res})
	//		}
	//	}
	//
	//	// A resource which was not handled by any of our controllers
	//	// is treated as "Completed". This is a leaf node.
	//	if !wasHandled {
	//		node.Status = StatusCompleted
	//	}
	//
	//	// All completed nodes are added to the client set
	//	if node.Status == StatusCompleted {
	//		kind := node.Resource.GetKind()
	//
	//		// We're only tracking configMaps and secrets. All
	//		// other information is already part of the resource tree.
	//		if kind == "ConfigMap" || kind == "Secret" {
	//			obj, err := node.Resource.Unstructured()
	//			if err == nil {
	//				_ = m.clientSet.Create(context.Background(), obj)
	//			}
	//		}
	//	}
	//	return nil
	//})

	return n, nil
}

func (m *Manager) GetAttachment(kind, namespace, name string) (any, bool) {
	found, ok := m.root.Find(kind, namespace, name)
	if !ok {
		return nil, false
	}
	return found.Attachment, found.Attachment != nil
}

func (m *Manager) GetResource(kind, namespace, name string) (*controller.Resource, bool) {
	found, ok := m.root.Find(kind, namespace, name)
	if !ok {
		return nil, false
	}
	return found.Resource, found.Resource != nil
}

func (m *Manager) ClientSet(kinds ...string) client.Client {
	return m.clientSet
}

func (m *Manager) ListWithKind(kind, namespace string, allNamespaces bool) []*ResourceNode {
	return m.root.ListNodes(kind, namespace, allNamespaces)
}
