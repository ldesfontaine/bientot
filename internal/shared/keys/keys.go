// Package keys handles loading Ed25519 signing keys from PEM files on disk.
package keys

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadPrivateKey reads a PEM-encoded PKCS#8 Ed25519 private key from path.
// Returns an error if the file is missing, malformed, or contains a non-Ed25519 key.
func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key %s: %w", path, err)
	}

	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in %s", path)
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS#8 private key from %s: %w", path, err)
	}

	ed25519Key, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key in %s is not Ed25519 (got %T)", path, key)
	}

	return ed25519Key, nil
}

// LoadPublicKey reads a PEM-encoded PKIX Ed25519 public key from path.
func LoadPublicKey(path string) (ed25519.PublicKey, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read public key %s: %w", path, err)
	}

	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in %s", path)
	}

	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key from %s: %w", path, err)
	}

	ed25519Key, ok := key.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("key in %s is not Ed25519 (got %T)", path, key)
	}

	return ed25519Key, nil
}

// LoadPublicKeysDir reads all files matching *.pub in dir and returns
// a map from the basename (without .pub extension) to the Ed25519 public key.
//
// Fails fast on the first malformed key: a dashboard booting with a corrupted
// public key must crash visibly rather than silently ignoring an agent.
func LoadPublicKeysDir(dir string) (map[string]ed25519.PublicKey, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}

	keys := make(map[string]ed25519.PublicKey)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".pub") {
			continue
		}

		machineID := strings.TrimSuffix(name, ".pub")
		fullPath := filepath.Join(dir, name)

		pub, err := LoadPublicKey(fullPath)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", machineID, err)
		}

		keys[machineID] = pub
	}

	return keys, nil
}
