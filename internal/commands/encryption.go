package commands

import (
	"errors"
	"fmt"
	"io"

	archivecrypt "github.com/massivemoose/backlot/internal/encryption"
	"github.com/massivemoose/backlot/internal/gitutil"
	"github.com/massivemoose/backlot/internal/paths"
	"github.com/massivemoose/chomp"
)

func runEncrypt(args []string, stdin io.Reader, stdout io.Writer) error {
	root, path, err := parseEncryptionFilterArgs("encrypt", args)
	if err != nil {
		return err
	}
	meta, key, err := loadEncryptionFilterKey(root)
	if err != nil {
		return err
	}
	plaintext, err := io.ReadAll(stdin)
	if err != nil {
		return err
	}
	blob, err := archivecrypt.Encrypt(key, meta.VaultID, meta.ActiveKeyID, path, plaintext)
	if err != nil {
		return err
	}
	_, err = stdout.Write(blob)
	return err
}

func runDecrypt(args []string, stdin io.Reader, stdout io.Writer) error {
	root, path, err := parseEncryptionFilterArgs("decrypt", args)
	if err != nil {
		return err
	}
	_, key, err := loadEncryptionFilterKey(root)
	if err != nil {
		return err
	}
	blob, err := io.ReadAll(stdin)
	if err != nil {
		return err
	}
	plaintext, err := archivecrypt.Decrypt(key, path, blob)
	if err != nil {
		return err
	}
	_, err = stdout.Write(plaintext)
	return err
}

func parseEncryptionFilterArgs(command string, args []string) (string, string, error) {
	result, err := encryptionFilterSpec(command).Parse(args)
	if err != nil {
		return "", "", err
	}
	root, err := encryptionFilterRoot(result.String("root"))
	if err != nil {
		return "", "", err
	}
	if err := requireBacklotArchiveRoot(root); err != nil {
		return "", "", err
	}
	return root, result.String("path"), nil
}

func encryptionFilterRoot(rootFlag string) (string, error) {
	if rootFlag != "" {
		return paths.BacklotRoot(rootFlag)
	}
	current, err := cwd()
	if err != nil {
		return "", err
	}
	return gitutil.RepoRoot(current)
}

func loadEncryptionFilterKey(root string) (archivecrypt.Metadata, []byte, error) {
	meta, err := archivecrypt.LoadMetadata(root)
	if err != nil {
		return archivecrypt.Metadata{}, nil, err
	}
	key, err := archivecrypt.LoadLocalKey(root, meta.VaultID)
	if errors.Is(err, archivecrypt.ErrKeyMissing) {
		return archivecrypt.Metadata{}, nil, fmt.Errorf("%w: encrypted Backlot archive is locked; run backlot unlock", err)
	}
	if err != nil {
		return archivecrypt.Metadata{}, nil, err
	}
	return meta, key, nil
}

func encryptionFilterSpec(command string) *chomp.Spec {
	return chomp.New("backlot", command).
		String("root", chomp.ValueName("path"), chomp.Description("Backlot root path")).
		String("path", chomp.ValueName("path"), chomp.Required(), chomp.Description("archive-relative path")).
		Positionals(0, 0)
}

func printEncryptUsage(w io.Writer) {
	printSpecUsage(w, encryptionFilterSpec("encrypt"))
}

func printDecryptUsage(w io.Writer) {
	printSpecUsage(w, encryptionFilterSpec("decrypt"))
}
