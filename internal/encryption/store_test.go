package encryption

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMetadataRoundTrip(t *testing.T) {
	root := newGitRepo(t)
	meta := Metadata{
		SchemaVersion: 1,
		VaultID:       "vault",
		ActiveKeyID:   "key",
		Algorithm:     Algorithm,
	}
	if err := WriteMetadata(root, meta); err != nil {
		t.Fatal(err)
	}
	got, err := LoadMetadata(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != meta {
		t.Fatalf("metadata = %#v, want %#v", got, meta)
	}
}

func TestMetadataRejectsInvalidAlgorithm(t *testing.T) {
	root := newGitRepo(t)
	if err := os.WriteFile(filepath.Join(root, MetadataFile), []byte(`{"schema_version":1,"vault_id":"vault","active_key_id":"key","algorithm":"nope"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMetadata(root); err == nil {
		t.Fatal("LoadMetadata accepted invalid algorithm")
	}
}

func TestLocalKeyStoreRoundTripAndPermissions(t *testing.T) {
	root := newGitRepo(t)
	key := testKey(7)
	if err := StoreLocalKey(root, "vault", key); err != nil {
		t.Fatal(err)
	}
	got, err := LoadLocalKey(root, "vault")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(key) {
		t.Fatalf("key changed")
	}
	path, err := LocalKeyPath(root, "vault")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key permissions = %o, want 0600", info.Mode().Perm())
	}
}

func TestStoreLocalKeyTightensExistingPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file mode test")
	}
	root := newGitRepo(t)
	path, err := LocalKeyPath(root, "vault")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, testKey(1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := StoreLocalKey(root, "vault", testKey(2)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key permissions = %o, want 0600", info.Mode().Perm())
	}
}

func TestLoadLocalKeyRejectsLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file mode test")
	}
	root := newGitRepo(t)
	path, err := LocalKeyPath(root, "vault")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, testKey(1), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLocalKey(root, "vault"); !errors.Is(err, ErrUnsafeKeyPermissions) {
		t.Fatalf("loose key permissions error = %v, want ErrUnsafeKeyPermissions", err)
	}
}

func TestLocalKeyStoreReportsMissingAndInvalidKeys(t *testing.T) {
	root := newGitRepo(t)
	if _, err := LoadLocalKey(root, "vault"); !errors.Is(err, ErrKeyMissing) {
		t.Fatalf("missing key error = %v, want ErrKeyMissing", err)
	}
	path, err := LocalKeyPath(root, "vault")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLocalKey(root, "vault"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("invalid key error = %v, want ErrInvalidKey", err)
	}
}

func TestRecoveryKeyEncodeDecode(t *testing.T) {
	key := testKey(8)
	text := EncodeRecoveryKey(key)
	got, err := DecodeRecoveryKey(text)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(key) {
		t.Fatalf("recovery key changed")
	}
	if _, err := DecodeRecoveryKey("not-a-key"); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("bad recovery error = %v, want ErrInvalidKey", err)
	}
}

func newGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	cmd := exec.Command("git", "init", root)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	return root
}
