package auth

import "sync"

// This file layers credentials sourced from the _system dataset onto the
// env-derived Config, with env taking precedence ("env overrides _system").
//
// The auth package stays free of a store dependency: the wiring layer reads
// _system records and hands them in as the neutral types below. Lookups
// consult env first, then the system layer, so an env entry always shadows a
// system entry for the same token/username.

// SystemKeyCred is a hashed API key sourced from the _system dataset. The
// plaintext is never held; Verify checks a presented token against Hash.
// Prefix is the key's non-secret leading characters, used to index candidates
// so token resolution does not hash every stored key.
type SystemKeyCred struct {
	Prefix  string // non-secret leading chars of the key (e.g. "pck_dead")
	Hash    string // salted hash, verified via Verify
	Role    string
	Dataset string
	Verify  func(stored, secret string) bool // injected hash comparator
}

// SystemUserCred is a user sourced from the _system dataset, with a hashed
// password.
type SystemUserCred struct {
	PasswordHash string
	Role         string
	Dataset      string
	Verify       func(stored, secret string) bool
}

// systemLayer holds the credentials merged in from _system. It is replaced
// wholesale on every _system write (see SetSystemCredentials), so reads take a
// snapshot under the lock.
//
// keysByPrefix indexes the keys by their non-secret prefix so resolving a
// presented token hashes only the (usually one) candidate sharing its prefix,
// not every stored key. prefixLens is the distinct set of prefix lengths
// present, so a lookup knows how to slice the token.
type systemLayer struct {
	mu           sync.RWMutex
	keysByPrefix map[string][]SystemKeyCred
	prefixLens   []int
	users        map[string]SystemUserCred
}

// SetSystemCredentials replaces the _system credential layer. Call it at
// startup and after any _system credential mutation so changes take effect
// in-process without a restart.
func (c *Config) SetSystemCredentials(keys []SystemKeyCred, users map[string]SystemUserCred) {
	if c.system == nil {
		c.system = &systemLayer{}
	}
	byPrefix := make(map[string][]SystemKeyCred, len(keys))
	lenSet := make(map[int]bool)
	for _, k := range keys {
		byPrefix[k.Prefix] = append(byPrefix[k.Prefix], k)
		lenSet[len(k.Prefix)] = true
	}
	lens := make([]int, 0, len(lenSet))
	for l := range lenSet {
		lens = append(lens, l)
	}

	c.system.mu.Lock()
	c.system.keysByPrefix = byPrefix
	c.system.prefixLens = lens
	c.system.users = users
	c.system.mu.Unlock()
}

// resolveSystemKey returns the role+dataset for a presented token matching a
// non-revoked _system key, or ("", "", false). Revoked keys are expected to be
// excluded by the wiring layer before SetSystemCredentials is called.
func (c Config) resolveSystemKey(token string) (role, dataset string, ok bool) {
	if c.system == nil {
		return "", "", false
	}
	c.system.mu.RLock()
	defer c.system.mu.RUnlock()
	// Only candidates sharing the token's prefix can match, so hash just those
	// rather than every stored key.
	for _, l := range c.system.prefixLens {
		if l > len(token) {
			continue
		}
		for _, k := range c.system.keysByPrefix[token[:l]] {
			if k.Verify != nil && k.Verify(k.Hash, token) {
				return k.Role, k.Dataset, true
			}
		}
	}
	return "", "", false
}

// systemUser returns the _system user for username, or (zero, false).
func (c Config) systemUser(username string) (SystemUserCred, bool) {
	if c.system == nil {
		return SystemUserCred{}, false
	}
	c.system.mu.RLock()
	defer c.system.mu.RUnlock()
	u, ok := c.system.users[username]
	return u, ok
}
