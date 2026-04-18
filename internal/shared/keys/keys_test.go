package keys

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func writePrivateKey(t *testing.T, path string, priv ed25519.PrivateKey) {
	t.Helper()
	b, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal PKCS#8: %v", err)
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: b}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func writePublicKey(t *testing.T, path string, pub ed25519.PublicKey) {
	t.Helper()
	b, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal PKIX: %v", err)
	}
	block := &pem.Block{Type: "PUBLIC KEY", Bytes: b}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestLoadPrivateKey_Roundtrip(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.key")

	writePrivateKey(t, path, priv)

	loaded, err := LoadPrivateKey(path)
	if err != nil {
		t.Fatalf("LoadPrivateKey: %v", err)
	}

	if !priv.Equal(loaded) {
		t.Error("loaded key does not match original")
	}
}

func TestLoadPublicKey_Roundtrip(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.pub")

	writePublicKey(t, path, pub)

	loaded, err := LoadPublicKey(path)
	if err != nil {
		t.Fatalf("LoadPublicKey: %v", err)
	}

	if !pub.Equal(loaded) {
		t.Error("loaded key does not match original")
	}
}

func TestLoadPublicKeysDir(t *testing.T) {
	dir := t.TempDir()

	pub1, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key 1: %v", err)
	}
	pub2, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key 2: %v", err)
	}
	writePublicKey(t, filepath.Join(dir, "vps.pub"), pub1)
	writePublicKey(t, filepath.Join(dir, "pi.pub"), pub2)

	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write notes.txt: %v", err)
	}

	keys, err := LoadPublicKeysDir(dir)
	if err != nil {
		t.Fatalf("LoadPublicKeysDir: %v", err)
	}

	if len(keys) != 2 {
		t.Errorf("got %d keys, want 2", len(keys))
	}
	if !keys["vps"].Equal(pub1) {
		t.Error("vps key mismatch")
	}
	if !keys["pi"].Equal(pub2) {
		t.Error("pi key mismatch")
	}
	if _, exists := keys["notes"]; exists {
		t.Error("notes.txt should not be loaded")
	}
}

func TestLoadPrivateKey_MissingFile(t *testing.T) {
	_, err := LoadPrivateKey("/nonexistent/path.key")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadPrivateKey_MalformedPEM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.key")
	if err := os.WriteFile(path, []byte("not a pem file"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := LoadPrivateKey(path)
	if err == nil {
		t.Fatal("expected error for malformed PEM, got nil")
	}
}
