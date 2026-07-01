package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	archivecrypt "github.com/massivemoose/backlot/internal/encryption"
	"github.com/massivemoose/backlot/internal/gitutil"
	"github.com/massivemoose/backlot/internal/paths"
	"github.com/massivemoose/chomp"
)

const archiveAttributes = `* filter=backlot diff=backlot
.gitattributes -filter -diff
.backlot-root -filter -diff
.backlot-encryption.json -filter -diff
README.md -filter -diff
`

var encryptionFilterCommandPrefix = defaultEncryptionFilterCommandPrefix

func runLock(args []string, stdout, stderr io.Writer) error {
	result, err := lockSpec().Parse(args)
	if err != nil {
		return err
	}
	root, err := paths.BacklotRoot(result.String("root"))
	if err != nil {
		return err
	}
	if err := requireBacklotArchiveRoot(root); err != nil {
		return err
	}

	meta, key, created, err := ensureEncryptionMetadataAndKey(root)
	if err != nil {
		return err
	}
	if err := configureEncryptionFilters(root); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, ".gitattributes"), []byte(archiveAttributes), 0o644); err != nil {
		return err
	}
	if _, err := gitutil.RunGit(root, "add", "--renormalize", "-A"); err != nil {
		return err
	}
	if _, err := gitutil.RunGit(root, "add", "-A"); err != nil {
		return err
	}

	fmt.Fprintln(stdout, "Backlot archive encryption enabled")
	if created {
		fmt.Fprintf(stdout, "Recovery key: %s\n", archivecrypt.EncodeRecoveryKey(key))
	}
	fmt.Fprintln(stdout, "Run backlot sync to commit and sync encrypted archive contents.")
	_ = meta
	return nil
}

func runUnlock(args []string, stdout, stderr io.Writer) error {
	result, err := unlockSpec().Parse(args)
	if err != nil {
		return err
	}
	root, err := paths.BacklotRoot(result.String("root"))
	if err != nil {
		return err
	}
	if err := requireBacklotArchiveRoot(root); err != nil {
		return err
	}
	meta, err := archivecrypt.LoadMetadata(root)
	if err != nil {
		return err
	}
	key, imported, err := loadOrImportLocalKey(root, meta, result.String("recovery-key-file"))
	if err != nil {
		return err
	}
	if err := configureEncryptionFilters(root); err != nil {
		return err
	}
	if err := refreshEncryptedWorktree(root, key); err != nil {
		if imported {
			_ = removeLocalEncryptionKey(root, meta.VaultID)
		}
		return err
	}
	fmt.Fprintln(stdout, "Backlot archive unlocked")
	return nil
}

