package transport

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Sign computes HMAC-SHA256 over the JSON-encoded body using the shared secret.
func Sign(body Body, secret string) (string, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal body for signing: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// Verify checks that the signature matches the HMAC-SHA256 of the body.
func Verify(body Body, secret, signature string) error {
	expected, err := Sign(body, secret)
	if err != nil {
		return fmt.Errorf("computing expected signature: %w", err)
	}

	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}
