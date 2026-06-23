package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Data format versions for the on-disk container layout.
//
//	v2 = folder            data/<name>/{data.json,meta.json}
const (
	formatVersionCurrent = 2
)

const metaFileName = "meta.json"

// nowUTC returns the current time in UTC. Centralized so timestamps across
// metadata are consistent.
func nowUTC() time.Time { return time.Now().UTC() }

// Metadata describes a dataset: identity, provenance, versions, and tags.
// It is persisted as data/<name>/meta.json in the v2 folder format.
type Metadata struct {
	Name              string    `json:"name"`
	Author            string    `json:"author"`
	Description       string    `json:"description"`
	CreatedAt         time.Time `json:"created_at"`
	ModifiedAt        time.Time `json:"modified_at"`
	DataFormatVersion int       `json:"data_format_version"`
	SchemaVersion     int       `json:"schema_version"`
	Tags              []string  `json:"tags"`
}

// newMetadata returns metadata for a freshly created/migrated dataset,
// with both timestamps set to now and current versions.
func newMetadata(name string, now time.Time) Metadata {
	return Metadata{
		Name:              name,
		CreatedAt:         now,
		ModifiedAt:        now,
		DataFormatVersion: formatVersionCurrent,
		SchemaVersion:     1,
		Tags:              []string{},
	}
}

// loadMetadata reads meta.json from a dataset directory.
func loadMetadata(dir string) (Metadata, error) {
	var m Metadata
	b, err := os.ReadFile(filepath.Join(dir, metaFileName))
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, err
	}
	if m.Tags == nil {
		m.Tags = []string{}
	}
	return m, nil
}

// saveMetadata writes meta.json into a dataset directory atomically
// (temp file + rename).
// REMIS why it is not a method of Dataset structure
func saveMetadata(dir string, m Metadata) error {
	if m.Tags == nil {
		m.Tags = []string{}
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "protocms-meta-*.json")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, filepath.Join(dir, metaFileName)); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
