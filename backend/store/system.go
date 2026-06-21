package store

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
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
// so they round-trip through the same JSON persistence as content. The typed
// structs carry json tags, so a single JSON round-trip converts between them
// and that representation — no per-field, per-type mapping to keep in sync.

func encodeUsers(users []SystemUser) []ContentItem { return encodeRecords(users) }
func decodeUsers(items []ContentItem) []SystemUser { return decodeRecords[SystemUser](items) }
func encodeKeys(keys []SystemKey) []ContentItem    { return encodeRecords(keys) }
func decodeKeys(items []ContentItem) []SystemKey   { return decodeRecords[SystemKey](items) }

// encodeRecords marshals typed records into ContentItem maps via their json
// tags. A record that fails to marshal is skipped (it cannot happen for the
// plain string/int structs used here).
func encodeRecords[T any](records []T) []ContentItem {
	out := make([]ContentItem, 0, len(records))
	for _, r := range records {
		b, err := json.Marshal(r)
		if err != nil {
			continue
		}
		var item ContentItem
		if err := json.Unmarshal(b, &item); err != nil {
			continue
		}
		out = append(out, item)
	}
	return out
}

// decodeRecords unmarshals ContentItem maps back into typed records. Numbers
// that arrived as float64 from a JSON load are coerced into the struct's
// declared field types by the stdlib decoder. A malformed item is skipped.
func decodeRecords[T any](items []ContentItem) []T {
	out := make([]T, 0, len(items))
	for _, it := range items {
		b, err := json.Marshal(it)
		if err != nil {
			continue
		}
		var r T
		if err := json.Unmarshal(b, &r); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out
}
