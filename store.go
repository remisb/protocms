package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
)

// ContentType defines a simple schema
type ContentType struct {
	Name   string            `json:"name"`
	Fields map[string]string `json:"fields,omitempty"`
}

// ContentItem is flexible for any content structure (using `any`)
type ContentItem map[string]any

// persistedData is the on-disk format for both files combined.
type persistedData struct {
	NextID  int                      `json:"next_id"`
	Schemas map[string]ContentType   `json:"schemas"`
	Content map[string][]ContentItem `json:"content"`
}

const dataDir = "data"

var dataFile string
var datasetName string

// In-memory stores (data resets on restart)
var (
	mu           sync.RWMutex
	contentStore     = make(map[string][]ContentItem)
	schemas          = make(map[string]ContentType)
	nextID       int = 1
)

// storeInit resolves the data file path for the given dataset name
// and ensures the data directory exists.
func storeInit(dataset string) {
	datasetName = dataset
	dataFile = dataDir + "/" + datasetName + ".json"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		slog.Error("could not create data directory", "dir", dataDir, "err", err)
		os.Exit(1)
	}
	slog.Info("data file", "path", dataFile)
}

// DatasetStats holds statistics about the active dataset.
type DatasetStats struct {
	Dataset      string         `json:"dataset"`
	ContentTypes int            `json:"content_types"`
	TotalItems   int            `json:"total_items"`
	ItemsPerType map[string]int `json:"items_per_type"`
}

func storeGetStats() DatasetStats {
	mu.RLock()
	defer mu.RUnlock()
	itemsPerType := make(map[string]int, len(schemas))
	total := 0
	for name := range schemas {
		count := len(contentStore[name])
		itemsPerType[name] = count
		total += count
	}
	return DatasetStats{
		Dataset:      datasetName,
		ContentTypes: len(schemas),
		TotalItems:   total,
		ItemsPerType: itemsPerType,
	}
}

// storeLoad reads persisted data from disk into the in-memory store.
// Must be called once before the HTTP server starts.
func storeLoad(dataset string) {
	f, err := os.Open(dataFile)
	if os.IsNotExist(err) && dataFile != dataDir+"/"+datasetName+".json" {
		return // first run, nothing to load
	}
	if err != nil {
		slog.Warn("could not open data file", "file", dataFile, "err", err)
		return
	}
	defer f.Close()

	var d persistedData
	if err := json.NewDecoder(f).Decode(&d); err != nil {
		slog.Warn("could not decode data file", "file", dataFile, "err", err)
		return
	}

	mu.Lock()
	defer mu.Unlock()
	if d.Schemas != nil {
		schemas = d.Schemas
	}
	if d.Content != nil {
		contentStore = d.Content
	}
	if d.NextID > 0 {
		nextID = d.NextID
	}
}

// persistLocked writes the current state to disk.
// Caller MUST hold mu (write lock) before calling this.
func persistLocked() {
	d := persistedData{
		NextID:  nextID,
		Schemas: schemas,
		Content: contentStore,
	}
	f, err := os.CreateTemp(dataDir, "protocms-*.json")
	if err != nil {
		slog.Warn("persist: could not create temp file", "err", err)
		return
	}
	tmpName := f.Name()
	if err := json.NewEncoder(f).Encode(d); err != nil {
		f.Close()
		os.Remove(tmpName)
		slog.Warn("persist: could not encode data", "err", err)
		return
	}
	f.Close()
	if err := os.Rename(tmpName, dataFile); err != nil {
		os.Remove(tmpName)
		slog.Warn("persist: could not rename temp file", "tmp", tmpName, "dest", dataFile, "err", err)
	}
}

func storeGetAllContentTypes() []ContentType {
	mu.RLock()
	defer mu.RUnlock()
	types := make([]ContentType, 0, len(schemas))
	for _, t := range schemas {
		types = append(types, t)
	}
	return types
}

func storeCreateContentType(ct ContentType) {
	mu.Lock()
	defer mu.Unlock()
	schemas[ct.Name] = ct
	if _, ok := contentStore[ct.Name]; !ok {
		contentStore[ct.Name] = []ContentItem{}
	}
	persistLocked()
}

func storeSchemaExists(contentType string) bool {
	mu.RLock()
	defer mu.RUnlock()
	_, ok := schemas[contentType]
	return ok
}

func storeListContent(contentType string) ([]ContentItem, bool) {
	mu.RLock()
	defer mu.RUnlock()
	items, exists := contentStore[contentType]
	return items, exists
}

func storeGetSingleContent(contentType, idStr string) (ContentItem, bool) {
	mu.RLock()
	defer mu.RUnlock()
	for _, item := range contentStore[contentType] {
		if fmt.Sprintf("%v", item["id"]) == idStr {
			return item, true
		}
	}
	return nil, false
}

func storeCreateContent(contentType string, item ContentItem) ContentItem {
	mu.Lock()
	defer mu.Unlock()
	item["id"] = nextID
	nextID++
	contentStore[contentType] = append(contentStore[contentType], item)
	persistLocked()
	return item
}

func storeUpdateContent(contentType, idStr string, update ContentItem) (ContentItem, bool) {
	mu.Lock()
	defer mu.Unlock()
	items := contentStore[contentType]
	for i, item := range items {
		if fmt.Sprintf("%v", item["id"]) == idStr {
			for k, v := range update {
				if k != "id" {
					item[k] = v
				}
			}
			items[i] = item
			persistLocked()
			return item, true
		}
	}
	return nil, false
}

func storeDeleteContent(contentType, idStr string) bool {
	mu.Lock()
	defer mu.Unlock()
	items := contentStore[contentType]
	for i, item := range items {
		if fmt.Sprintf("%v", item["id"]) == idStr {
			contentStore[contentType] = append(items[:i], items[i+1:]...)
			persistLocked()
			return true
		}
	}
	return false
}

// storeFilterContent returns items of contentType where every field in filters matches.
// Matching is done by string-converting stored values, which covers numbers, booleans and strings.
func storeFilterContent(contentType string, filters map[string]string) ([]ContentItem, bool) {
	mu.RLock()
	defer mu.RUnlock()
	items, exists := contentStore[contentType]
	if !exists {
		return nil, false
	}
	result := make([]ContentItem, 0)
	for _, item := range items {
		if itemMatchesFilters(item, filters) {
			result = append(result, item)
		}
	}
	return result, true
}

func itemMatchesFilters(item ContentItem, filters map[string]string) bool {
	for field, want := range filters {
		val, ok := item[field]
		if !ok {
			return false
		}
		if fmt.Sprintf("%v", val) != want {
			return false
		}
	}
	return true
}
