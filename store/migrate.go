package store

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MigrateResult summarizes a single dataset migration.
type MigrateResult struct {
	Name            string
	OldFile         string
	NewDir          string
	Items           int
	Types           int
	UploadsMigrated int
}

// legacyUploadDir is the shared, pre-v2 uploads folder.
const legacyUploadDir = dataDir + "/uploads"

// legacyUploadPrefix is the URL shape stored by the old dataset-less upload
// route: /api/uploads/<name>.
const legacyUploadPrefix = "/api/uploads/"

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

	// Create the folder.
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		return res, fmt.Errorf("create %s: %w", newDir, err)
	}

	// Move any shared uploads this dataset references into its own folder,
	// rewriting the stored URLs from /api/uploads/<file> to
	// /api/uploads/<dataset>/<file>.
	moved, err := migrateUploads(name, newDir, pd.Content)
	if err != nil {
		return res, fmt.Errorf("migrate uploads: %w", err)
	}
	res.UploadsMigrated = moved

	// Write data.json + meta.json (data.json now has rewritten upload URLs).
	if err := writeDataFile(filepath.Join(newDir, dataFileName), pd); err != nil {
		return res, fmt.Errorf("write data.json: %w", err)
	}
	if err := saveMetadata(newDir, newMetadata(name, nowUTC())); err != nil {
		return res, fmt.Errorf("write meta.json: %w", err)
	}
	return res, nil
}

// migrateUploads copies shared-folder uploads referenced by this dataset's
// content into <newDir>/uploads/ and rewrites the stored URLs to include the
// dataset name. It mutates the content items in place. The shared source file
// is left in place (no data files are deleted by migration). Returns the
// number of distinct files copied.
func migrateUploads(dataset, newDir string, content map[string][]ContentItem) (int, error) {
	destDir := filepath.Join(newDir, uploadsSubdir)
	copied := map[string]bool{}

	for _, items := range content {
		for _, item := range items {
			for key, val := range item {
				s, ok := val.(string)
				if !ok || !strings.HasPrefix(s, legacyUploadPrefix) {
					continue
				}
				file := strings.TrimPrefix(s, legacyUploadPrefix)
				// Skip anything that already carries a dataset segment.
				if file == "" || strings.Contains(file, "/") {
					continue
				}
				src := filepath.Join(legacyUploadDir, file)
				if !fileExists(src) {
					// Referenced file isn't in the shared dir; leave the URL as
					// is rather than fabricating a broken copy.
					continue
				}
				if !copied[file] {
					if err := os.MkdirAll(destDir, 0o755); err != nil {
						return len(copied), err
					}
					if err := copyFile(src, filepath.Join(destDir, file)); err != nil {
						return len(copied), err
					}
					copied[file] = true
				}
				item[key] = legacyUploadPrefix + dataset + "/" + file
			}
		}
	}
	return len(copied), nil
}

// copyFile copies src to dst, creating dst (does not overwrite an existing
// dst — migration should not clobber).
func copyFile(src, dst string) error {
	if fileExists(dst) {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
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
