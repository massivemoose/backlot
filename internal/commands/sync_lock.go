package commands

import "errors"

var errSyncBusy = errors.New("another Backlot sync is already running")

type syncLockRelease func()
