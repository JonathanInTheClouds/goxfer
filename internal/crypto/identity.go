package crypto

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/flynn/noise"
)

type Identity struct {
	SigningPrivateKey       ed25519.PrivateKey
	SigningPublicKey        ed25519.PublicKey
	KeyAgreementPrivateKey []byte
	KeyAgreementPublicKey  []byte
}

func GenerateIdentity() (*Identity, error) {
	signingPublicKey, signingPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 key: %w", err)
	}

	curve := ecdh.X25519()
	keyAgreementPrivateKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate x25519 key: %w", err)
	}

	return &Identity{
		SigningPrivateKey:       signingPrivateKey,
		SigningPublicKey:        signingPublicKey,
		KeyAgreementPrivateKey: append([]byte(nil), keyAgreementPrivateKey.Bytes()...),
		KeyAgreementPublicKey:  append([]byte(nil), keyAgreementPrivateKey.PublicKey().Bytes()...),
	}, nil
}

func (i *Identity) Fingerprint() string {
	sum := sha256.Sum256(append(append([]byte(nil), i.SigningPublicKey...), i.KeyAgreementPublicKey...))
	encoded := strings.ToUpper(hex.EncodeToString(sum[:16]))

	parts := make([]string, 0, len(encoded)/2)
	for idx := 0; idx < len(encoded); idx += 2 {
		parts = append(parts, encoded[idx:idx+2])
	}

	return strings.Join(parts, ":")
}

func (i *Identity) NoiseStaticKeypair() noise.DHKey {
	return noise.DHKey{
		Private: append([]byte(nil), i.KeyAgreementPrivateKey...),
		Public:  append([]byte(nil), i.KeyAgreementPublicKey...),
	}
}

func (i *Identity) SignedStaticKey() []byte {
	return ed25519.Sign(i.SigningPrivateKey, i.KeyAgreementPublicKey)
}

func VerifySignedStaticKey(signingPublicKey, staticKey, signature []byte) bool {
	return ed25519.Verify(signingPublicKey, staticKey, signature)
}

func (i *Identity) Validate() error {
	if i == nil {
		return errors.New("identity is nil")
	}
	if len(i.SigningPrivateKey) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid ed25519 private key size: %d", len(i.SigningPrivateKey))
	}
	if len(i.SigningPublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid ed25519 public key size: %d", len(i.SigningPublicKey))
	}
	if len(i.KeyAgreementPrivateKey) != 32 {
		return fmt.Errorf("invalid x25519 private key size: %d", len(i.KeyAgreementPrivateKey))
	}
	if len(i.KeyAgreementPublicKey) != 32 {
		return fmt.Errorf("invalid x25519 public key size: %d", len(i.KeyAgreementPublicKey))
	}

	derivedSigningPublic := i.SigningPrivateKey.Public().(ed25519.PublicKey)
	if !ed25519.PublicKey.Equal(derivedSigningPublic, i.SigningPublicKey) {
		return errors.New("ed25519 public key does not match private key")
	}

	curve := ecdh.X25519()
	keyAgreementPrivateKey, err := curve.NewPrivateKey(i.KeyAgreementPrivateKey)
	if err != nil {
		return fmt.Errorf("parse x25519 private key: %w", err)
	}
	if !equalBytes(keyAgreementPrivateKey.PublicKey().Bytes(), i.KeyAgreementPublicKey) {
		return errors.New("x25519 public key does not match private key")
	}

	return nil
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}
