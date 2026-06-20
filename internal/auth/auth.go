package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/remisb/muxstack/middleware"
)

// validRoles is the hard-coded set of roles ProtoCMS understands.
var validRoles = map[string]bool{"admin": true, "editor": true}

// defaultDataset is the dataset bound to a credential when its config entry
// omits the optional dataset segment.
const defaultDataset = "default"

// jwtHeader is the fixed HS256 header, pre-encoded.
var jwtHeader = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

var ErrInvalidCredentials = errors.New("invalid credentials")

type jwtPayload struct {
	Sub     string `json:"sub"`
	Role    string `json:"role"`
	Dataset string `json:"ds,omitempty"`
	Iat     int64  `json:"iat"`
	Exp     int64  `json:"exp"`
}

// keyCred is the role + dataset bound to a static API key.
type keyCred struct {
	role    string
	dataset string
}

// Config holds the credential material loaded from the environment.
type Config struct {
	apiKeys   map[string]keyCred // token -> role + dataset
	users     map[string]UserCred
	jwtSecret []byte // nil if JWT auth is disabled
}

func (c Config) HasUsers() bool { return len(c.users) > 0 }

func (c Config) HasAPIKeys() bool { return len(c.apiKeys) > 0 }

func (c Config) ValidatePassword(userName, password string) error {
	_, err := c.GetUser(userName, password)
	if err != nil && errors.Is(err, ErrInvalidCredentials) {
		return ErrInvalidCredentials
	}
	return nil
}

func (c Config) IsJWTAuthDisabled() bool {
	return c.jwtSecret == nil
}

func (c Config) GetUser(userName string, password string) (UserCred, error) {
	user, ok := c.users[userName]
	if !ok || user.password != password {
		return UserCred{}, ErrInvalidCredentials
	}
	return user, nil
}

type UserCred struct {
	password string
	role     string
	dataset  string
}

func (c UserCred) Role() string { return c.role }

func (c UserCred) Dataset() string { return c.dataset }

// LoadConfig reads PROTOCMS_API_KEYS, PROTOCMS_JWT_SECRET and
// PROTOCMS_USERS from the environment. Misconfigured roles cause a hard exit,
// so the operator finds out at startup rather than at request time.
func LoadConfig() Config {
	cfg := Config{
		apiKeys: make(map[string]keyCred),
		users:   make(map[string]UserCred),
	}

	// PROTOCMS_API_KEYS: "key:role[:dataset],..." (dataset optional, defaults
	// to "default").
	for _, entry := range splitList(os.Getenv("PROTOCMS_API_KEYS")) {
		parts := strings.SplitN(entry, ":", 3)
		if len(parts) < 2 || parts[0] == "" {
			slog.Error("invalid PROTOCMS_API_KEYS entry (want key:role[:dataset])", "entry", entry)
			os.Exit(1)
		}
		key, role := parts[0], parts[1]
		ds := defaultDataset
		if len(parts) == 3 && parts[2] != "" {
			ds = parts[2]
		}
		if !validRoles[role] {
			slog.Error("unknown role in PROTOCMS_API_KEYS", "role", role, "valid", "admin,editor")
			os.Exit(1)
		}
		cfg.apiKeys[key] = keyCred{role: role, dataset: ds}
	}

	// PROTOCMS_USERS: "user:pass:role[:dataset],..." (dataset optional,
	// defaults to "default").
	for _, entry := range splitList(os.Getenv("PROTOCMS_USERS")) {
		parts := strings.SplitN(entry, ":", 4)
		if len(parts) < 3 || parts[0] == "" {
			slog.Error("invalid PROTOCMS_USERS entry (want user:pass:role[:dataset])", "entry", entry)
			os.Exit(1)
		}
		ds := defaultDataset
		if len(parts) == 4 && parts[3] != "" {
			ds = parts[3]
		}
		if !validRoles[parts[2]] {
			slog.Error("unknown role in PROTOCMS_USERS", "role", parts[2], "valid", "admin,editor")
			os.Exit(1)
		}
		cfg.users[parts[0]] = UserCred{password: parts[1], role: parts[2], dataset: ds}
	}

	if secret := os.Getenv("PROTOCMS_JWT_SECRET"); secret != "" {
		cfg.jwtSecret = []byte(secret)
	}

	if len(cfg.apiKeys) == 0 && cfg.jwtSecret == nil {
		slog.Warn("no PROTOCMS_API_KEYS and no PROTOCMS_JWT_SECRET set: " +
			"writes are locked because no valid credential can be presented")
	}
	if len(cfg.users) > 0 && cfg.jwtSecret == nil {
		slog.Warn("PROTOCMS_USERS set but PROTOCMS_JWT_SECRET empty: /api/login is disabled")
	}

	return cfg
}

// NewVerifier returns a middleware.TokenVerifier that accepts either a static
// API key or an HS256 JWT signed with the configured secret.
func NewVerifier(cfg Config) middleware.TokenVerifier {
	return func(_ context.Context, token string) (*middleware.Claims, error) {
		if kc, ok := cfg.apiKeys[token]; ok {
			return &middleware.Claims{Subject: "apikey", Roles: []string{kc.role}}, nil
		}
		if cfg.jwtSecret != nil {
			sub, role, _, err := verifyJWT(cfg.jwtSecret, token)
			if err == nil {
				return &middleware.Claims{Subject: sub, Roles: []string{role}}, nil
			}
		}
		return nil, errors.New("invalid token")
	}
}

// splitList splits a comma-separated env value, trimming whitespace and
// dropping empty fields.
func splitList(s string) []string {
	var out []string
	for p := range strings.SplitSeq(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// verifyJWT validates an HS256 JWT's signature and expiry, returning the
// subject, role, and bound dataset on success. A token without a ds claim
// resolves to the default dataset.
func verifyJWT(secret []byte, token string) (sub, role, dataset string, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", "", "", errors.New("malformed token")
	}
	if parts[0] != jwtHeader {
		return "", "", "", errors.New("unexpected token header")
	}

	expectedSig := hmacSHA256(secret, parts[0]+"."+parts[1])
	gotSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", "", "", errors.New("bad signature encoding")
	}
	if !hmac.Equal(expectedSig, gotSig) {
		return "", "", "", errors.New("signature mismatch")
	}

	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", "", errors.New("bad payload encoding")
	}
	var p jwtPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", "", "", errors.New("bad payload json")
	}
	if p.Exp <= time.Now().Unix() {
		return "", "", "", errors.New("token expired")
	}
	ds := p.Dataset
	if ds == "" {
		ds = defaultDataset
	}
	return p.Sub, p.Role, ds, nil
}

// ResolveDataset returns the dataset bound to a valid token (API key or JWT),
// or ("", false) if the token is invalid. Used by the dataset-resolution
// middleware after authentication has already succeeded.
func (c Config) ResolveDataset(token string) (string, bool) {
	if kc, ok := c.apiKeys[token]; ok {
		return kc.dataset, true
	}
	if c.jwtSecret != nil {
		if _, _, ds, err := verifyJWT(c.jwtSecret, token); err == nil {
			return ds, true
		}
	}
	return "", false
}

func hmacSHA256(secret []byte, data string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}
