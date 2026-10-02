package secrets

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // OAEP uses SHA-1 to match LibreSSL defaults; see Encrypt
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"
	"testing"
)

func newRSAKey(t *testing.T, bits int) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	return key
}

func pkixPEM(t *testing.T, pub any) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshaling public key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func decryptBlocks(t *testing.T, key *rsa.PrivateKey, blocks []string) []byte {
	t.Helper()
	var out []byte
	for i, block := range blocks {
		ct, err := base64.StdEncoding.DecodeString(block)
		if err != nil {
			t.Fatalf("block %d is not base64: %v", i, err)
		}
		plain, err := rsa.DecryptOAEP(sha1.New(), nil, key, ct, nil)
		if err != nil {
			t.Fatalf("decrypting block %d: %v", i, err)
		}
		out = append(out, plain...)
	}
	return out
}

func TestEncryptRoundTrip(t *testing.T) {
	t.Parallel()

	key := newRSAKey(t, 2048)
	pub := pkixPEM(t, &key.PublicKey)
	maxBlock := key.PublicKey.Size() - 2*sha1.Size - 2

	tests := []struct {
		name       string
		plain      []byte
		wantBlocks int
	}{
		{"empty value", []byte{}, 1},
		{"short value", []byte("s3cr3t-p@ss"), 1},
		{"exactly one block", bytes.Repeat([]byte("a"), maxBlock), 1},
		{"one byte over a block", bytes.Repeat([]byte("a"), maxBlock+1), 2},
		{"certificate-sized value", bytes.Repeat([]byte("0123456789abcdef"), 256), 4096/maxBlock + 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			blocks, err := Encrypt(pub, tt.plain)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}
			if len(blocks) != tt.wantBlocks {
				t.Fatalf("Encrypt() returned %d blocks, want %d", len(blocks), tt.wantBlocks)
			}
			if got := decryptBlocks(t, key, blocks); !bytes.Equal(got, tt.plain) {
				t.Fatalf("round trip mismatch: got %d bytes, want %d", len(got), len(tt.plain))
			}
		})
	}
}

func TestEncryptAcceptsPKCS1PublicKey(t *testing.T) {
	t.Parallel()

	key := newRSAKey(t, 2048)
	pub := string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey),
	}))

	blocks, err := Encrypt(pub, []byte("value"))
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if got := decryptBlocks(t, key, blocks); string(got) != "value" {
		t.Fatalf("round trip mismatch: got %q", got)
	}
}

func TestEncryptRejectsUnsuitableKeys(t *testing.T) {
	t.Parallel()

	edPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		key     string
		wantErr string
	}{
		{"not PEM", "age1qyqszqgpqyqszqgpqyqszqgpqyqszqgp", "PEM"},
		{"private key instead of public", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("x")})), "public key"},
		{"not an RSA key", pkixPEM(t, edPub), "RSA"},
		{"RSA key too small", pkixPEM(t, &newRSAKey(t, 1024).PublicKey), "2048"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := Encrypt(tt.key, []byte("value"))
			if err == nil {
				t.Fatal("Encrypt() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Encrypt() error = %q, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}
