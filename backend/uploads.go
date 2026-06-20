package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/remisb/protocms/store"
)

const maxUploadSize = 8 << 20 // 8 MiB

var allowedImageExt = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
}

// uploadHandler accepts multipart/form-data with a "file" field and writes it
// to the request dataset's uploads folder (data/<dataset>/uploads/<random>.<ext>).
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	d := datasetForRequest(w, r)
	if d == nil {
		return
	}
	uploadDir, err := d.UploadDir()
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusConflict)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		http.Error(w, `{"error":"file too large or invalid form"}`, http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, `{"error":"missing 'file' field"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if _, ok := allowedImageExt[ext]; !ok {
		http.Error(w, `{"error":"unsupported image type"}`, http.StatusBadRequest)
		return
	}

	name, err := randomName(ext)
	if err != nil {
		http.Error(w, `{"error":"could not generate filename"}`, http.StatusInternalServerError)
		return
	}

	dstPath := filepath.Join(uploadDir, name)
	dst, err := os.Create(dstPath)
	if err != nil {
		http.Error(w, `{"error":"could not write file"}`, http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		os.Remove(dstPath)
		http.Error(w, `{"error":"could not save file"}`, http.StatusInternalServerError)
		return
	}

	jsonResponse(w, http.StatusCreated, map[string]string{
		"url":      "/api/uploads/" + d.Name() + "/" + name,
		"name":     name,
		"original": header.Filename,
	})
}

// serveUploadHandler streams a previously uploaded file from a dataset's
// uploads folder. The route is public (GET /api/uploads/{dataset}/{name}) so
// plain <img> tags can load images; the dataset is taken from the path. Both
// the dataset name and file name are validated to prevent path traversal.
func serveUploadHandler(w http.ResponseWriter, r *http.Request) {
	dataset := r.PathValue("dataset")
	name := r.PathValue("name")
	if !safeDatasetName(dataset) {
		http.Error(w, `{"error":"invalid dataset"}`, http.StatusBadRequest)
		return
	}
	if !safeUploadName(name) {
		http.Error(w, `{"error":"invalid name"}`, http.StatusBadRequest)
		return
	}
	d, ok := store.DefaultRegistry().Get(dataset)
	if !ok {
		d = store.DefaultRegistry().Load(dataset)
	}
	uploadDir, err := d.UploadDir()
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusConflict)
		return
	}
	path := filepath.Join(uploadDir, name)
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"cannot read file"}`, http.StatusInternalServerError)
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		http.Error(w, `{"error":"cannot stat file"}`, http.StatusInternalServerError)
		return
	}
	if ct, ok := allowedImageExt[strings.ToLower(filepath.Ext(name))]; ok {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, name, stat.ModTime(), f)
}

func randomName(ext string) (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%s", hex.EncodeToString(buf[:]), ext), nil
}

// safeDatasetName disallows path separators and dotfiles so a dataset name
// from the URL cannot escape the data directory.
func safeDatasetName(name string) bool {
	if name == "" || strings.ContainsRune(name, '/') || strings.ContainsRune(name, '\\') {
		return false
	}
	return !strings.HasPrefix(name, ".")
}

// safeUploadName disallows path separators and dotfiles, and requires
// a known image extension.
func safeUploadName(name string) bool {
	if name == "" || strings.ContainsRune(name, '/') || strings.ContainsRune(name, '\\') {
		return false
	}
	if strings.HasPrefix(name, ".") {
		return false
	}
	_, ok := allowedImageExt[strings.ToLower(filepath.Ext(name))]
	return ok
}
