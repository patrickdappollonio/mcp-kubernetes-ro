package secrets

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // see Encrypt for why OAEP uses SHA-1
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
)

// MinKeyBits is the smallest RSA key Encrypt accepts.
const MinKeyBits = 2048

// Encrypt encrypts plaintext to a PEM RSA public key (PKIX or PKCS#1) and returns base64
// RSA-OAEP blocks; decrypting them in order and concatenating the results restores it.
//
// OAEP uses SHA-1: it is the default of "openssl pkeyutl", and LibreSSL documents no
// option to change it. SHA-1's collision weakness does not affect OAEP.
func Encrypt(publicKeyPEM string, plaintext []byte) ([]string, error) {
	pub, err := parsePublicKey(publicKeyPEM)
	if err != nil {
		return nil, err
	}

	maxChunk := pub.Size() - 2*sha1.Size - 2
	blocks := make([]string, 0, len(plaintext)/maxChunk+1)

	for {
		n := min(maxChunk, len(plaintext))
		ct, err := rsa.EncryptOAEP(sha1.New(), rand.Reader, pub, plaintext[:n], nil)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt value: %w", err)
		}
		blocks = append(blocks, base64.StdEncoding.EncodeToString(ct))

		plaintext = plaintext[n:]
		if len(plaintext) == 0 {
			return blocks, nil
		}
	}
}

func parsePublicKey(publicKeyPEM string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return nil, errors.New(`public key is not PEM encoded: expected a "-----BEGIN PUBLIC KEY-----" block`)
	}

	var parsed any
	var err error
	switch block.Type {
	case "PUBLIC KEY":
		parsed, err = x509.ParsePKIXPublicKey(block.Bytes)
	case "RSA PUBLIC KEY":
		parsed, err = x509.ParsePKCS1PublicKey(block.Bytes)
	default:
		return nil, fmt.Errorf("PEM block %q is not a public key: send the public key, never the private key", block.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	pub, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key must be an RSA key, got %T", parsed)
	}

	if bits := pub.N.BitLen(); bits < MinKeyBits {
		return nil, fmt.Errorf("RSA key is %d bits, at least %d are required", bits, MinKeyBits)
	}

	return pub, nil
}
