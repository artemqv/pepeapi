package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

func GenerateSessionToken() (string, error) {
	const sessionTokenLength = 32
	sessionTokenBytes := make([]byte, sessionTokenLength)
	_, err := rand.Read(sessionTokenBytes)
	if err != nil {
		return "", err
	}

	sessionToken := base64.RawURLEncoding.EncodeToString(sessionTokenBytes)
	return sessionToken, nil
}
func HashSessionToken(token string) [32]byte {
	tokenBytes := []byte(token)
	hash := sha256.Sum256(tokenBytes)
	return hash
}
