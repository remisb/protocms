package store

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

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
// Format is detected on disk: a v2 folder (data/<name>/) is preferred.
// If neither exists, a new
// empty v2 folder-backed dataset is created in memory (persisted on first
// write).
func (r *Registry) Load(name string) *Dataset {
	d := openDataset(name)
	d.load()
	r.mu.Lock()
	r.datasets[name] = d
	r.mu.Unlock()
	return d
}

// openDataset resolves the on-disk layout for name and returns an unloaded
// Dataset bound to it.
func openDataset(name string) *Dataset {
	dir := filepath.Join(dataDir, name)

	if isDir(dir) {
		meta, err := loadMetadata(dir)
		if err != nil {
			slog.Warn("dataset folder missing/invalid meta.json; using defaults",
				"dataset", name, "err", err)
			meta = newMetadata(name, nowUTC())
		}
		return newFolderDataset(name, dir, meta)
	}

	// Neither exists: brand-new dataset, default to the current folder format.
	return newFolderDataset(name, dir, newMetadata(name, nowUTC()))
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
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

// List returns an info aggregate (meta + stats + memory + metrics) for every
// loaded dataset.
func (r *Registry) List() []DatasetInfo {
	r.mu.RLock()
	datasets := make([]*Dataset, 0, len(r.datasets))
	for _, d := range r.datasets {
		datasets = append(datasets, d)
	}
	r.mu.RUnlock()

	out := make([]DatasetInfo, 0, len(datasets))
	for _, d := range datasets {
		out = append(out, d.Info())
	}
	return out
}

// System returns a typed accessor over the registry's _system dataset,
// loading it if it is not already in memory.
func (r *Registry) System() *SystemStore {
	d, ok := r.Get(SystemDatasetName)
	if !ok {
		d = r.Load(SystemDatasetName)
	}
	return &SystemStore{d: d}
}
