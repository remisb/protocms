package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// MigrateResult summarizes a single dataset migration.
type MigrateResult struct {
	Name    string
	OldFile string
	NewDir  string
	Items   int
	Types   int
}

// MigrateDataset converts a dataset from the legacy v1 flat file
// (data/<name>.json) to the v2 folder format (data/<name>/{data.json,
// meta.json}).
//
// It never deletes the old file (manual cleanup, by design) and refuses to
// overwrite an existing folder. The data.json content is copied byte-for-byte
// in shape; a fresh meta.json is written with created_at/modified_at = now,
// data_format_version = 2, schema_version = 1.
func MigrateDataset(name string) (MigrateResult, error) {
	ensureDataDir()

	oldFile := filepath.Join(dataDir, name+".json")
	newDir := filepath.Join(dataDir, name)
	var res MigrateResult
	res.Name = name
	res.OldFile = oldFile
	res.NewDir = newDir

	if !fileExists(oldFile) {
		return res, fmt.Errorf("legacy dataset file not found: %s", oldFile)
	}
	if isDir(newDir) {
		return res, fmt.Errorf("target folder already exists, refusing to overwrite: %s", newDir)
	}

	// Read and validate the legacy file.
	raw, err := os.ReadFile(oldFile)
	if err != nil {
		return res, fmt.Errorf("read %s: %w", oldFile, err)
	}
	var pd persistedData
	if err := json.Unmarshal(raw, &pd); err != nil {
		return res, fmt.Errorf("parse %s: %w", oldFile, err)
	}
	res.Types = len(pd.Schemas)
	for _, items := range pd.Content {
		res.Items += len(items)
	}

	// Create the folder and write data.json + meta.json.
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		return res, fmt.Errorf("create %s: %w", newDir, err)
	}
	if err := writeDataFile(filepath.Join(newDir, dataFileName), pd); err != nil {
		return res, fmt.Errorf("write data.json: %w", err)
	}
	if err := saveMetadata(newDir, newMetadata(name, nowUTC())); err != nil {
		return res, fmt.Errorf("write meta.json: %w", err)
	}
	return res, nil
}

// writeDataFile atomically writes persisted content to path.
func writeDataFile(path string, pd persistedData) error {
	b, err := json.MarshalIndent(pd, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "protocms-*.json")
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
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
