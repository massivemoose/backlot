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
adds local exclude entries for `.backlot` to `.git/info/exclude`, so the public
repo ignores it locally without changing tracked files.

## Example

```sh
backlot init --remote git@github.com:you/backlot-archive.git
cd ~/code/my-public-repo
backlot attach
```

Backlot creates:

```txt
~/code/my-public-repo/.backlot -> ~/.backlot/github.com/you/my-public-repo
```

and adds `.backlot` to `.git/info/exclude`.

Your private files stay local to your machine and sync through your own private
state repo.

For normal setup, you should not need to enter `~/.backlot` directly. Create a
private `backlot-archive` repo, initialize Backlot with its remote URL, and run
`backlot sync` when you want to publish private state:

```sh
backlot init --remote git@github.com:you/backlot-archive.git
backlot sync -m "Initial Backlot archive"
```

If you have an existing `backlot-archive` repo, you can easily `clone` it to
another machine and then `attach` it to whatever repos you're working on. If you
already have things in your `backlot-archive` for a repo then cloning will let
you pick up where you left off:

```sh
backlot clone git@github.com:you/backlot-archive.git
cd ~/code/my-public-repo
backlot attach
```

## Commands

```sh
backlot init [--root PATH] [--remote URL]
backlot clone <archive-url> [--root PATH]
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
macOS and Linux binaries with GoReleaser and publishes archives, checksums,
release notes, and a Homebrew formula.

Maintainers can publish a release from the commit currently at `origin/main`
with:

```sh
scripts/release v0.1.2
```

The release helper verifies the working tree, runs tests, runs a local
GoReleaser snapshot, creates an annotated tag, and pushes the tag to trigger the
GitHub Actions release workflow.

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
