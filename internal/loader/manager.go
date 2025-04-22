package loader

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/omnicate/flx/internal/controller"
)

const maxIterations = 4096

// Manager runs controllers, builds and queries the resource tree.
type Manager struct {
	logger      zerolog.Logger
	controllers map[string][]controller.Controller
	root        *ResourceNode
}

// NewManager from a slice of controllers.
func NewManager(
	logger zerolog.Logger,
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
		logger:      logger,
		controllers: controllerMap,
	}
}

// Initialize the resource tree by loading some resources.
func (m *Manager) Initialize(
	fs filesys.FileSystem,
	path string,
	defaultNamespace string,
) error {
	resources, err := LoadPath(fs, path)
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

// Run the manager until completion. Reconciliation is finished if no more resources are created or all controllers
// report errors that could not be resolved.
func (m *Manager) Run() error {
	total := time.Duration(0)
	for i := range maxIterations {
		start := time.Now()
		n, err := m.runOnce()
		if err != nil {
			return err
		}
		total += time.Since(start)

		m.logger.Debug().
			Int("iteration", i).
			Int("resources", n).
			Str("took", time.Since(start).String()).
			Str("total", total.String()).
			Msg("reconciled resources")

		if n == 0 {
			return nil
		}
	}
	return fmt.Errorf("max iterations reached")
}

func (m *Manager) processNode(node *ResourceNode) bool {
	kind := node.Resource.GetKind()
	if node.Status == StatusCompleted || node.Status == StatusError {
		return false
	}
	start := time.Now()
	defer func() {
		node.Duration = time.Since(start)
	}()
	controllers := m.controllers[kind]
	if len(controllers) == 0 {
		node.Status = StatusCompleted
		return true
	}

	for _, ctrl := range controllers {

		// Reconcile it:
		result, err := ctrl.Reconcile(&Context{tree: m.root}, node.Resource)

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

		// Never add a resource twice, even if a controller
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

	// This resource was processed by at least one controller.
	return true
}

// runOnce reconciles every resource once by calling controllers and adding to the resource tree.
func (m *Manager) runOnce() (int, error) {
	nodes := m.root.FlatByStatus(StatusUnknown)
	nodeChan := make(chan *ResourceNode)
	go func() {
		for _, node := range nodes {
			nodeChan <- node
		}
		close(nodeChan)
	}()

	// TODO: nothing here is thread safe.
	//  This will crash eventually.
	var threads = runtime.NumCPU()
	var wg sync.WaitGroup
	var n uint32
	for range threads {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for node := range nodeChan {
				wasProcessed := m.processNode(node)
				if wasProcessed {
					atomic.AddUint32(&n, 1)
				}
			}
		}()
	}
	wg.Wait()

	return int(n), nil
}

// AllNodes returns a flat representation of all resource nodes encountered during Run.
func (m *Manager) AllNodes() []*ResourceNode {
	return m.root.Flat()
}

// ListWithKind retrieves resources that are of a specific kind.
func (m *Manager) ListWithKind(kind, namespace string, allNamespaces bool) []*ResourceNode {
	result := m.root.Flat().FilterByKind(kind)
	if allNamespaces {
		return result
	}
	return result.FilterByNamespace(namespace)
}

var _ controller.Context = new(Context)

type Context struct {
	tree *ResourceNode
}

func (m *Context) GetAttachment(kind, namespace, name string) (any, bool) {
	found, ok := m.tree.Find(kind, namespace, name)
	if !ok {
		return nil, false
	}
	return found.Attachment, found.Attachment != nil
}

func (m *Context) GetResource(kind, namespace, name string) (*controller.Resource, bool) {
	found, ok := m.tree.Find(kind, namespace, name)
	if !ok {
		return nil, false
	}
	return found.Resource, found.Resource != nil
}

func (m *Context) ClientSet() client.Client {
	return NewClientSet(controller.Scheme, m.tree)
}
