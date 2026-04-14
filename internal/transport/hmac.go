package transport

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Sign calcule le HMAC-SHA256 sur le body encodé en JSON avec le secret partagé.
func Sign(body Body, secret string) (string, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("sérialisation du body pour signature: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// Verify vérifie que la signature correspond au HMAC-SHA256 du body.
func Verify(body Body, secret, signature string) error {
	expected, err := Sign(body, secret)
	if err != nil {
		return fmt.Errorf("calcul de la signature attendue: %w", err)
	}

	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return fmt.Errorf("signature invalide")
	}
	return nil
}
