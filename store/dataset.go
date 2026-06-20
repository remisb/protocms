package store

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

// persistedData is the on-disk format for a dataset.
type persistedData struct {
	NextID  int                      `json:"next_id"`
	Schemas map[string]ContentType   `json:"schemas"`
	Content map[string][]ContentItem `json:"content"`
}

// DatasetStats holds statistics about a dataset.
type DatasetStats struct {
	Dataset      string         `json:"dataset"`
	ContentTypes int            `json:"content_types"`
	TotalItems   int            `json:"total_items"`
	ItemsPerType map[string]int `json:"items_per_type"`
}

// Dataset is a single, independently-loadable content store. Its own
// RWMutex guards the schemas/content/nextID state, so multiple datasets
// can be served concurrently without contending on a global lock.
type Dataset struct {
	mu       sync.RWMutex
	name     string
	dataFile string // resolved path, e.g. data/<name>.json
	schemas  map[string]ContentType
	content  map[string][]ContentItem
	nextID   int
}

// newDataset returns an empty dataset bound to the given name and on-disk
// file. It does not read from disk; call load for that.
func newDataset(name, dataFile string) *Dataset {
	return &Dataset{
		name:     name,
		dataFile: dataFile,
		schemas:  make(map[string]ContentType),
		content:  make(map[string][]ContentItem),
		nextID:   1,
	}
}

// Name returns the dataset's name.
func (d *Dataset) Name() string { return d.name }

// load reads persisted data from disk into the dataset. A missing file is
// not an error (first run); the dataset stays empty.
func (d *Dataset) load() {
	f, err := os.Open(d.dataFile)
	if os.IsNotExist(err) {
		return // first run, nothing to load
	}
	if err != nil {
		slog.Warn("could not open data file", "file", d.dataFile, "err", err)
		return
	}
	defer f.Close()

	var pd persistedData
	if err := json.NewDecoder(f).Decode(&pd); err != nil {
		slog.Warn("could not decode data file", "file", d.dataFile, "err", err)
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if pd.Schemas != nil {
		d.schemas = pd.Schemas
	}
	if pd.Content != nil {
		d.content = pd.Content
	}
	if pd.NextID > 0 {
		d.nextID = pd.NextID
	}
}

// persistLocked writes the current state to disk.
// Caller MUST hold d.mu (write lock) before calling this.
func (d *Dataset) persistLocked() {
	pd := persistedData{
		NextID:  d.nextID,
		Schemas: d.schemas,
		Content: d.content,
	}
	f, err := os.CreateTemp(dataDir, "protocms-*.json")
	if err != nil {
		slog.Warn("persist: could not create temp file", "err", err)
		return
	}
	tmpName := f.Name()
	if err := json.NewEncoder(f).Encode(pd); err != nil {
		f.Close()
		os.Remove(tmpName)
		slog.Warn("persist: could not encode data", "err", err)
		return
	}
	f.Close()
	if err := os.Rename(tmpName, d.dataFile); err != nil {
		os.Remove(tmpName)
		slog.Warn("persist: could not rename temp file", "tmp", tmpName, "dest", d.dataFile, "err", err)
	}
}

// GetStats returns counts for the dataset.
func (d *Dataset) GetStats() DatasetStats {
	d.mu.RLock()
	defer d.mu.RUnlock()
	itemsPerType := make(map[string]int, len(d.schemas))
	total := 0
	for name := range d.schemas {
		count := len(d.content[name])
		itemsPerType[name] = count
		total += count
	}
	return DatasetStats{
		Dataset:      d.name,
		ContentTypes: len(d.schemas),
		TotalItems:   total,
		ItemsPerType: itemsPerType,
	}
}

func (d *Dataset) GetAllContentTypes() []ContentType {
	d.mu.RLock()
	defer d.mu.RUnlock()
	types := make([]ContentType, 0, len(d.schemas))
	for _, t := range d.schemas {
		types = append(types, t)
	}
	return types
}

func (d *Dataset) CreateContentType(ct ContentType) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.schemas[ct.Name] = ct
	if _, ok := d.content[ct.Name]; !ok {
		d.content[ct.Name] = []ContentItem{}
	}
	d.persistLocked()
}

func (d *Dataset) SchemaExists(contentType string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	_, ok := d.schemas[contentType]
	return ok
}

func (d *Dataset) ListContent(contentType string) ([]ContentItem, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	items, exists := d.content[contentType]
	return items, exists
}

func (d *Dataset) GetSingleContent(contentType, idStr string) (ContentItem, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, item := range d.content[contentType] {
		if fmt.Sprintf("%v", item["id"]) == idStr {
			return item, true
		}
	}
	return nil, false
}

func (d *Dataset) GetContentType(contentType string) (ContentType, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	cType, ok := d.schemas[contentType]
	if !ok {
		return ContentType{}, fmt.Errorf("unknown content type %s", contentType)
	}
	return cType, nil
}

func (d *Dataset) CreateContent(contentType string, item ContentItem) (ContentItem, error) {
	schema, err := d.GetContentType(contentType)
	if err != nil {
		return ContentItem{}, err
	}

	// Validate against schema
	for fieldName, fieldDef := range schema.Fields {
		if err := validateField(item[fieldName], fieldDef); err != nil {
			return ContentItem{}, fmt.Errorf("invalid field %s", fieldName)
		}
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	item["id"] = d.nextID
	d.nextID++
	d.content[contentType] = append(d.content[contentType], item)
	d.persistLocked()
	return item, nil
}

func (d *Dataset) UpdateContent(contentType, idStr string, update ContentItem) (ContentItem, error) {
	if _, err := d.GetContentType(contentType); err != nil {
		return ContentItem{}, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	items := d.content[contentType]
	for i, item := range items {
		if fmt.Sprintf("%v", item["id"]) == idStr {
			for k, v := range update {
				if k != "id" {
					item[k] = v
				}
			}
			items[i] = item
			d.persistLocked()
			return item, nil
		}
	}
	return ContentItem{}, fmt.Errorf("content item with id %s not found", idStr)
}

func (d *Dataset) DeleteContent(contentType, idStr string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	items := d.content[contentType]
	for i, item := range items {
		if fmt.Sprintf("%v", item["id"]) == idStr {
			d.content[contentType] = append(items[:i], items[i+1:]...)
			d.persistLocked()
			return true
		}
	}
	return false
}

// FilterContent returns items of contentType where every field in filters matches.
// Matching is done by string-converting stored values, which cover numbers, booleans, and strings.
func (d *Dataset) FilterContent(contentType string, filters map[string]string) ([]ContentItem, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	items, exists := d.content[contentType]
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

// validateField checks if a value matches the field definition.
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
