package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"
)

// FieldType represents supported field types
type FieldType string

const (
	FieldText      FieldType = "text"
	FieldTextarea  FieldType = "textarea"
	FieldRichText  FieldType = "richText"
	FieldNumber    FieldType = "number"
	FieldBoolean   FieldType = "boolean"
	FieldDate      FieldType = "date"
	FieldDateTime  FieldType = "datetime"
	FieldImage     FieldType = "image"
	FieldMedia     FieldType = "media"
	FieldSelect    FieldType = "select"
	FieldReference FieldType = "reference"
	FieldSlug      FieldType = "slug"
	FieldJSON      FieldType = "json"
)

// FieldDefinition defines a field in a content type schema
type FieldDefinition struct {
	Type     FieldType `json:"type"`
	Required bool      `json:"required,omitempty"`
	Options  []string  `json:"options,omitempty"` // for select/enum
	RefType  string    `json:"refType,omitempty"` // for reference
	Default  any       `json:"default,omitempty"`
}

// ContentType defines a simple schema
type ContentType struct {
	Name   string                     `json:"name"`
	Fields map[string]FieldDefinition `json:"fields,omitempty"`
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
	contentStore = make(map[string][]ContentItem)
	schemas      = make(map[string]ContentType)
	nextID       = 1
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

// ValidateField checks if a value matches the field definition
func validateField(value any, field FieldDefinition) error {
	if value == nil {
		if field.Required {
			return fmt.Errorf("field is required")
		}
		return nil
	}

	switch field.Type {
	case FieldText, FieldTextarea, FieldRichText, FieldSlug:
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string")
		}
	case FieldNumber:
		switch v := value.(type) {
		case float64, int, int64:
			// ok
		case string:
			if _, err := strconv.ParseFloat(v, 64); err != nil {
				return fmt.Errorf("expected number")
			}
		default:
			return fmt.Errorf("expected number")
		}
	case FieldBoolean:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected boolean")
		}
	case FieldDate, FieldDateTime:
		if s, ok := value.(string); ok {
			_, err := time.Parse(time.RFC3339, s)
			if err != nil {
				_, err = time.Parse("2006-01-02", s)
				if err != nil {
					return fmt.Errorf("invalid date format")
				}
			}
		} else {
			return fmt.Errorf("expected date string")
		}
	case FieldSelect:
		if str, ok := value.(string); ok {
			for _, opt := range field.Options {
				if opt == str {
					return nil
				}
			}
			return fmt.Errorf("value not in allowed options")
		}
	case FieldReference:
		if _, ok := value.(string); !ok { // simple ID reference
			return fmt.Errorf("expected reference ID (string)")
		}
	case FieldImage, FieldMedia:
		if _, ok := value.(string); !ok && value != nil {
			return fmt.Errorf("expected URL string for media")
		}
	case FieldJSON:
		// any is allowed
	default:
		// flexible
	}
	return nil
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

// storeLoad reads persisted data from the disk into the in-memory store.
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

func storeGetSchema(contentType string) (ContentType, bool) {
	mu.RLock()
	defer mu.RUnlock()
	schema, ok := schemas[contentType]
	return schema, ok
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

func storeGetContentType(contentType string) (ContentType, error) {
	mu.RLock()
	defer mu.RUnlock()
	cType, ok := schemas[contentType]
	if !ok {
		return ContentType{}, fmt.Errorf("unknown content type %s", contentType)
	}
	return cType, nil
}

func storeCreateContent(contentType string, item ContentItem) (ContentItem, error) {
	schema, err := storeGetContentType(contentType)
	if err != nil {
		return ContentItem{}, err
	}

	// Validate against schema
	for fieldName, fieldDef := range schema.Fields {
		if err := validateField(item[fieldName], fieldDef); err != nil {
			return ContentItem{}, fmt.Errorf("invalid field %s", fieldName)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	item["id"] = nextID
	nextID++
	contentStore[contentType] = append(contentStore[contentType], item)
	persistLocked()
	return item, nil
}

func storeUpdateContent(contentType, idStr string, update ContentItem) (ContentItem, error) {
	_, err := storeGetContentType(contentType)
	if err != nil {
		return ContentItem{}, err
	}

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
			return item, nil
		}
	}
	return ContentItem{}, fmt.Errorf("content item with id %s not found", idStr)
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
// Matching is done by string-converting stored values, which cover numbers, booleans, and strings.
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
