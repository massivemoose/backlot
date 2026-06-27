package commands

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/massivemoose/backlot/internal/encryption"
)

func TestEncryptionFilterCommandsRoundTrip(t *testing.T) {
	root := newEncryptedArchive(t)
	var encrypted bytes.Buffer
	if err := runEncrypt([]string{"--root", root, "--path", "notes.md"}, strings.NewReader("secret\n"), &encrypted); err != nil {
		t.Fatal(err)
	}
	if !encryption.IsEncrypted(encrypted.Bytes()) {
		t.Fatalf("encrypt output was not encrypted:\n%s", encrypted.String())
	}

	var decrypted bytes.Buffer
	if err := runDecrypt([]string{"--root", root, "--path", "notes.md"}, bytes.NewReader(encrypted.Bytes()), &decrypted); err != nil {
		t.Fatal(err)
	}
	if decrypted.String() != "secret\n" {
		t.Fatalf("decrypt output = %q", decrypted.String())
	}
}

func TestEncryptionFilterResolvesRootFromGitRepo(t *testing.T) {
	root := newEncryptedArchive(t)
	withChdir(t, root, func() {
		var encrypted bytes.Buffer
		if err := runEncrypt([]string{"--path", "notes.md"}, strings.NewReader("secret\n"), &encrypted); err != nil {
			t.Fatal(err)
		}
		if !encryption.IsEncrypted(encrypted.Bytes()) {
			t.Fatalf("encrypt output was not encrypted")
		}
	})
}

func TestEncryptionFilterMissingKeyIsActionable(t *testing.T) {
	root := newEncryptedArchive(t)
	path, err := encryption.LocalKeyPath(root, "vault")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = runEncrypt([]string{"--root", root, "--path", "notes.md"}, strings.NewReader("secret\n"), &out)
	if !errors.Is(err, encryption.ErrKeyMissing) || !strings.Contains(err.Error(), "run backlot unlock") {
		t.Fatalf("missing key error = %v", err)
	}
}

func newEncryptedArchive(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "state")
	mustRunBacklotInit(t, root)
	meta := encryption.Metadata{
		SchemaVersion: encryption.SchemaVersion,
		VaultID:       "vault",
		ActiveKeyID:   "key",
		Algorithm:     encryption.Algorithm,
	}
	if err := encryption.WriteMetadata(root, meta); err != nil {
		t.Fatal(err)
	}
	if err := encryption.StoreLocalKey(root, meta.VaultID, testCommandKey()); err != nil {
		t.Fatal(err)
	}
	return root
}

func testCommandKey() []byte {
	key := make([]byte, encryption.KeySize)
	for i := range key {
		key[i] = byte(i + 11)
	}
	return key
}
