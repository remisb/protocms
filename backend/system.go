package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/remisb/protocms/internal/auth"
	"github.com/remisb/protocms/store"
)

// syncSystemCredentials reads the current _system users and API keys and pushes
// them into the auth Config's system layer. Env credentials still take
// precedence (see internal/auth/system.go). Call this at startup and after any
// _system credential mutation so changes take effect in-process without a
// restart.
func syncSystemCredentials(cfg auth.Config) {
	sys := store.DefaultRegistry().System()

	keys := make([]auth.SystemKeyCred, 0)
	for _, k := range sys.Keys() {
		if k.Revoked() {
			continue // revoked keys must not authenticate
		}
		keys = append(keys, auth.SystemKeyCred{
			Prefix:  k.Prefix,
			Hash:    k.Hash,
			Role:    k.Role,
			Dataset: k.Dataset,
			Verify:  store.VerifySecret,
		})
	}

	users := make(map[string]auth.SystemUserCred)
	for _, u := range sys.Users() {
		if !u.Active() {
			continue // only active users may authenticate (see SystemUser.Active)
		}
		users[u.Username] = auth.SystemUserCred{
			PasswordHash: u.PasswordHash,
			Role:         u.Role,
			Dataset:      u.Dataset,
			Verify:       store.VerifySecret,
		}
	}

	cfg.SetSystemCredentials(keys, users)
}

// bootstrapGuard warns loudly when no admin credential exists in either env or
// _system. Because env overrides _system, an env admin key is always a usable
// escape hatch; this only flags the lockout case so the operator notices.
func bootstrapGuard(cfg auth.Config) {
	// Only an admin credential can reach the admin-only /api/system routes, so
	// an editor-only env key must not suppress the warning.
	if cfg.HasAdminAPIKey() {
		return
	}
	if store.DefaultRegistry().System().HasAdmin() {
		return
	}
	slog.Warn("no admin credential found in PROTOCMS_API_KEYS or the _system store: " +
		"create one via env (PROTOCMS_API_KEYS=key:admin) before managing /api/system/*")
}

// --- /api/system/* handlers ----------------------------------------------
//
// All routes are admin-only (enforced at registration). They are the only
// door into the reserved _system dataset; the tenant content API rejects any
// credential bound to a "_"-prefixed dataset (see datasetForRequest).

func systemUsersHandler(cfg auth.Config) http.HandlerFunc {
	type createReq struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
		Dataset  string `json:"dataset"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		sys := store.DefaultRegistry().System()
		switch r.Method {
		case http.MethodGet:
			jsonResponse(w, http.StatusOK, redactUsers(sys.Users()))
		case http.MethodPost:
			var req createReq
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
				return
			}
			if req.Username == "" || req.Password == "" || !auth.ValidRole(req.Role) {
				http.Error(w, `{"error":"username, password, and a valid role (admin|editor) are required"}`, http.StatusBadRequest)
				return
			}
			ds := req.Dataset
			if ds == "" {
				ds = "default"
			}
			if store.ReservedDatasetName(ds) {
				http.Error(w, `{"error":"cannot bind a credential to a reserved dataset"}`, http.StatusBadRequest)
				return
			}
			u, err := sys.CreateUser(req.Username, req.Password, req.Role, ds)
			if err == store.ErrUserExists {
				http.Error(w, `{"error":"user already exists"}`, http.StatusConflict)
				return
			}
			if err != nil {
				http.Error(w, `{"error":"failed to create user"}`, http.StatusInternalServerError)
				return
			}
			syncSystemCredentials(cfg)
			jsonResponse(w, http.StatusCreated, redactUser(u))
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	}
}

func deleteSystemUserHandler(cfg auth.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
			return
		}
		if !store.DefaultRegistry().System().DeleteUser(id) {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		syncSystemCredentials(cfg)
		jsonResponse(w, http.StatusNoContent, nil)
	}
}

func systemKeysHandler(cfg auth.Config) http.HandlerFunc {
	type createReq struct {
		Name    string `json:"name"`
		Role    string `json:"role"`
		Dataset string `json:"dataset"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		sys := store.DefaultRegistry().System()
		switch r.Method {
		case http.MethodGet:
			jsonResponse(w, http.StatusOK, redactKeys(sys.Keys()))
		case http.MethodPost:
			var req createReq
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
				return
			}
			if req.Name == "" || !auth.ValidRole(req.Role) {
				http.Error(w, `{"error":"name and a valid role (admin|editor) are required"}`, http.StatusBadRequest)
				return
			}
			ds := req.Dataset
			if ds == "" {
				ds = "default"
			}
			if store.ReservedDatasetName(ds) {
				http.Error(w, `{"error":"cannot bind a credential to a reserved dataset"}`, http.StatusBadRequest)
				return
			}
			k, plaintext, err := sys.CreateKey(req.Name, req.Role, ds)
			if err != nil {
				http.Error(w, `{"error":"failed to create key"}`, http.StatusInternalServerError)
				return
			}
			syncSystemCredentials(cfg)
			// The plaintext key is shown exactly once, here.
			jsonResponse(w, http.StatusCreated, map[string]any{
				"id":      k.ID,
				"name":    k.Name,
				"prefix":  k.Prefix,
				"role":    k.Role,
				"dataset": k.Dataset,
				"key":     plaintext,
			})
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	}
}

func deleteSystemKeyHandler(cfg auth.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
			return
		}
		if !store.DefaultRegistry().System().RevokeKey(id) {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		syncSystemCredentials(cfg)
		jsonResponse(w, http.StatusNoContent, nil)
	}
}

// redactUser strips the password hash from a user record before it leaves the
// API. The hash never needs to be exposed.
func redactUser(u store.SystemUser) map[string]any {
	return map[string]any{
		"id":         u.ID,
		"username":   u.Username,
		"role":       u.Role,
		"dataset":    u.Dataset,
		"status":     u.Status,
		"created_at": u.CreatedAt,
		"updated_at": u.UpdatedAt,
	}
}

func redactUsers(users []store.SystemUser) []map[string]any {
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		out = append(out, redactUser(u))
	}
	return out
}

// redactKey strips the stored hash from a key record before it leaves the API.
func redactKey(k store.SystemKey) map[string]any {
	return map[string]any{
		"id":           k.ID,
		"name":         k.Name,
		"prefix":       k.Prefix,
		"role":         k.Role,
		"dataset":      k.Dataset,
		"created_at":   k.CreatedAt,
		"last_used_at": k.LastUsedAt,
		"revoked_at":   k.RevokedAt,
	}
}

func redactKeys(keys []store.SystemKey) []map[string]any {
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, redactKey(k))
	}
	return out
}
