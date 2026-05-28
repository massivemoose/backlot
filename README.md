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

Build from source:

```sh
go build -o backlot .
```

Install into your Go binary path:

```sh
go install .
```

## Limitations

- macOS/Linux first.
- No encryption yet.
- No daemon.
- No hosted sync.
- Requires Git.
- Requires a Git remote origin for MVP.
