package store

import "sync"

// Registry holds the datasets currently loaded in memory, keyed by name.
// Each dataset carries its own lock; the registry lock only guards the map
// of datasets (membership), not the datasets' contents.
type Registry struct {
	mu       sync.RWMutex
	datasets map[string]*Dataset
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{datasets: make(map[string]*Dataset)}
}

// Load reads the named dataset from disk and inserts it into the registry,
// replacing any existing entry. It returns the loaded dataset.
//
// Phase 1 keeps the original flat-file layout (data/<name>.json); later
// phases will resolve a per-dataset folder here.
func (r *Registry) Load(name string) *Dataset {
	d := newDataset(name, dataDir+"/"+name+".json")
	d.load()
	r.mu.Lock()
	r.datasets[name] = d
	r.mu.Unlock()
	return d
}

// Unload drops the named dataset from memory. The on-disk files are left
// untouched. It reports whether a dataset was present.
func (r *Registry) Unload(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.datasets[name]
	delete(r.datasets, name)
	return ok
}

// Get returns the named dataset if it is loaded.
func (r *Registry) Get(name string) (*Dataset, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.datasets[name]
	return d, ok
}

// Names returns the names of all loaded datasets.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.datasets))
	for name := range r.datasets {
		names = append(names, name)
	}
	return names
}
