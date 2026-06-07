//go:build windows

package commands

import (
	"errors"
	"os"

	"github.com/massivemoose/backlot/internal/gitutil"
)

func acquireSyncLock(root string) (syncLockRelease, error) {
	lockPath, err := gitutil.GitPath(root, "backlot-sync.lock")
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil, errSyncBusy
	}
	if err != nil {
		return nil, err
	}
	return func() {
		_ = file.Close()
		_ = os.Remove(lockPath)
	}, nil
}
