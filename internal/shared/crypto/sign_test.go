package crypto

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"google.golang.org/protobuf/proto"

	bientotv1 "github.com/ldesfontaine/bientot/api/v1/gen/v1"
)

func TestSignVerify_Roundtrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	req := &bientotv1.PushRequest{
		V:           1,
		MachineId:   "vps",
		TimestampNs: 1234567890,
		Nonce:       "test-nonce",
		Modules: []*bientotv1.ModuleData{
			{Module: "heartbeat", TimestampNs: 1234567890},
		},
	}

	signed, err := Sign(req, priv)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	if len(signed.Signature) != ed25519.SignatureSize {
		t.Errorf("signature size = %d, want %d", len(signed.Signature), ed25519.SignatureSize)
	}

	if err := Verify(signed, pub); err != nil {
		t.Errorf("Verify after Sign: %v", err)
	}
}

func TestVerify_TamperedMessage(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	req := &bientotv1.PushRequest{
		V:           1,
		MachineId:   "vps",
		TimestampNs: 1234567890,
		Nonce:       "test-nonce",
	}

	signed, err := Sign(req, priv)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	signed.MachineId = "attacker"

	if err := Verify(signed, pub); err == nil {
		t.Fatal("Verify expected error on tampered message, got nil")
	}
}

func TestVerify_WrongKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	wrongPub, _, _ := ed25519.GenerateKey(rand.Reader)

	req := &bientotv1.PushRequest{V: 1, MachineId: "vps"}
	signed, _ := Sign(req, priv)

	if err := Verify(signed, wrongPub); err == nil {
		t.Fatal("Verify expected error with wrong key, got nil")
	}
}

func TestSign_Deterministic(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)

	req1 := &bientotv1.PushRequest{
		V:           1,
		MachineId:   "vps",
		TimestampNs: 1234567890,
		Nonce:       "test-nonce",
	}
	req2 := proto.Clone(req1).(*bientotv1.PushRequest)

	signed1, _ := Sign(req1, priv)
	signed2, _ := Sign(req2, priv)

	if !bytes.Equal(signed1.Signature, signed2.Signature) {
		t.Errorf("signatures differ: %x vs %x", signed1.Signature, signed2.Signature)
	}
}

func TestSign_InvalidKey(t *testing.T) {
	req := &bientotv1.PushRequest{V: 1}
	_, err := Sign(req, ed25519.PrivateKey{0x01, 0x02})
	if err == nil {
		t.Fatal("Sign expected error on short key, got nil")
	}
}