func runEncryptionDisable(args []string, stdout, stderr io.Writer) error {
	result, err := encryptionDisableSpec().Parse(args)
	if err != nil {
		return err
	}
	root, err := paths.BacklotRoot(result.String("root"))
	if err != nil {
		return err
	}
	if err := requireBacklotArchiveRoot(root); err != nil {
		return err
	}

	meta, err := archivecrypt.LoadMetadata(root)
	if err == nil {
		key, err := archivecrypt.LoadLocalKey(root, meta.VaultID)
		if errors.Is(err, archivecrypt.ErrKeyMissing) {
			return fmt.Errorf("%w: encrypted Backlot archive is locked; run backlot unlock --root %s --recovery-key-file PATH", err, root)
		}
		if err != nil {
			return err
		}
		if err := refreshEncryptedWorktree(root, key); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := removeArchiveEncryptionFile(root, archivecrypt.MetadataFile); err != nil {
		return err
	}
	if err := removeArchiveEncryptionFile(root, ".gitattributes"); err != nil {
		return err
	}
	for _, key := range []string{
		"filter.backlot.clean",
		"filter.backlot.smudge",
		"filter.backlot.required",
	} {
		if err := unsetGitConfigIfPresent(root, key); err != nil {
			return err
		}
	}
	if _, err := gitutil.RunGit(root, "add", "-A"); err != nil {
		return err
	}
	if _, err := gitutil.RunGit(root, "add", "--renormalize", "-A"); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Backlot archive encryption disabled")
	fmt.Fprintln(stdout, "Run backlot sync to commit and sync plaintext archive contents.")
	return nil
}

func ensureEncryptionMetadataAndKey(root string) (archivecrypt.Metadata, []byte, bool, error) {
	meta, err := archivecrypt.LoadMetadata(root)
	if err == nil {
		key, err := archivecrypt.LoadLocalKey(root, meta.VaultID)
		return meta, key, false, err
	}
	if !errors.Is(err, os.ErrNotExist) {
		return archivecrypt.Metadata{}, nil, false, err
	}
	vaultID, err := archivecrypt.GenerateID()
	if err != nil {
		return archivecrypt.Metadata{}, nil, false, err
	}
	keyID, err := archivecrypt.GenerateID()
	if err != nil {
		return archivecrypt.Metadata{}, nil, false, err
	}
	key, err := archivecrypt.GenerateKey()
	if err != nil {
		return archivecrypt.Metadata{}, nil, false, err
	}
	meta = archivecrypt.Metadata{
		SchemaVersion: archivecrypt.SchemaVersion,
		VaultID:       vaultID,
		ActiveKeyID:   keyID,
		Algorithm:     archivecrypt.Algorithm,
	}
	if err := archivecrypt.WriteMetadata(root, meta); err != nil {
		return archivecrypt.Metadata{}, nil, false, err
	}
	if err := archivecrypt.StoreLocalKey(root, vaultID, key); err != nil {
		return archivecrypt.Metadata{}, nil, false, err
	}
	return meta, key, true, nil
}

func loadOrImportLocalKey(root string, meta archivecrypt.Metadata, recoveryKeyFile string) ([]byte, bool, error) {
	key, err := archivecrypt.LoadLocalKey(root, meta.VaultID)
	if err == nil {
		return key, false, nil
	}
	if !errors.Is(err, archivecrypt.ErrKeyMissing) {
		return nil, false, err
	}
	if recoveryKeyFile == "" {
		return nil, false, fmt.Errorf("%w: encrypted Backlot archive is locked; run backlot unlock --recovery-key-file PATH", err)
	}
	data, err := os.ReadFile(recoveryKeyFile)
	if err != nil {
		return nil, false, err
	}
	key, err = archivecrypt.DecodeRecoveryKey(string(data))
	if err != nil {
		return nil, false, err
	}
	if err := authenticateEncryptedWorktreeBlob(root, key); err != nil {
		return nil, false, err
	}
	if err := archivecrypt.StoreLocalKey(root, meta.VaultID, key); err != nil {
		return nil, false, err
	}
	return key, true, nil
}

func configureEncryptionFilters(root string) error {
	prefix, err := encryptionFilterCommandPrefix()
	if err != nil {
		return err
	}
	settings := map[string]string{
		"filter.backlot.clean":    prefix + " encrypt --path %f",
		"filter.backlot.smudge":   prefix + " decrypt --path %f",
		"filter.backlot.required": "true",
	}
	for key, value := range settings {
		if _, err := gitutil.RunGit(root, "config", key, value); err != nil {
			return err
		}
	}
	return nil
}

func refreshEncryptedWorktree(root string, key []byte) error {
	files, err := gitutil.RunGit(root, "ls-files")
	if err != nil {
		return err
	}
	for _, rel := range strings.Split(files, "\n") {
		rel = strings.TrimSpace(rel)
		if rel == "" {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !archivecrypt.IsEncrypted(data) {
			continue
		}
		plaintext, err := archivecrypt.Decrypt(key, rel, data)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, plaintext, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func authenticateEncryptedWorktreeBlob(root string, key []byte) error {
	files, err := gitutil.RunGit(root, "ls-files")
	if err != nil {
		return err
	}
	for _, rel := range strings.Split(files, "\n") {
		rel = strings.TrimSpace(rel)
		if rel == "" {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !archivecrypt.IsEncrypted(data) {
			continue
		}
		_, err = archivecrypt.Decrypt(key, rel, data)
		return err
	}
	return nil
}

func removeLocalEncryptionKey(root, vaultID string) error {
	path, err := archivecrypt.LocalKeyPath(root, vaultID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func removeArchiveEncryptionFile(root, name string) error {
	err := os.Remove(filepath.Join(root, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func unsetGitConfigIfPresent(root, key string) error {
	if _, err := gitutil.RunGit(root, "config", "--get-all", key); err != nil {
		return nil
	}
	_, err := gitutil.RunGit(root, "config", "--unset-all", key)
	return err
}

func defaultEncryptionFilterCommandPrefix() (string, error) {
	binary, err := os.Executable()
	if err != nil {
		return "", err
	}
	return shellQuote(binary), nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func lockSpec() *chomp.Spec {
	return chomp.New("backlot", "lock").
		String("root", chomp.ValueName("path"), chomp.Description("Backlot root path")).
		Positionals(0, 0)
}

func unlockSpec() *chomp.Spec {
	return chomp.New("backlot", "unlock").
		String("root", chomp.ValueName("path"), chomp.Description("Backlot root path")).
		String("recovery-key-file", chomp.ValueName("path"), chomp.Description("file containing the recovery key")).
		Positionals(0, 0)
}

func encryptionDisableSpec() *chomp.Spec {
	return chomp.New("backlot", "encryption disable").
		String("root", chomp.ValueName("path"), chomp.Description("Backlot root path")).
		Positionals(0, 0)
}

func printLockUsage(w io.Writer) {
	printSpecUsage(w, lockSpec())
}

func printUnlockUsage(w io.Writer) {
	printSpecUsage(w, unlockSpec())
}

func printEncryptionDisableUsage(w io.Writer) {
	printSpecUsage(w, encryptionDisableSpec())
}
