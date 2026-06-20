// Package store holds ProtoCMS dataset state and persistence.
//
// A Dataset is a single, independently-loadable content store (schemas +
// content) guarded by its own lock. A Registry holds many loaded datasets.
//
// For backward compatibility, this file also exposes package-level functions
// (Init, Load, GetStats, CreateContent, ...) that operate on a single default
// dataset. Existing handlers use these; later phases resolve the dataset
// per-request from the registry instead.
package store

import (
	"log/slog"
	"os"
)

const dataDir = "data"

// ensureDataDir creates the data directory if it does not exist. A failure
// here is fatal: without it, no dataset can be loaded or persisted.
func ensureDataDir() {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		slog.Error("could not create data directory", "dir", dataDir, "err", err)
		os.Exit(1)
	}
}

// defaultRegistry and defaultDataset back the package-level convenience
// functions below. They are set up by Init.
var (
	defaultRegistry = NewRegistry()
	defaultDataset  *Dataset
)

// DefaultRegistry returns the process-wide registry.
func DefaultRegistry() *Registry { return defaultRegistry }

// Default returns the default dataset (the one named in Init). It is nil
// until Init has run.
func Default() *Dataset { return defaultDataset }

// Init ensures the data directory exists. It must run before Load.
func Init(dataset string) {
	ensureDataDir()
}

// Load reads the default dataset's persisted data from disk into memory and
// registers it in the default registry. The on-disk format (v2 folder or v1
// flat file) is detected automatically. Init must have run first.
func Load(dataset string) {
	d := defaultRegistry.Load(dataset)
	defaultDataset = d
	slog.Info("dataset loaded", "name", d.Name(), "data", d.dataPath())
}

// --- Package-level convenience wrappers over the default dataset ----------
//
// These preserve the original store API so handlers need no changes in this
// phase. Each simply delegates to the default dataset.

func GetStats() DatasetStats { return defaultDataset.GetStats() }

// GetMetrics returns the default dataset's query-metrics snapshot.
func GetMetrics() MetricsReport { return defaultDataset.Metrics().Snapshot() }

// GetInfo returns the default dataset's info aggregate (meta + stats +
// memory + metrics).
func GetInfo() DatasetInfo { return defaultDataset.Info() }

// ListDatasets returns info for every loaded dataset.
func ListDatasets() []DatasetInfo { return defaultRegistry.List() }

func GetAllContentTypes() []ContentType { return defaultDataset.GetAllContentTypes() }

func CreateContentType(ct ContentType) { defaultDataset.CreateContentType(ct) }

func SchemaExists(contentType string) bool { return defaultDataset.SchemaExists(contentType) }

func ListContent(contentType string) ([]ContentItem, bool) {
	return defaultDataset.ListContent(contentType)
}

func GetSingleContent(contentType, idStr string) (ContentItem, bool) {
	return defaultDataset.GetSingleContent(contentType, idStr)
}

func GetContentType(contentType string) (ContentType, error) {
	return defaultDataset.GetContentType(contentType)
}

func CreateContent(contentType string, item ContentItem) (ContentItem, error) {
	return defaultDataset.CreateContent(contentType, item)
}

func UpdateContent(contentType, idStr string, update ContentItem) (ContentItem, error) {
	return defaultDataset.UpdateContent(contentType, idStr, update)
}

func DeleteContent(contentType, idStr string) bool {
	return defaultDataset.DeleteContent(contentType, idStr)
}

func FilterContent(contentType string, filters map[string]string) ([]ContentItem, bool) {
	return defaultDataset.FilterContent(contentType, filters)
}
