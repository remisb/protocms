package store

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// SystemDatasetName is the reserved dataset that holds system-level records
// (users and API keys). It is always loaded at startup and is never reachable
// through the tenant content API — see ReservedDatasetName.
const SystemDatasetName = "_system"

// Collections inside the _system dataset. They live in the same content map a
// normal dataset uses, but are accessed through the typed helpers below rather
// than the schema-validated CreateContent path.
const (
	systemUsersCollection = "users"
	systemKeysCollection  = "apikeys"
)

// ReservedDatasetName reports whether name is reserved for internal use and
// therefore must not be bindable by a tenant credential. The "_" prefix is
// reserved wholesale so future internal datasets need no extra plumbing.
func ReservedDatasetName(name string) bool {
	return strings.HasPrefix(name, "_")
}

var (
	// ErrUserExists is returned when creating a user whose username is taken.
	ErrUserExists = errors.New("user already exists")
	// ErrNotFound is returned when a system record cannot be located.
	ErrNotFound = errors.New("not found")
)

// SystemUser is a persisted user record in the _system dataset. The password
// is stored only as a salted hash; the plaintext is never retained.
type SystemUser struct {
	ID           int    `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	Role         string `json:"role"`
	Dataset      string `json:"dataset"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// UserStatusActive is the only status value that lets a user authenticate.
// Any other value (including an empty/unset status) is treated as inactive, so
// clearing the status disables the user.
const UserStatusActive = "active"

// Active reports whether the user may authenticate. It is the single source of
// truth shared by the auth-credential sync and the bootstrap guard, so they
// cannot disagree about which users count.
func (u SystemUser) Active() bool { return u.Status == UserStatusActive }

// SystemKey is a persisted API key record in the _system dataset. Only a
// salted hash of the key is stored; the plaintext is shown once at creation.
type SystemKey struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Prefix     string `json:"prefix"`
	Hash       string `json:"hash"`
	Role       string `json:"role"`
	Dataset    string `json:"dataset"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at,omitempty"`
	RevokedAt  string `json:"revoked_at,omitempty"`
}

// Revoked reports whether the key has been revoked and must be excluded from
// authentication.
func (k SystemKey) Revoked() bool { return k.RevokedAt != "" }

// SystemStore is a typed facade over the reserved _system dataset. It reuses
// the dataset's lock and atomic persistence; callers never touch the content
// maps directly.
type SystemStore struct {
	d *Dataset
}

// System returns a typed accessor over the registry's _system dataset,
// loading it if it is not already in memory.
func (r *Registry) System() *SystemStore {
	d, ok := r.Get(SystemDatasetName)
	if !ok {
		d = r.Load(SystemDatasetName)
	}
	return &SystemStore{d: d}
}

// --- Users ---------------------------------------------------------------

// Users returns all user records.
func (s *SystemStore) Users() []SystemUser {
	s.d.mu.RLock()
	defer s.d.mu.RUnlock()
	return decodeUsers(s.d.content[systemUsersCollection])
}

// UserByName returns the user with the given username, or ErrNotFound.
func (s *SystemStore) UserByName(username string) (SystemUser, error) {
	s.d.mu.RLock()
	defer s.d.mu.RUnlock()
	for _, u := range decodeUsers(s.d.content[systemUsersCollection]) {
		if u.Username == username {
			return u, nil
		}
	}
	return SystemUser{}, ErrNotFound
}

