package wireguard

import (
	"encoding/base64"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	key, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate key pair: %v", err)
	}

	private, err := base64.StdEncoding.DecodeString(key.PrivateKey)
	if err != nil {
		t.Fatalf("decode private key: %v", err)
	}
	public, err := base64.StdEncoding.DecodeString(key.PublicKey)
	if err != nil {
		t.Fatalf("decode public key: %v", err)
	}
	if len(private) != 32 {
		t.Fatalf("expected private key length 32, got %d", len(private))
	}
	if len(public) != 32 {
		t.Fatalf("expected public key length 32, got %d", len(public))
	}
}
