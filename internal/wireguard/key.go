package wireguard

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

type KeyPair struct {
	PrivateKey string
	PublicKey  string
}

func GenerateKeyPair() (KeyPair, error) {
	private := make([]byte, curve25519.ScalarSize)
	if _, err := rand.Read(private); err != nil {
		return KeyPair{}, fmt.Errorf("generate wireguard private key: %w", err)
	}

	private[0] &= 248
	private[31] = (private[31] & 127) | 64

	public, err := curve25519.X25519(private, curve25519.Basepoint)
	if err != nil {
		return KeyPair{}, fmt.Errorf("generate wireguard public key: %w", err)
	}

	return KeyPair{
		PrivateKey: base64.StdEncoding.EncodeToString(private),
		PublicKey:  base64.StdEncoding.EncodeToString(public),
	}, nil
}
