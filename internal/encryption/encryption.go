package encryption

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	KeySize = 32

	magic     = "BACKLOT-ENCRYPTED-V1"
	algorithm = "XCHACHA20POLY1305-HMAC-SHA256"
)

var (
	ErrInvalidKey    = errors.New("invalid encryption key")
	ErrInvalidBlob   = errors.New("invalid encrypted blob")
	ErrDecryptFailed = errors.New("decrypt encrypted blob")
)

type Envelope struct {
	VaultID    string
	KeyID      string
	Nonce      []byte
	Ciphertext []byte
}

func Encrypt(key []byte, vaultID, keyID, path string, plaintext []byte) ([]byte, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	if isEncryptedFor(key, vaultID, keyID, path, plaintext) {
		return append([]byte(nil), plaintext...), nil
	}
	aead, err := chacha20poly1305.NewX(deriveKey(key, "backlot encryption key"))
	if err != nil {
		return nil, err
	}
	nonce := deriveNonce(key, vaultID, keyID, normalizePath(path), plaintext)
	ciphertext := aead.Seal(nil, nonce, plaintext, aad(vaultID, keyID, path))
	return renderEnvelope(Envelope{
		VaultID:    vaultID,
		KeyID:      keyID,
		Nonce:      nonce,
		Ciphertext: ciphertext,
	}), nil
}

func Decrypt(key []byte, path string, blob []byte) ([]byte, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	envelope, err := ParseEnvelope(blob)
	if err != nil {
		return nil, err
	}
	aead, err := chacha20poly1305.NewX(deriveKey(key, "backlot encryption key"))
	if err != nil {
		return nil, err
	}
	plaintext, err := aead.Open(nil, envelope.Nonce, envelope.Ciphertext, aad(envelope.VaultID, envelope.KeyID, path))
	if err != nil {
		return nil, ErrDecryptFailed
	}
	return plaintext, nil
}

func IsEncrypted(data []byte) bool {
	_, err := ParseEnvelope(data)
	return err == nil
}

func isEncryptedFor(key []byte, vaultID, keyID, path string, data []byte) bool {
	envelope, err := ParseEnvelope(data)
	if err != nil {
		return false
	}
	if envelope.VaultID != vaultID || envelope.KeyID != keyID {
		return false
	}
	_, err = Decrypt(key, path, data)
	return err == nil
}

func ParseEnvelope(data []byte) (Envelope, error) {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 6 || lines[0] != magic || lines[1] != algorithm {
		return Envelope{}, ErrInvalidBlob
	}
	if !strings.HasPrefix(lines[2], "vault:") || !strings.HasPrefix(lines[3], "key:") ||
		!strings.HasPrefix(lines[4], "nonce:") || !strings.HasPrefix(lines[5], "ciphertext:") {
		return Envelope{}, ErrInvalidBlob
	}
	nonce, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(lines[4], "nonce:"))
	if err != nil || len(nonce) != chacha20poly1305.NonceSizeX {
		return Envelope{}, ErrInvalidBlob
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(lines[5], "ciphertext:"))
	if err != nil {
		return Envelope{}, ErrInvalidBlob
	}
	return Envelope{
		VaultID:    strings.TrimPrefix(lines[2], "vault:"),
		KeyID:      strings.TrimPrefix(lines[3], "key:"),
		Nonce:      nonce,
		Ciphertext: ciphertext,
	}, nil
}

func renderEnvelope(envelope Envelope) []byte {
	return []byte(fmt.Sprintf("%s\n%s\nvault:%s\nkey:%s\nnonce:%s\nciphertext:%s\n",
		magic,
		algorithm,
		envelope.VaultID,
		envelope.KeyID,
		base64.RawURLEncoding.EncodeToString(envelope.Nonce),
		base64.RawURLEncoding.EncodeToString(envelope.Ciphertext),
	))
}

func validateKey(key []byte) error {
	if len(key) != KeySize {
		return fmt.Errorf("%w: want %d bytes", ErrInvalidKey, KeySize)
	}
	return nil
}

func deriveKey(key []byte, label string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(label))
	return mac.Sum(nil)
}

func deriveNonce(key []byte, vaultID, keyID, path string, plaintext []byte) []byte {
	mac := hmac.New(sha256.New, deriveKey(key, "backlot nonce key"))
	mac.Write([]byte(magic))
	mac.Write([]byte{0})
	mac.Write([]byte(vaultID))
	mac.Write([]byte{0})
	mac.Write([]byte(keyID))
	mac.Write([]byte{0})
	mac.Write([]byte(path))
	mac.Write([]byte{0})
	mac.Write(plaintext)
	return mac.Sum(nil)[:chacha20poly1305.NonceSizeX]
}

func aad(vaultID, keyID, path string) []byte {
	return []byte(magic + "\x00" + algorithm + "\x00" + vaultID + "\x00" + keyID + "\x00" + normalizePath(path))
}

func normalizePath(path string) string {
	return strings.TrimPrefix(strings.ReplaceAll(path, "\\", "/"), "./")
}