// CreateUser stores a new user, hashing the supplied plaintext password. The
// returned record carries the assigned ID. Usernames must be unique.
func (s *SystemStore) CreateUser(username, password, role, dataset string) (SystemUser, error) {
	hash, err := HashSecret(password)
	if err != nil {
		return SystemUser{}, err
	}
	now := nowUTC().Format(time.RFC3339)

	s.d.mu.Lock()
	defer s.d.mu.Unlock()
	users := decodeUsers(s.d.content[systemUsersCollection])
	for _, u := range users {
		if u.Username == username {
			return SystemUser{}, ErrUserExists
		}
	}
	u := SystemUser{
		ID:           s.d.nextID,
		Username:     username,
		PasswordHash: hash,
		Role:         role,
		Dataset:      dataset,
		Status:       UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.d.nextID++
	users = append(users, u)
	s.d.content[systemUsersCollection] = encodeUsers(users)
	s.d.persistLocked()
	return u, nil
}

// DeleteUser removes the user with the given id, reporting whether one was
// found.
func (s *SystemStore) DeleteUser(id int) bool {
	s.d.mu.Lock()
	defer s.d.mu.Unlock()
	users := decodeUsers(s.d.content[systemUsersCollection])
	for i, u := range users {
		if u.ID == id {
			users = append(users[:i], users[i+1:]...)
			s.d.content[systemUsersCollection] = encodeUsers(users)
			s.d.persistLocked()
			return true
		}
	}
	return false
}

// --- API keys ------------------------------------------------------------

// Keys returns all API key records (including revoked ones).
func (s *SystemStore) Keys() []SystemKey {
	s.d.mu.RLock()
	defer s.d.mu.RUnlock()
	return decodeKeys(s.d.content[systemKeysCollection])
}

// CreateKey generates a new random API key, stores its hash, and returns both
// the persisted record and the plaintext key. The plaintext is returned only
// here and is never persisted — the caller must surface it to the user once.
func (s *SystemStore) CreateKey(name, role, dataset string) (SystemKey, string, error) {
	plaintext, err := generateAPIKey()
	if err != nil {
		return SystemKey{}, "", err
	}
	hash, err := HashSecret(plaintext)
	if err != nil {
		return SystemKey{}, "", err
	}
	prefix := plaintext
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	now := nowUTC().Format(time.RFC3339)

	s.d.mu.Lock()
	defer s.d.mu.Unlock()
	keys := decodeKeys(s.d.content[systemKeysCollection])
	k := SystemKey{
		ID:        s.d.nextID,
		Name:      name,
		Prefix:    prefix,
		Hash:      hash,
		Role:      role,
		Dataset:   dataset,
		CreatedAt: now,
	}
	s.d.nextID++
	keys = append(keys, k)
	s.d.content[systemKeysCollection] = encodeKeys(keys)
	s.d.persistLocked()
	return k, plaintext, nil
}

// RevokeKey marks the key with the given id as revoked, reporting whether one
// was found.
func (s *SystemStore) RevokeKey(id int) bool {
	s.d.mu.Lock()
	defer s.d.mu.Unlock()
	keys := decodeKeys(s.d.content[systemKeysCollection])
	for i, k := range keys {
		if k.ID == id {
			if k.RevokedAt != "" {
				return true // already revoked
			}
			keys[i].RevokedAt = nowUTC().Format(time.RFC3339)
			s.d.content[systemKeysCollection] = encodeKeys(keys)
			s.d.persistLocked()
			return true
		}
	}
	return false
}

// HasAdmin reports whether any active (non-revoked) credential grants the
// admin role. Used by the startup bootstrap guard.
func (s *SystemStore) HasAdmin() bool {
	for _, u := range s.Users() {
		if u.Role == "admin" && u.Active() {
			return true
		}
	}
	for _, k := range s.Keys() {
		if k.Role == "admin" && !k.Revoked() {
			return true
		}
	}
	return false
}

// --- Hashing -------------------------------------------------------------

// HashSecret returns a salted SHA-256 hash of secret, encoded as
// "salthex$hashhex". A fresh 16-byte random salt is used per call. SHA-256 is
// sufficient for the high-entropy random API keys ProtoCMS issues and keeps
// the project dependency-free.
func HashSecret(secret string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	sum := sha256.Sum256(append(salt, []byte(secret)...))
	return hex.EncodeToString(salt) + "$" + hex.EncodeToString(sum[:]), nil
}

// VerifySecret reports whether secret matches the stored salted hash, using a
// constant-time comparison.
func VerifySecret(stored, secret string) bool {
	salthex, hashhex, ok := strings.Cut(stored, "$")
	if !ok {
		return false
	}
	salt, err := hex.DecodeString(salthex)
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(hashhex)
	if err != nil {
		return false
	}
	sum := sha256.Sum256(append(salt, []byte(secret)...))
	return subtle.ConstantTimeCompare(sum[:], want) == 1
}

// generateAPIKey returns a new high-entropy key string ("pck_" + 32 hex chars).
func generateAPIKey() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "pck_" + hex.EncodeToString(b), nil
}

// --- Collection (de)serialization ---------------------------------------
//
// Records live in the dataset content map as []ContentItem (map[string]any),
// so they round-trip through the same JSON persistence as content. These
// helpers convert between the typed structs and that representation.

func encodeUsers(users []SystemUser) []ContentItem {
	out := make([]ContentItem, 0, len(users))
	for _, u := range users {
		out = append(out, ContentItem{
			"id":            u.ID,
			"username":      u.Username,
			"password_hash": u.PasswordHash,
			"role":          u.Role,
			"dataset":       u.Dataset,
			"status":        u.Status,
			"created_at":    u.CreatedAt,
			"updated_at":    u.UpdatedAt,
		})
	}
	return out
}

func decodeUsers(items []ContentItem) []SystemUser {
	out := make([]SystemUser, 0, len(items))
	for _, it := range items {
		out = append(out, SystemUser{
			ID:           toInt(it["id"]),
			Username:     toStr(it["username"]),
			PasswordHash: toStr(it["password_hash"]),
			Role:         toStr(it["role"]),
			Dataset:      toStr(it["dataset"]),
			Status:       toStr(it["status"]),
			CreatedAt:    toStr(it["created_at"]),
			UpdatedAt:    toStr(it["updated_at"]),
		})
	}
	return out
}

func encodeKeys(keys []SystemKey) []ContentItem {
	out := make([]ContentItem, 0, len(keys))
	for _, k := range keys {
		out = append(out, ContentItem{
			"id":           k.ID,
			"name":         k.Name,
			"prefix":       k.Prefix,
			"hash":         k.Hash,
			"role":         k.Role,
			"dataset":      k.Dataset,
			"created_at":   k.CreatedAt,
			"last_used_at": k.LastUsedAt,
			"revoked_at":   k.RevokedAt,
		})
	}
	return out
}

func decodeKeys(items []ContentItem) []SystemKey {
	out := make([]SystemKey, 0, len(items))
	for _, it := range items {
		out = append(out, SystemKey{
			ID:         toInt(it["id"]),
			Name:       toStr(it["name"]),
			Prefix:     toStr(it["prefix"]),
			Hash:       toStr(it["hash"]),
			Role:       toStr(it["role"]),
			Dataset:    toStr(it["dataset"]),
			CreatedAt:  toStr(it["created_at"]),
			LastUsedAt: toStr(it["last_used_at"]),
			RevokedAt:  toStr(it["revoked_at"]),
		})
	}
	return out
}

// toStr coerces a content value to a string, tolerating the absence of a key.
func toStr(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// toInt coerces a content value to an int. After a JSON round-trip numbers
// arrive as float64; freshly-created records hold a Go int.
func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}
