package main

import (
	"encoding/json"
	"net/http"

	"github.com/remisb/protocms/store"
)

// listDatasetsHandler returns info (meta + stats + memory + metrics) for every
// loaded dataset. Admin only.
func listDatasetsHandler(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, http.StatusOK, store.DefaultRegistry().List())
}

// getDatasetHandler returns info for one loaded dataset. Admin only.
func getDatasetHandler(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	d, ok := store.DefaultRegistry().Get(name)
	if !ok {
		http.Error(w, `{"error":"dataset not loaded"}`, http.StatusNotFound)
		return
	}
	jsonResponse(w, http.StatusOK, d.Info())
}

// loadDatasetHandler loads a dataset into memory at runtime. Admin only.
func loadDatasetHandler(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, `{"error":"dataset name required"}`, http.StatusBadRequest)
		return
	}
	d := store.DefaultRegistry().Load(name)
	jsonResponse(w, http.StatusOK, d.Info())
}

// unloadDatasetHandler drops a dataset from memory (files untouched). Admin
// only.
func unloadDatasetHandler(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !store.DefaultRegistry().Unload(name) {
		http.Error(w, `{"error":"dataset not loaded"}`, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// patchDatasetHandler updates editable metadata (author, description, tags,
// schema_version) for a loaded dataset. Admin only.
func patchDatasetHandler(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	d, ok := store.DefaultRegistry().Get(name)
	if !ok {
		http.Error(w, `{"error":"dataset not loaded"}`, http.StatusNotFound)
		return
	}
	var patch store.MetaPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	meta, err := d.UpdateMeta(patch)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusConflict)
		return
	}
	jsonResponse(w, http.StatusOK, meta)
}
