//go:build darwin || linux || freebsd || openbsd || netbsd

package commands

import (
	"errors"
	"os"
	"syscall"

	"github.com/massivemoose/backlot/internal/gitutil"
)

func acquireSyncLock(root string) (syncLockRelease, error) {
	lockPath, err := gitutil.GitPath(root, "backlot-sync.lock")
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errSyncBusy
		}
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}
