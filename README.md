# Backlot

Private workspace for public repos.

Backlot gives every Git repo a local `.backlot/` directory for notes, LLM
state, scratch files, prompts, local scripts, and project memory without
committing any of it to the public repository.

One private state repo. Many public projects. No nested Git repo sprawl. No
private blobs in public history.

## Problem

Developers increasingly keep private project context beside code: notes, LLM
state, prompts, scratch files, local scripts, roadmap drafts, and unreleased
planning. In public repos, committing those files is unsafe, but creating one
nested private repo per project becomes annoying and hard to maintain.

## Approach

Backlot creates a local `.backlot/` symlink inside each public repo. The symlink
points into one central private state repo, defaulting to `~/.backlot`. Backlot
adds `.backlot/` to `.git/info/exclude`, so the public repo ignores it locally
without changing tracked files.

## Example

```sh
backlot init --remote git@github.com:you/backlot-state.git
cd ~/code/my-public-repo
backlot attach
```

Backlot creates:

```txt
~/code/my-public-repo/.backlot -> ~/.backlot/github.com/you/my-public-repo
```

and adds `.backlot/` to `.git/info/exclude`.

Your private files stay local to your machine and sync through your own private
state repo.

## Commands

```sh
backlot init [--root PATH] [--remote URL]
backlot attach [--root PATH] [--link-name .backlot]
backlot status [--root PATH]
backlot sync [--root PATH] [-m MESSAGE]
backlot protect
backlot doctor [--root PATH]
backlot version
```

Backlot root resolution order:

1. `--root`
2. `BACKLOT_ROOT`
3. `~/.backlot`

## Safety Model

- Backlot does not write `.gitignore`.
- Backlot does not commit to your public repo.
- Backlot does not stage files in your public repo.
- Backlot only syncs private state when you run `backlot sync`.
- Private state is stored in your own local/private Git repo.

## Install

Homebrew:

```sh
brew install massivemoose/tap/backlot
```

Manual download:

1. Download the archive for your OS and architecture from the GitHub Releases page.
2. Verify it against `checksums.txt`.
3. Place the `backlot` binary somewhere on your `PATH`.

Build from source:

```sh
go build -o backlot .
```

Install into your Go binary path:

```sh
go install .
```

## Releases

Backlot uses semantic version tags like `v0.1.0`. A tagged release builds
macOS and Linux binaries with GoReleaser and publishes archives, checksums, and
release notes to GitHub Releases.

For v0 releases:

- Patch versions are for bug fixes, docs, release pipeline fixes, and small polish.
- Minor versions are for new user-facing commands or workflows.
- `v1.0.0` waits until the core workflows are stable: init, attach, sync,
  clone/new-machine, and release/install docs.

The Homebrew tap is expected at `massivemoose/homebrew-tap`. Publishing to the
tap requires a `TAP_GITHUB_TOKEN` secret with write access to that repository.

## Limitations

- macOS/Linux first.
- No encryption yet.
- No daemon.
- No hosted sync.
- Requires Git.
- Requires a Git remote origin for MVP.
