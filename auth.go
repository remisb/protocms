package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/remisb/muxstack/middleware"
)

// validRoles is the hard-coded set of roles ProtoCMS understands.
var validRoles = map[string]bool{"admin": true, "editor": true}

// authConfig holds the credential material loaded from the environment.
type authConfig struct {
	apiKeys   map[string]string // token -> role
	users     map[string]userCred
	jwtSecret []byte // nil if JWT auth is disabled
}

type userCred struct {
	password string
	role     string
}

// jwtTTL is the lifetime of tokens issued by /api/login.
const jwtTTL = time.Hour

// loadAuthConfig reads PROTOCMS_API_KEYS, PROTOCMS_JWT_SECRET and
// PROTOCMS_USERS from the environment. Misconfigured roles cause a hard exit
// so the operator finds out at startup rather than at request time.
func loadAuthConfig() authConfig {
	cfg := authConfig{
		apiKeys: make(map[string]string),
		users:   make(map[string]userCred),
	}

	// PROTOCMS_API_KEYS: "key:role,key2:role2"
	for _, entry := range splitList(os.Getenv("PROTOCMS_API_KEYS")) {
		key, role, ok := strings.Cut(entry, ":")
		if !ok || key == "" {
			slog.Error("invalid PROTOCMS_API_KEYS entry (want key:role)", "entry", entry)
			os.Exit(1)
		}
		if !validRoles[role] {
			slog.Error("unknown role in PROTOCMS_API_KEYS", "role", role, "valid", "admin,editor")
			os.Exit(1)
		}
		cfg.apiKeys[key] = role
	}

	// PROTOCMS_USERS: "user:pass:role,user2:pass2:role2"
	for _, entry := range splitList(os.Getenv("PROTOCMS_USERS")) {
		parts := strings.SplitN(entry, ":", 3)
		if len(parts) != 3 || parts[0] == "" {
			slog.Error("invalid PROTOCMS_USERS entry (want user:pass:role)", "entry", entry)
			os.Exit(1)
		}
		if !validRoles[parts[2]] {
			slog.Error("unknown role in PROTOCMS_USERS", "role", parts[2], "valid", "admin,editor")
			os.Exit(1)
		}
		cfg.users[parts[0]] = userCred{password: parts[1], role: parts[2]}
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

// newVerifier returns a middleware.TokenVerifier that accepts either a static
// API key or an HS256 JWT signed with the configured secret.
func newVerifier(cfg authConfig) middleware.TokenVerifier {
	return func(_ context.Context, token string) (*middleware.Claims, error) {
		if role, ok := cfg.apiKeys[token]; ok {
			return &middleware.Claims{Subject: "apikey", Roles: []string{role}}, nil
		}
		if cfg.jwtSecret != nil {
			sub, role, err := verifyJWT(cfg.jwtSecret, token)
			if err == nil {
				return &middleware.Claims{Subject: sub, Roles: []string{role}}, nil
			}
		}
		return nil, errors.New("invalid token")
	}
}

// jwtHeader is the fixed HS256 header, pre-encoded.
var jwtHeader = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

type jwtPayload struct {
	Sub  string `json:"sub"`
	Role string `json:"role"`
	Iat  int64  `json:"iat"`
	Exp  int64  `json:"exp"`
}

// signJWT produces an HS256 JWT carrying the subject and role.
func signJWT(secret []byte, sub, role string, ttl time.Duration) (string, error) {
	now := time.Now()
	payload, err := json.Marshal(jwtPayload{
		Sub:  sub,
		Role: role,
		Iat:  now.Unix(),
		Exp:  now.Add(ttl).Unix(),
	})
	if err != nil {
		return "", err
	}
	signingInput := jwtHeader + "." + base64.RawURLEncoding.EncodeToString(payload)
	sig := hmacSHA256(secret, signingInput)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// verifyJWT validates an HS256 JWT's signature and expiry, returning the
// subject and role on success.
func verifyJWT(secret []byte, token string) (sub, role string, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", "", errors.New("malformed token")
	}
	if parts[0] != jwtHeader {
		return "", "", errors.New("unexpected token header")
	}

	expectedSig := hmacSHA256(secret, parts[0]+"."+parts[1])
	gotSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", "", errors.New("bad signature encoding")
	}
	if !hmac.Equal(expectedSig, gotSig) {
		return "", "", errors.New("signature mismatch")
	}

	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", errors.New("bad payload encoding")
	}
	var p jwtPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", "", errors.New("bad payload json")
	}
	if p.Exp <= time.Now().Unix() {
		return "", "", errors.New("token expired")
	}
	return p.Sub, p.Role, nil
}

func hmacSHA256(secret []byte, data string) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

// loginHandler authenticates a username/password against PROTOCMS_USERS and
// issues a JWT. Registered as POST /api/login (public).
func loginHandler(cfg authConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.jwtSecret == nil {
			http.Error(w, `{"error":"login disabled"}`, http.StatusServiceUnavailable)
			return
		}

		var creds struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}

		user, ok := cfg.users[creds.Username]
		if !ok || user.password != creds.Password {
			http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
			return
		}

		token, err := signJWT(cfg.jwtSecret, creds.Username, user.role, jwtTTL)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to sign token: %v"}`, err), http.StatusInternalServerError)
			return
		}

		jsonResponse(w, http.StatusOK, map[string]any{
			"token":      token,
			"role":       user.role,
			"expires_in": int(jwtTTL.Seconds()),
		})
	}
}
