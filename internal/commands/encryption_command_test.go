package commands

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

func TestLockConfiguresFiltersAndEncryptsGitBlob(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	mustRunBacklotInit(t, root)
	configureGitIdentity(t, root)
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	restore := withEncryptionFilterHelper(t)
	defer restore()

	var out, errOut bytes.Buffer
	if code := Run([]string{"lock", "--root", root}, &out, &errOut); code != 0 {
		t.Fatalf("lock exit code = %d, stderr = %s", code, errOut.String())
	}
	if _, err := encryption.LoadMetadata(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".gitattributes")); err != nil {
		t.Fatal(err)
	}
	if got := runGitOutput(t, root, "config", "--get", "filter.backlot.required"); got != "true" {
		t.Fatalf("filter.backlot.required = %q", got)
	}
	if got := mustReadFile(t, filepath.Join(root, "secret.txt")); got != "secret\n" {
		t.Fatalf("worktree secret = %q", got)
	}
	if !strings.Contains(runGitOutput(t, root, "cat-file", "-p", ":secret.txt"), "BACKLOT-ENCRYPTED-V1") {
		t.Fatal("staged secret blob is not encrypted")
	}
	if !strings.Contains(out.String(), "Recovery key:") || !strings.Contains(out.String(), "backlot sync") {
		t.Fatalf("lock output missing recovery key or sync guidance:\n%s", out.String())
	}
}

func TestUnlockFromRecoveryKeyRestoresPlaintextClone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell filter helper is Unix-only")
	}
	tmp := t.TempDir()
	seed := filepath.Join(tmp, "seed")
	clone := filepath.Join(tmp, "clone")
	mustRunBacklotInit(t, seed)
	configureGitIdentity(t, seed)
	if err := os.WriteFile(filepath.Join(seed, "secret.txt"), []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	restore := withEncryptionFilterHelper(t)
	defer restore()

	var out, errOut bytes.Buffer
	if code := Run([]string{"lock", "--root", seed}, &out, &errOut); code != 0 {
		t.Fatalf("lock exit code = %d, stderr = %s", code, errOut.String())
	}
	recovery := recoveryKeyFromOutput(t, out.String())
	mustRunGit(t, seed, "commit", "-m", "Encrypt archive")
	mustRunGit(t, tmp, "clone", seed, clone)
	if !strings.Contains(mustReadFile(t, filepath.Join(clone, "secret.txt")), "BACKLOT-ENCRYPTED-V1") {
		t.Fatal("clone was unexpectedly plaintext before unlock")
	}
	recoveryFile := filepath.Join(tmp, "recovery.key")
	if err := os.WriteFile(recoveryFile, []byte(recovery+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"unlock", "--root", clone, "--recovery-key-file", recoveryFile}, &out, &errOut); code != 0 {
		t.Fatalf("unlock exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := mustReadFile(t, filepath.Join(clone, "secret.txt")); got != "secret\n" {
		t.Fatalf("clone secret after unlock = %q", got)
	}
}

func TestUnlockWithWrongRecoveryKeyDoesNotPersistLocalKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell filter helper is Unix-only")
	}
	tmp := t.TempDir()
	seed := filepath.Join(tmp, "seed")
	clone := filepath.Join(tmp, "clone")
	mustRunBacklotInit(t, seed)
	configureGitIdentity(t, seed)
	if err := os.WriteFile(filepath.Join(seed, "secret.txt"), []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	restore := withEncryptionFilterHelper(t)
	defer restore()

	var out, errOut bytes.Buffer
	if code := Run([]string{"lock", "--root", seed}, &out, &errOut); code != 0 {
		t.Fatalf("lock exit code = %d, stderr = %s", code, errOut.String())
	}
	mustRunGit(t, seed, "commit", "-m", "Encrypt archive")
	mustRunGit(t, tmp, "clone", seed, clone)
	wrongRecoveryFile := filepath.Join(tmp, "wrong.key")
	if err := os.WriteFile(wrongRecoveryFile, []byte(encryption.EncodeRecoveryKey(testCommandKey())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"unlock", "--root", clone, "--recovery-key-file", wrongRecoveryFile}, &out, &errOut); code == 0 {
		t.Fatalf("unlock succeeded with wrong recovery key, stdout = %s", out.String())
	}
	meta, err := encryption.LoadMetadata(clone)
	if err != nil {
		t.Fatal(err)
	}
	keyPath, err := encryption.LocalKeyPath(clone, meta.VaultID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("wrong recovery key persisted at %s: %v", keyPath, err)
	}
}

func TestUnlockSkipsTrackedSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink behavior differs on Windows")
	}
	tmp := t.TempDir()
	seed := filepath.Join(tmp, "seed")
	clone := filepath.Join(tmp, "clone")
	mustRunBacklotInit(t, seed)
	configureGitIdentity(t, seed)
	if err := os.WriteFile(filepath.Join(seed, "secret.txt"), []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("secret.txt", filepath.Join(seed, "secret-link.txt")); err != nil {
		t.Fatal(err)
	}

	restore := withEncryptionFilterHelper(t)
	defer restore()

	var out, errOut bytes.Buffer
	if code := Run([]string{"lock", "--root", seed}, &out, &errOut); code != 0 {
		t.Fatalf("lock exit code = %d, stderr = %s", code, errOut.String())
	}
	recovery := recoveryKeyFromOutput(t, out.String())
	mustRunGit(t, seed, "commit", "-m", "Encrypt archive")
	mustRunGit(t, tmp, "clone", seed, clone)
	recoveryFile := filepath.Join(tmp, "recovery.key")
	if err := os.WriteFile(recoveryFile, []byte(recovery+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	errOut.Reset()
	if code := Run([]string{"unlock", "--root", clone, "--recovery-key-file", recoveryFile}, &out, &errOut); code != 0 {
		t.Fatalf("unlock exit code = %d, stderr = %s", code, errOut.String())
	}
	if got := mustReadFile(t, filepath.Join(clone, "secret.txt")); got != "secret\n" {
		t.Fatalf("clone secret after unlock = %q", got)
	}
	if target, err := os.Readlink(filepath.Join(clone, "secret-link.txt")); err != nil {
		t.Fatalf("secret-link.txt is not a symlink: %v", err)
	} else if target != "secret.txt" {
		t.Fatalf("secret-link.txt target = %q, want secret.txt", target)
	}
}

func TestBacklotFilterHelper(t *testing.T) {
	if os.Getenv("BACKLOT_FILTER_HELPER") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		os.Exit(2)
	}
	os.Exit(Run(args[1:], os.Stdout, os.Stderr))
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

func withEncryptionFilterHelper(t *testing.T) func() {
	t.Helper()
	old := encryptionFilterCommandPrefix
	encryptionFilterCommandPrefix = func() (string, error) {
		return "BACKLOT_FILTER_HELPER=1 " + shellQuote(os.Args[0]) + " -test.run TestBacklotFilterHelper --", nil
	}
	return func() { encryptionFilterCommandPrefix = old }
}

func recoveryKeyFromOutput(t *testing.T, output string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if key, ok := strings.CutPrefix(line, "Recovery key: "); ok {
			return strings.TrimSpace(key)
		}
	}
	t.Fatalf("missing recovery key in output:\n%s", output)
	return ""
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func testCommandKey() []byte {
	key := make([]byte, encryption.KeySize)
	for i := range key {
		key[i] = byte(i + 11)
	}
	return key
}
