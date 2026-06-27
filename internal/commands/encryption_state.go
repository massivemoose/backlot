package commands

import (
	"errors"
	"fmt"
	"os"
	"strings"

	archivecrypt "github.com/massivemoose/backlot/internal/encryption"
	"github.com/massivemoose/backlot/internal/gitutil"
)

const (
	encryptionDisabled      = "disabled"
	encryptionUnlocked      = "unlocked"
	encryptionLocked        = "locked"
	encryptionMisconfigured = "misconfigured"
)

type archiveEncryptionState struct {
	Status   string
	Problem  string
	Recovery string
	Err      error
}

func collectArchiveEncryptionState(root string) archiveEncryptionState {
	meta, err := archivecrypt.LoadMetadata(root)
	if errors.Is(err, os.ErrNotExist) {
		return archiveEncryptionState{Status: encryptionDisabled}
	}
	if err != nil {
		return archiveEncryptionState{
			Status:   encryptionMisconfigured,
			Problem:  "Backlot archive encryption is misconfigured",
			Recovery: fmt.Sprintf("inspect %s", archivecrypt.MetadataFile),
			Err:      err,
		}
	}
	key, err := archivecrypt.LoadLocalKey(root, meta.VaultID)
	if errors.Is(err, archivecrypt.ErrKeyMissing) {
		return archiveEncryptionState{
			Status:   encryptionLocked,
			Problem:  "Backlot archive encryption is locked",
			Recovery: fmt.Sprintf("backlot unlock --root %s --recovery-key-file PATH", root),
		}
	} else if err != nil {
		return archiveEncryptionState{
			Status:   encryptionMisconfigured,
			Problem:  "Backlot archive encryption is misconfigured",
			Recovery: fmt.Sprintf("backlot unlock --root %s --recovery-key-file PATH", root),
			Err:      err,
		}
	}
	if err := authenticateEncryptedWorktreeBlob(root, key); err != nil {
		return archiveEncryptionState{
			Status:   encryptionMisconfigured,
			Problem:  "Backlot archive encryption is misconfigured",
			Recovery: fmt.Sprintf("backlot unlock --root %s --recovery-key-file PATH", root),
			Err:      err,
		}
	}
	if !encryptionFiltersConfigured(root) {
		return archiveEncryptionState{
			Status:   encryptionMisconfigured,
			Problem:  "Backlot archive encryption is misconfigured",
			Recovery: fmt.Sprintf("backlot unlock --root %s", root),
		}
	}
	if !encryptionAttributesActive(root) {
		return archiveEncryptionState{
			Status:   encryptionMisconfigured,
			Problem:  "Backlot archive encryption is misconfigured",
			Recovery: fmt.Sprintf("backlot lock --root %s", root),
		}
	}
	return archiveEncryptionState{Status: encryptionUnlocked}
}

func encryptionFiltersConfigured(root string) bool {
	required, err := gitutil.RunGit(root, "config", "--get", "filter.backlot.required")
	if err != nil || strings.TrimSpace(required) != "true" {
		return false
	}
	clean, err := gitutil.RunGit(root, "config", "--get", "filter.backlot.clean")
	if err != nil || !strings.Contains(clean, " encrypt ") {
		return false
	}
	smudge, err := gitutil.RunGit(root, "config", "--get", "filter.backlot.smudge")
	return err == nil && strings.Contains(smudge, " decrypt ")
}

func encryptionAttributesActive(root string) bool {
	attrs, err := gitutil.RunGit(root, "check-attr", "filter", "diff", "--", "backlot-encryption-probe")
	if err != nil {
		return false
	}
	hasFilter := false
	hasDiff := false
	for _, line := range strings.Split(attrs, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, ": filter: backlot") {
			hasFilter = true
		}
		if strings.HasSuffix(line, ": diff: backlot") {
			hasDiff = true
		}
	}
	return hasFilter && hasDiff
}
