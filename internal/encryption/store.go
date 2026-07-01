package encryption

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/massivemoose/backlot/internal/gitutil"
)

const (
	MetadataFile  = ".backlot-encryption.json"
	SchemaVersion = 1
)

var (
	ErrKeyMissing           = errors.New("encryption key missing")
	ErrUnsafeKeyPermissions = errors.New("unsafe encryption key permissions")
)

type Metadata struct {
	SchemaVersion int    `json:"schema_version"`
	VaultID       string `json:"vault_id"`
	ActiveKeyID   string `json:"active_key_id"`
	Algorithm     string `json:"algorithm"`
}

func LoadMetadata(root string) (Metadata, error) {
	var meta Metadata
	data, err := os.ReadFile(filepath.Join(root, MetadataFile))
	if err != nil {
		return Metadata{}, err
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return Metadata{}, fmt.Errorf("parse %s: %w", MetadataFile, err)
	}
	if meta.SchemaVersion != SchemaVersion {
		return Metadata{}, fmt.Errorf("unsupported encryption metadata schema version %d", meta.SchemaVersion)
	}
	if meta.VaultID == "" || meta.ActiveKeyID == "" {
		return Metadata{}, errors.New("encryption metadata is missing vault or key id")
	}
	if meta.Algorithm != Algorithm {
		return Metadata{}, fmt.Errorf("unsupported encryption algorithm %q", meta.Algorithm)
	}
	return meta, nil
}

func WriteMetadata(root string, meta Metadata) error {
	if meta.SchemaVersion == 0 {
		meta.SchemaVersion = SchemaVersion
	}
	if meta.Algorithm == "" {
		meta.Algorithm = Algorithm
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(root, MetadataFile), data, 0o644)
}

func LocalKeyPath(root, vaultID string) (string, error) {
	if strings.ContainsAny(vaultID, `/\`) || vaultID == "" || vaultID == "." || vaultID == ".." {
		return "", fmt.Errorf("unsafe vault id %q", vaultID)
	}
	return gitutil.GitPath(root, filepath.ToSlash(filepath.Join("backlot", "keys", vaultID)))
}

func StoreLocalKey(root, vaultID string, key []byte) error {
	if err := validateKey(key); err != nil {
		return err
	}
	path, err := LocalKeyPath(root, vaultID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".backlot-key-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(key); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func LoadLocalKey(root, vaultID string) ([]byte, error) {
	path, err := LocalKeyPath(root, vaultID)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, ErrKeyMissing
	} else if err != nil {
		return nil, err
	}
	if err := checkLocalKeyPermissions(path); err != nil {
		return nil, err
	}
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := validateKey(key); err != nil {
		return nil, err
	}
	return key, nil
}

func checkLocalKeyPermissions(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: %s has mode %o", ErrUnsafeKeyPermissions, path, info.Mode().Perm())
	}
	return nil
}

func EncodeRecoveryKey(key []byte) string {
	return base64.RawURLEncoding.EncodeToString(key)
}

func DecodeRecoveryKey(text string) ([]byte, error) {
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil {
		return nil, ErrInvalidKey
	}
	if err := validateKey(key); err != nil {
		return nil, err
	}
	return key, nil
}

func GenerateKey() ([]byte, error) {
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

func GenerateID() (string, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}
