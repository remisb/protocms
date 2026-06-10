package main

import (
	"encoding/json"
	"fmt"
	"log"
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
const dataFile = "data/data.json"

// In-memory stores (data resets on restart)
var (
	mu           sync.RWMutex
	contentStore     = make(map[string][]ContentItem)
	schemas          = make(map[string]ContentType)
	nextID       int = 1
)

// storeLoad reads persisted data from disk into the in-memory store.
// Must be called once before the HTTP server starts.
func storeLoad() {
	f, err := os.Open(dataFile)
	if os.IsNotExist(err) {
		return // first run, nothing to load
	}
	if err != nil {
		log.Printf("warn: could not open %s: %v", dataFile, err)
		return
	}
	defer f.Close()

	var d persistedData
	if err := json.NewDecoder(f).Decode(&d); err != nil {
		log.Printf("warn: could not decode %s: %v", dataFile, err)
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
		log.Printf("warn: persist create temp: %v", err)
		return
	}
	tmpName := f.Name()
	if err := json.NewEncoder(f).Encode(d); err != nil {
		f.Close()
		os.Remove(tmpName)
		log.Printf("warn: persist encode: %v", err)
		return
	}
	f.Close()
	if err := os.Rename(tmpName, dataFile); err != nil {
		os.Remove(tmpName)
		log.Printf("warn: persist rename: %v", err)
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
