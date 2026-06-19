package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/remisb/muxstack/middleware"
	"github.com/remisb/protocms/internal/auth"
	"github.com/remisb/protocms/store"
)

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"version":   "Go 1.26 stdlib POC with Field Types",
		"timestamp": time.Now().UTC().String(),
	})
}

func statsHandler(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, http.StatusOK, store.GetStats())
}

// meHandler returns the identity and role of the current bearer token.
// Registered behind the Authenticator middleware, so Claims are always present.
func meHandler(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"subject": claims.Subject,
		"roles":   claims.Roles,
	})
}

// Content Types
func getContentTypesHandler(w http.ResponseWriter, _ *http.Request) {
	jsonResponse(w, http.StatusOK, store.GetAllContentTypes())
}

func createContentTypeHandler(w http.ResponseWriter, r *http.Request) {
	var ct store.ContentType
	if err := json.NewDecoder(r.Body).Decode(&ct); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	// validate request data
	if ct.Name == "" || len(ct.Fields) == 0 {
		http.Error(w, `{"error":"name and fields are required"}`, http.StatusBadRequest)
		return
	}
	store.CreateContentType(ct)
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

	var items []store.ContentItem
	var exists bool
	if len(filters) > 0 {
		items, exists = store.FilterContent(contentType, filters)
	} else {
		items, exists = store.ListContent(contentType)
	}

	if !exists {
		jsonResponse(w, http.StatusOK, []store.ContentItem{})
		return
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 && limit < len(items) {
			items = append([]store.ContentItem{}, items[:limit]...)
		}
	}
	jsonResponse(w, http.StatusOK, items)
}

// Get a single item (GET /api/content/{contentType}/{id})
func getSingleContentHandler(w http.ResponseWriter, r *http.Request) {
	contentType := r.PathValue("contentType")
	idStr := r.PathValue("id")
	if contentType == "" || idStr == "" {
		http.Error(w, `{"error":"content type and id required"}`, http.StatusBadRequest)
		return
	}

	item, found := store.GetSingleContent(contentType, idStr)
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
	if !store.SchemaExists(contentType) {
		http.Error(w, `{"error":"unknown content type. Register first via /content-types"}`, http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}
	var item store.ContentItem
	if err := json.Unmarshal(body, &item); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	content, err := store.CreateContent(contentType, item)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to create content: %v"}`, err), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, http.StatusCreated, content)
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
	var update store.ContentItem
	json.Unmarshal(body, &update)

	item, err := store.UpdateContent(contentType, idStr, update)
	if err != nil {
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

	if !store.DeleteContent(contentType, idStr) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	jsonResponse(w, http.StatusNoContent, nil)
}

// loginHandler authenticates a username/password against PROTOCMS_USERS and
// issues a JWT. Registered as POST /api/login (public).
func loginHandler(cfg auth.Config) http.HandlerFunc {
	type creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	type loginResponse struct {
		Token     string `json:"token"`
		Role      string `json:"role"`
		ExpiresIn int    `json:"expires_in"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.IsJWTAuthDisabled() {
			http.Error(w, `{"error":"login disabled"}`, http.StatusServiceUnavailable)
			return
		}

		var credentials creds
		if err := json.NewDecoder(r.Body).Decode(&credentials); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}

		user, err := cfg.GetUser(credentials.Username, credentials.Password)
		if err != nil && errors.Is(err, auth.ErrInvalidCredentials) {
			http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
			return
		}

		token, err := auth.SignJWT(cfg, credentials.Username, user, jwtTTL)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to sign token: %v"}`, err), http.StatusInternalServerError)
			return
		}

		jsonResponse(w, http.StatusOK,
			loginResponse{Token: token, Role: user.Role(), ExpiresIn: int(jwtTTL.Seconds())},
		)
	}
}

func jsonResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}
