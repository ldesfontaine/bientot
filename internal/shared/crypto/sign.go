// Package crypto implements Ed25519 signing and verification for PushRequest.
// It uses deterministic protobuf encoding as the canonical form to hash.
package crypto

import (
	"crypto/ed25519"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	bientotv1 "github.com/ldesfontaine/bientot/api/v1/gen/v1"
)

func canonicalBytes(req *bientotv1.PushRequest) ([]byte, error) {
	clone, ok := proto.Clone(req).(*bientotv1.PushRequest)
	if !ok {
		return nil, errors.New("proto.Clone returned unexpected type")
	}

	clone.Signature = nil

	opts := proto.MarshalOptions{Deterministic: true}
	data, err := opts.Marshal(clone)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical: %w", err)
	}

	return data, nil
}

// Sign computes an Ed25519 signature over the canonical encoding of req
// (with Signature cleared), populates req.Signature, and returns req.
//
// The input req is mutated in place for simplicity — callers who need to
// preserve the original should Clone it first.
func Sign(req *bientotv1.PushRequest, privKey ed25519.PrivateKey) (*bientotv1.PushRequest, error) {
	if req == nil {
		return nil, errors.New("sign: nil request")
	}
	if len(privKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("sign: invalid private key size %d, want %d", len(privKey), ed25519.PrivateKeySize)
	}

	canonical, err := canonicalBytes(req)
	if err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}

	sig := ed25519.Sign(privKey, canonical)
	req.Signature = sig

	return req, nil
}

// Verify checks that req.Signature is a valid Ed25519 signature of req
// with Signature cleared. Returns nil if valid, an error otherwise.
func Verify(req *bientotv1.PushRequest, pubKey ed25519.PublicKey) error {
	if req == nil {
		return errors.New("verify: nil request")
	}
	if len(pubKey) != ed25519.PublicKeySize {
		return fmt.Errorf("verify: invalid public key size %d, want %d", len(pubKey), ed25519.PublicKeySize)
	}
	if len(req.Signature) != ed25519.SignatureSize {
		return fmt.Errorf("verify: invalid signature size %d, want %d", len(req.Signature), ed25519.SignatureSize)
	}

	canonical, err := canonicalBytes(req)
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}

	if !ed25519.Verify(pubKey, canonical, req.Signature) {
		return errors.New("verify: signature mismatch")
	}

	return nil
}
