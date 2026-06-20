package store

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// dataFileName is the schemas/content file inside a v2 dataset folder.
const dataFileName = "data.json"

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
//
// A dataset is stored in one of two on-disk layouts (see format):
//
//	v2 (current): a folder data/<name>/ holding data.json + meta.json
//	v1 (legacy):  a flat file data/<name>.json (read + written in place,
//	              with no metadata, until migrated)
type Dataset struct {
	mu      sync.RWMutex
	name    string
	format  int      // formatVersionCurrent (folder) or formatVersionLegacy (flat file)
	dir     string   // data/<name>      (v2)
	flatPath string  // data/<name>.json (v1)
	meta    Metadata // populated for v2; zero-ish for v1
	schemas map[string]ContentType
	content map[string][]ContentItem
	nextID  int
	metrics *Metrics
}

// newFolderDataset returns an empty dataset backed by the v2 folder layout.
func newFolderDataset(name, dir string, meta Metadata) *Dataset {
	return &Dataset{
		name:    name,
		format:  formatVersionCurrent,
		dir:     dir,
		meta:    meta,
		schemas: make(map[string]ContentType),
		content: make(map[string][]ContentItem),
		nextID:  1,
		metrics: newMetrics(),
	}
}

// newLegacyDataset returns an empty dataset backed by the v1 flat file.
func newLegacyDataset(name, flatPath string) *Dataset {
	return &Dataset{
		name:     name,
		format:   formatVersionLegacy,
		flatPath: flatPath,
		schemas:  make(map[string]ContentType),
		content:  make(map[string][]ContentItem),
		nextID:   1,
		metrics:  newMetrics(),
	}
}

// Name returns the dataset's name.
func (d *Dataset) Name() string { return d.name }

// Meta returns a copy of the dataset's metadata.
func (d *Dataset) Meta() Metadata {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.meta
}

// Metrics returns the dataset's query-metrics recorder.
func (d *Dataset) Metrics() *Metrics { return d.metrics }

// MetaPatch carries the editable metadata fields. A nil pointer means "leave
// unchanged"; a non-nil pointer (including empty) overwrites.
type MetaPatch struct {
	Author        *string   `json:"author,omitempty"`
	Description   *string   `json:"description,omitempty"`
	Tags          *[]string `json:"tags,omitempty"`
	SchemaVersion *int      `json:"schema_version,omitempty"`
}

// UpdateMeta applies a metadata patch and persists it. Only legacy v1
// datasets reject this (no meta.json); they return an error.
func (d *Dataset) UpdateMeta(p MetaPatch) (Metadata, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.format != formatVersionCurrent {
		return Metadata{}, fmt.Errorf("dataset %q is in legacy format; migrate it before editing metadata", d.name)
	}
	if p.Author != nil {
		d.meta.Author = *p.Author
	}
	if p.Description != nil {
		d.meta.Description = *p.Description
	}
	if p.Tags != nil {
		d.meta.Tags = *p.Tags
	}
	if p.SchemaVersion != nil {
		d.meta.SchemaVersion = *p.SchemaVersion
	}
	d.meta.ModifiedAt = time.Now().UTC()
	if err := saveMetadata(d.dir, d.meta); err != nil {
		return Metadata{}, err
	}
	return d.meta, nil
}

// track returns a function that records a metrics observation for (op,
// contentType) with the elapsed time since track was called. Intended use:
//
//	defer d.track(OpList, contentType)()
func (d *Dataset) track(op, contentType string) func() {
	start := time.Now()
	return func() { d.metrics.Record(op, contentType, time.Since(start)) }
}

// dataPath returns the file the schemas/content JSON lives in for this
// dataset's format.
func (d *Dataset) dataPath() string {
	if d.format == formatVersionCurrent {
		return filepath.Join(d.dir, dataFileName)
	}
	return d.flatPath
}

// uploadsSubdir is the per-dataset uploads folder name inside a v2 dataset.
const uploadsSubdir = "uploads"

