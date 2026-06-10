package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"version":   "Go 1.26 stdlib POC",
		"timestamp": time.Now().UTC().String(),
	})
}

func statsHandler(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, storeGetStats())
}

// Content Types
func getContentTypesHandler(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, storeGetAllContentTypes())
}

func createContentTypeHandler(w http.ResponseWriter, r *http.Request) {
	var ct ContentType
	if err := json.NewDecoder(r.Body).Decode(&ct); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if ct.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}
	storeCreateContentType(ct)
	jsonResponse(w, http.StatusCreated, ct)
}

// List content (GET /api/content/{contentType})
func listContentHandler(w http.ResponseWriter, r *http.Request) {
	contentType := r.PathValue("contentType")
	if contentType == "" {
		http.Error(w, `{"error":"content type required"}`, http.StatusBadRequest)
		return
	}

	// Collect field filters from query params, excluding reserved keys.
	reserved := map[string]bool{"limit": true}
	filters := make(map[string]string)
	for key, vals := range r.URL.Query() {
		if !reserved[key] {
			filters[key] = vals[0]
		}
	}

	var items []ContentItem
	var exists bool
	if len(filters) > 0 {
		items, exists = storeFilterContent(contentType, filters)
	} else {
		items, exists = storeListContent(contentType)
	}

	if !exists {
		jsonResponse(w, http.StatusOK, []ContentItem{})
		return
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 && limit < len(items) {
			items = append([]ContentItem{}, items[:limit]...)
		}
	}
	jsonResponse(w, http.StatusOK, items)
}

// Get single item (GET /api/content/{contentType}/{id})
func getSingleContentHandler(w http.ResponseWriter, r *http.Request) {
	contentType := r.PathValue("contentType")
	idStr := r.PathValue("id")
	if contentType == "" || idStr == "" {
		http.Error(w, `{"error":"content type and id required"}`, http.StatusBadRequest)
		return
	}

	item, found := storeGetSingleContent(contentType, idStr)
	if !found {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	jsonResponse(w, http.StatusOK, item)
}

func createContentHandler(w http.ResponseWriter, r *http.Request) {
	contentType := r.PathValue("contentType")
	if contentType == "" {
		http.Error(w, `{"error":"content type required"}`, http.StatusBadRequest)
		return
	}
	if !storeSchemaExists(contentType) {
		http.Error(w, `{"error":"unknown content type. Register first via /content-types"}`, http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}
	var item ContentItem
	if err := json.Unmarshal(body, &item); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	jsonResponse(w, http.StatusCreated, storeCreateContent(contentType, item))
}

func updateContentHandler(w http.ResponseWriter, r *http.Request) {
	contentType := r.PathValue("contentType")
	idStr := r.PathValue("id")
	if contentType == "" || idStr == "" {
		http.Error(w, `{"error":"type and id required"}`, http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}
	var update ContentItem
	json.Unmarshal(body, &update)

	item, found := storeUpdateContent(contentType, idStr, update)
	if !found {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	jsonResponse(w, http.StatusOK, item)
}

func deleteContentHandler(w http.ResponseWriter, r *http.Request) {
	contentType := r.PathValue("contentType")
	idStr := r.PathValue("id")
	if contentType == "" || idStr == "" {
		http.Error(w, `{"error":"type and id required"}`, http.StatusBadRequest)
		return
	}

	if !storeDeleteContent(contentType, idStr) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	jsonResponse(w, http.StatusNoContent, nil)
}

func jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}
