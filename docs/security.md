# Backlot Security Model

Backlot keeps private project workspace files out of the public project repo.
It can also encrypt archive Git blobs before they are pushed to the archive
remote.

## Protected

- Backlot writes `.backlot/` ignore rules to the project repo's local
  `.git/info/exclude`, not tracked `.gitignore`.
- Backlot sync commands run in the private archive repo, not the project repo.
- With archive encryption enabled, private file contents are stored as
  encrypted Git blobs in the archive remote.
- Locked encrypted archives fail closed before sync when the local key or
  encryption filters are not ready.

## Not Protected

Backlot does not protect against a compromised local machine or a malicious
process running as your user.

Archive encryption does not hide Git metadata. Remotes, filenames, tree shape,
commit history, commit messages, timestamps, and approximate object sizes
remain visible.

`backlot lock` does not rewrite old history. Archive commits made before
encryption was enabled may still contain plaintext.

## Encryption Metadata

Backlot stores encryption metadata in `.backlot-encryption.json`. The metadata
is not secret. The local key and recovery key are secret.

## Local Machine Access

Unlocked worktrees stay plaintext so editors, shells, and coding agents can use
ordinary files. Any local tool with access to the Backlot workspace can read
those plaintext files.

## Recovery Keys

`backlot lock` prints a recovery key once. Store it somewhere safe. If the
local key store is gone and the recovery key is lost, Backlot cannot recover
encrypted archive contents.