// UploadDir returns the dataset's uploads directory (data/<name>/uploads),
// creating it if needed. Only v2 folder datasets support uploads; legacy v1
// datasets return an error (migrate them first).
func (d *Dataset) UploadDir() (string, error) {
	if d.format != formatVersionCurrent {
		return "", fmt.Errorf("dataset %q is in legacy format; migrate it to enable uploads", d.name)
	}
	dir := filepath.Join(d.dir, uploadsSubdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// load reads persisted data from disk into the dataset. A missing file is
// not an error (first run); the dataset stays empty.
func (d *Dataset) load() {
	path := d.dataPath()
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return // first run, nothing to load
	}
	if err != nil {
		slog.Warn("could not open data file", "file", path, "err", err)
		return
	}
	defer f.Close()

	var pd persistedData
	if err := json.NewDecoder(f).Decode(&pd); err != nil {
		slog.Warn("could not decode data file", "file", path, "err", err)
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

// persistLocked writes the current state to disk. For the v2 folder format
// it also stamps meta.json's ModifiedAt. Writes are atomic (temp + rename).
// Caller MUST hold d.mu (write lock) before calling this.
func (d *Dataset) persistLocked() {
	path := d.dataPath()
	// The temp file must share a filesystem with the destination for the
	// rename to be atomic, so create it in the destination's directory.
	tmpDir := filepath.Dir(path)
	// For a brand-new v2 dataset the folder may not exist yet.
	if d.format == formatVersionCurrent {
		if err := os.MkdirAll(d.dir, 0o755); err != nil {
			slog.Warn("persist: could not create dataset dir", "dir", d.dir, "err", err)
			return
		}
	}
	pd := persistedData{
		NextID:  d.nextID,
		Schemas: d.schemas,
		Content: d.content,
	}
	f, err := os.CreateTemp(tmpDir, "protocms-*.json")
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
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		slog.Warn("persist: could not rename temp file", "tmp", tmpName, "dest", path, "err", err)
		return
	}

	// v2: stamp and write metadata alongside the data.
	if d.format == formatVersionCurrent {
		d.meta.ModifiedAt = time.Now().UTC()
		if err := saveMetadata(d.dir, d.meta); err != nil {
			slog.Warn("persist: could not write metadata", "dir", d.dir, "err", err)
		}
	}
}

// DatasetInfo aggregates a dataset's identity, size, and metrics for the
// management/list endpoints.
type DatasetInfo struct {
	Name        string        `json:"name"`
	Format      int           `json:"data_format_version"`
	Meta        Metadata      `json:"meta"`
	Stats       DatasetStats  `json:"stats"`
	ApproxBytes int           `json:"approx_bytes"`
	Metrics     MetricsReport `json:"metrics"`
}

// approxBytesLocked estimates the dataset's in-memory footprint by the
// marshaled size of its schemas+content. It is an estimate, not an exact
// allocation count (Go provides no per-object sizing). Caller holds d.mu.
func (d *Dataset) approxBytesLocked() int {
	n := 0
	if b, err := json.Marshal(d.content); err == nil {
		n += len(b)
	}
	if b, err := json.Marshal(d.schemas); err == nil {
		n += len(b)
	}
	return n
}

// Info returns a point-in-time aggregate of identity, stats, estimated
// memory, and query metrics.
func (d *Dataset) Info() DatasetInfo {
	d.mu.RLock()
	itemsPerType := make(map[string]int, len(d.schemas))
	total := 0
	for name := range d.schemas {
		count := len(d.content[name])
		itemsPerType[name] = count
		total += count
	}
	info := DatasetInfo{
		Name:   d.name,
		Format: d.format,
		Meta:   d.meta,
		Stats: DatasetStats{
			Dataset:      d.name,
			ContentTypes: len(d.schemas),
			TotalItems:   total,
			ItemsPerType: itemsPerType,
		},
		ApproxBytes: d.approxBytesLocked(),
	}
	d.mu.RUnlock()
	// Metrics has its own lock; snapshot outside d.mu to avoid lock nesting.
	info.Metrics = d.metrics.Snapshot()
	return info
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
	defer d.track(OpList, contentType)()
	d.mu.RLock()
	defer d.mu.RUnlock()
	items, exists := d.content[contentType]
	return items, exists
}

func (d *Dataset) GetSingleContent(contentType, idStr string) (ContentItem, bool) {
	defer d.track(OpGet, contentType)()
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
	defer d.track(OpCreate, contentType)()
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
	defer d.track(OpUpdate, contentType)()
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
	defer d.track(OpDelete, contentType)()
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
	defer d.track(OpFilter, contentType)()
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
