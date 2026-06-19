package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/remisb/muxstack/middleware"
)

// jwtTTL is the lifetime of tokens issued by /api/login.
const jwtTTL = time.Hour

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

// protect wraps a writing handler with authentication and a role check.
func protectHandler(h http.HandlerFunc, authMiddleware middleware.Middleware, roles ...string) http.HandlerFunc {
	return middleware.Chain(h, authMiddleware, middleware.Authorizer(roles...)).ServeHTTP
}

//// loginHandler authenticates a username/password against PROTOCMS_USERS and
//// issues a JWT. Registered as POST /api/login (public).
//func loginHandler(cfg auth.AuthConfig) http.HandlerFunc {
//	return func(w http.ResponseWriter, r *http.Request) {
//		if cfg.jwtSecret == nil {
//			http.Error(w, `{"error":"login disabled"}`, http.StatusServiceUnavailable)
//			return
//		}
//
//		var creds struct {
//			Username string `json:"username"`
//			Password string `json:"password"`
//		}
//		if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
//			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
//			return
//		}
//
//		user, ok := cfg.users[creds.Username]
//		if !ok || user.password != creds.Password {
//			http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
//			return
//		}
//
//		token, err := signJWT(cfg.jwtSecret, creds.Username, user.role, jwtTTL)
//		if err != nil {
//			http.Error(w, fmt.Sprintf(`{"error":"failed to sign token: %v"}`, err), http.StatusInternalServerError)
//			return
//		}
//
//		jsonResponse(w, http.StatusOK, map[string]any{
//			"token":      token,
//			"role":       user.role,
//			"expires_in": int(jwtTTL.Seconds()),
//		})
//	}
//}
