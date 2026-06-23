package store

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
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

// --- Users ---------------------------------------------------------------

// Users returns all user records.
func (s *SystemStore) Users() []SystemUser {
	return decodeUsers(s.d.CollectionItems(systemUsersCollection))
}

// UserByName returns the user with the given username, or ErrNotFound.
func (s *SystemStore) UserByName(username string) (SystemUser, error) {
	for _, u := range s.Users() {
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

	var created SystemUser
	_, err = s.d.AppendCollectionItem(
		systemUsersCollection,
		func(items []ContentItem) error {
			for _, u := range decodeUsers(items) {
				if u.Username == username {
					return ErrUserExists
				}
			}
			return nil
		},
		func(id int) ContentItem {
			created = SystemUser{
				ID:           id,
				Username:     username,
				PasswordHash: hash,
				Role:         role,
				Dataset:      dataset,
				Status:       UserStatusActive,
				CreatedAt:    now,
				UpdatedAt:    now,
			}
			return encodeRecord(created)
		},
	)
	if err != nil {
		return SystemUser{}, err
	}
	return created, nil
}

// DeleteUser removes the user with the given id, reporting whether one was
// found.
func (s *SystemStore) DeleteUser(id int) bool {
	return s.d.MutateCollection(systemUsersCollection, func(items []ContentItem) ([]ContentItem, bool) {
		users := decodeUsers(items)
		for i, u := range users {
			if u.ID == id {
				return encodeUsers(append(users[:i], users[i+1:]...)), true
			}
		}
		return items, false
	})
}

// --- API keys ------------------------------------------------------------

// Keys returns all API key records (including revoked ones).
func (s *SystemStore) Keys() []SystemKey {
	return decodeKeys(s.d.CollectionItems(systemKeysCollection))
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

	var created SystemKey
	if _, err := s.d.AppendCollectionItem(systemKeysCollection, nil, func(id int) ContentItem {
		created = SystemKey{
			ID:        id,
			Name:      name,
			Prefix:    prefix,
			Hash:      hash,
			Role:      role,
			Dataset:   dataset,
			CreatedAt: now,
		}
		return encodeRecord(created)
	}); err != nil {
		return SystemKey{}, "", err
	}
	return created, plaintext, nil
}

// RevokeKey marks the key with the given id as revoked, reporting whether one
// was found. An already-revoked key is treated as found (idempotent) without a
// redundant write.
func (s *SystemStore) RevokeKey(id int) bool {
	found := false
	s.d.MutateCollection(systemKeysCollection, func(items []ContentItem) ([]ContentItem, bool) {
		keys := decodeKeys(items)
		for i, k := range keys {
			if k.ID == id {
				found = true
				if k.RevokedAt != "" {
					return items, false // already revoked: no write
				}
				keys[i].RevokedAt = nowUTC().Format(time.RFC3339)
				return encodeKeys(keys), true
			}
		}
		return items, false
	})
	return found
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
func decodeUsers(items []ContentItem) []SystemUser {
	return decodeRecords[SystemUser](items, systemUsersCollection)
}
func encodeKeys(keys []SystemKey) []ContentItem { return encodeRecords(keys) }
func decodeKeys(items []ContentItem) []SystemKey {
	return decodeRecords[SystemKey](items, systemKeysCollection)
}

// encodeRecord marshals a single typed record into a ContentItem map via its
// json tags. It returns an empty item if the record cannot be marshaled (it
// cannot happen for the plain string/int structs used here).
func encodeRecord[T any](record T) ContentItem {
	b, err := json.Marshal(record)
	if err != nil {
		return ContentItem{}
	}
	var item ContentItem
	if err := json.Unmarshal(b, &item); err != nil {
		return ContentItem{}
	}
	return item
}

// encodeRecords marshals typed records into ContentItem maps via their json
// tags. A record that fails to marshal is skipped (it cannot happen for the
// plain string/int structs used here).
func encodeRecords[T any](records []T) []ContentItem {
	out := make([]ContentItem, 0, len(records))
	for _, r := range records {
		out = append(out, encodeRecord(r))
	}
	return out
}

// decodeRecords unmarshals ContentItem maps back into typed records. Numbers
// that arrived as float64 from a JSON load are coerced into the struct's
// declared field types by the stdlib decoder. A malformed item is skipped and
// logged with its collection so an operator can spot a corrupted or
// hand-edited record (records written by encodeRecord always decode cleanly,
// so a skip means the on-disk data diverged from the schema).
func decodeRecords[T any](items []ContentItem, collection string) []T {
	out := make([]T, 0, len(items))
	for i, it := range items {
		b, err := json.Marshal(it)
		if err != nil {
			slog.Warn("skipping unencodable system record",
				"collection", collection, "index", i, "err", err)
			continue
		}
		var r T
		if err := json.Unmarshal(b, &r); err != nil {
			slog.Warn("skipping malformed system record",
				"collection", collection, "index", i, "err", err)
			continue
		}
		out = append(out, r)
	}
	return out
}
