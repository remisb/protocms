package auth

import (
	"encoding/base64"
	"encoding/json"
	"time"
)

// SignJWT produces an HS256 JWT carrying the subject and role.
func SignJWT(authCfg Config, userName string, userCred UserCred, ttl time.Duration) (string, error) {
	now := time.Now()
	payload, err := json.Marshal(jwtPayload{
		Sub:     userName,
		Role:    userCred.role,
		Dataset: userCred.dataset,
		Iat:     now.Unix(),
		Exp:     now.Add(ttl).Unix(),
	})
	if err != nil {
		return "", err
	}
	signingInput := jwtHeader + "." + base64.RawURLEncoding.EncodeToString(payload)
	sig := hmacSHA256(authCfg.jwtSecret, signingInput)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}
