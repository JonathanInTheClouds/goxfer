package crypto

import (
	"strings"
	"testing"
)

func TestGenerateIdentity(t *testing.T) {
	id, err := GenerateIdentity()
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}
	if err := id.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestGenerateIdentity_Unique(t *testing.T) {
	a, _ := GenerateIdentity()
	b, _ := GenerateIdentity()
	if equalBytes(a.KeyAgreementPublicKey, b.KeyAgreementPublicKey) {
		t.Fatal("two identities have the same X25519 public key")
	}
}

func TestFingerprint_Format(t *testing.T) {
	id, _ := GenerateIdentity()
	fp := id.Fingerprint()
	// Expect "XX:XX:XX:..." with 16 hex pairs
	parts := strings.Split(fp, ":")
	if len(parts) != 16 {
		t.Fatalf("fingerprint has %d parts, want 16: %s", len(parts), fp)
	}
	for _, p := range parts {
		if len(p) != 2 {
			t.Fatalf("fingerprint part %q is not 2 chars", p)
		}
	}
}

func TestFingerprint_Deterministic(t *testing.T) {
	id, _ := GenerateIdentity()
	fp1 := id.Fingerprint()
	fp2 := id.Fingerprint()
	if fp1 != fp2 {
		t.Fatal("fingerprint is not deterministic")
	}
}

func TestFingerprint_Unique(t *testing.T) {
	a, _ := GenerateIdentity()
	b, _ := GenerateIdentity()
	if a.Fingerprint() == b.Fingerprint() {
		t.Fatal("two different identities have the same fingerprint")
	}
}

func TestNoiseStaticKeypair(t *testing.T) {
	id, _ := GenerateIdentity()
	kp := id.NoiseStaticKeypair()
	if len(kp.Private) != 32 {
		t.Fatalf("noise private key len %d, want 32", len(kp.Private))
	}
	if len(kp.Public) != 32 {
		t.Fatalf("noise public key len %d, want 32", len(kp.Public))
	}
}

func TestSignedStaticKey(t *testing.T) {
	id, _ := GenerateIdentity()
	sig := id.SignedStaticKey()
	if !VerifySignedStaticKey(id.SigningPublicKey, id.KeyAgreementPublicKey, sig) {
		t.Fatal("signature verification failed")
	}
}
