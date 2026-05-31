# Backlot

Your private workspace for public repos.

Backlot gives each project repo a local `.backlot/` directory for private
notes, agent state, prompts, drafts, scratch files, and local scripts without
putting that material in the project repo's Git history.

One private archive. Many projects. No nested Git repo sprawl.

> Demo GIF coming soon.

## Contents

- [Why Backlot?](#why-backlot)
- [Install](#install)
- [Quickstart](#quickstart)
- [How It Works](#how-it-works)
- [Directory Layout](#directory-layout)
- [Using Backlot With LLMs And Agents](#using-backlot-with-llms-and-agents)
- [Commands](#commands)
- [Safety Model](#safety-model)
- [Cleanup](#cleanup)
- [Limitations](#limitations)
- [FAQ](#faq)

## Why Backlot?

Project context tends to collect beside the code: notes, LLM memory, prompts,
roadmap drafts, local scripts, experiments, and half-formed thoughts. That
context is useful precisely because it lives near the work, but it should not
end up in the repo's history by accident.

Backlot keeps that private workspace close to the project while storing it in a
separate Git repo you control. It works for open source repos, private company
repos, and anything that might become public later.

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

## Quickstart

Start locally:

```sh
backlot init
cd ~/code/my-project
backlot attach
```

Your private workspace now lives at `.backlot/` inside the project repo and is
stored under `~/.backlot`.

To back it up or use it across machines, create a private repo named
`backlot-archive` on GitHub or your Git host, then add it as the archive remote:

```sh
backlot init --remote git@github.com:you/backlot-archive.git
backlot sync -m "Initial Backlot archive"
```

If you want to work on another machine, you can easily clone the existing archive
and attach your project repo again:

```sh
backlot clone git@github.com:you/backlot-archive.git
cd ~/code/my-project
backlot attach
```

For normal setup, you should not need to modify `~/.backlot` directly.

## How It Works

Backlot creates a `.backlot` symlink in the repo you are working in:

```txt
~/code/my-project/.backlot -> ~/.backlot/github.com/you/my-project
```

The target lives inside one central private archive, defaulting to `~/.backlot`.
Backlot adds local ignore entries for `.backlot` to `.git/info/exclude`, so the
project repo ignores the private workspace without changing tracked files.

The archive path is based on the repo's `origin` remote. For example:

```txt
git@github.com:you/my-project.git
=> github.com/you/my-project
```

## Directory Layout

The first time Backlot creates a private workspace for a project, it looks like
this:

```txt
~/code/my-project/
  README.md
  src/
  .backlot -> ~/.backlot/github.com/you/my-project

~/.backlot/
  github.com/you/my-project/
    notes.md
    llm/
    scratch/
```

`notes.md`, `llm/`, and `scratch/` are starter files only. Rename them, delete
them, or add your own structure. Backlot does not enforce a layout inside a
project's private workspace, and later `backlot attach` runs will not recreate
starter files you removed.

## Using Backlot With LLMs And Agents

Backlot is useful as a local memory space for coding agents. Add something like
this to `AGENTS.md`, `CLAUDE.md`, or your tool's project instructions:

```md
Use `.backlot/` for private project context, notes, drafts, prompts, and agent state.
Read `.backlot/notes.md` and relevant files under `.backlot/llm/` when starting work.
Do not copy private `.backlot/` content into commits, PRs, issues, or public docs unless explicitly asked.
```

One simple layout:

```txt
.backlot/
  notes.md
  roadmap.md
  llm/
    agent_state.md
    prompts.md
  scratch/
    experiments.md
    local-scripts/
```

The structure is yours. Backlot only provides the private place to keep it.

## Commands

```sh
backlot init [--root PATH] [--remote URL]
backlot clone <archive-url> [--root PATH]
backlot attach [--root PATH] [--link-name .backlot]
backlot detach [--root PATH]
backlot status [--root PATH]
backlot sync [--root PATH] [-m MESSAGE]
backlot protect
backlot doctor [--root PATH]
backlot version
```

- `init` creates or configures the local Backlot archive.
- `clone` clones an existing Backlot archive on a new machine.
- `attach` creates `.backlot` for the current repo.
- `detach` removes the current repo's Backlot symlink and local exclude entries.
- `status` shows the current repo's Backlot state.
- `sync` commits and pushes the private archive.
- `protect` installs a local pre-commit guard for `.backlot`.
- `doctor` diagnoses setup issues.
- `version` prints build metadata.

Backlot root resolution order:

1. `--root`
2. `BACKLOT_ROOT`
3. `~/.backlot`

## Safety Model

- Backlot writes local ignore rules to `.git/info/exclude`, not `.gitignore`.
- Backlot does not commit to your project repo.
- Backlot does not stage files in your project repo.
- Backlot does not push from your project repo.
- Backlot only syncs private archive contents when you run `backlot sync`.
- Backlot does not encrypt files.

### Why not `.gitignore`?

`.gitignore` is usually tracked. Writing to it would mutate the project repo and
could leak Backlot-specific setup into shared history.

`.git/info/exclude` is local to your clone. Backlot uses it so `.backlot` stays
ignored on your machine without changing files that belong to the project.

### Privacy

Private files stay local until you run `backlot sync`. When you sync, Backlot
commits and pushes the contents of your Backlot archive to the `origin` remote
configured for that archive. Use a private remote for anything sensitive.

## Cleanup

To disconnect Backlot from a repo, run:

```sh
backlot detach
```

This removes the managed `.backlot` symlink from the current repo and removes
Backlot's local exclude entries from `.git/info/exclude`. It does not delete
your private archive or any project notes.

If you intentionally want to remove the entire private archive from your
machine, delete `~/.backlot` yourself after detaching the repos you care about.
Deleting `~/.backlot` does not automatically clean up `.backlot` symlinks in
attached repos; those links become broken until you remove them.

## Limitations

- macOS and Linux are supported.
- Windows is not currently supported.
- No encryption yet.
- No daemon.
- No hosted sync.
- Requires Git.
- Requires a Git remote `origin` for project repos in the MVP.

## FAQ

### Can I use Backlot with private repos?

Yes. Backlot is useful for any repo where you want private notes or agent state
beside the code without putting that context in the repo's history.

### Does Backlot commit to my project repo?

No. Backlot never runs `git add`, `git commit`, or `git push` in your project
repo. `backlot sync` runs Git commands inside the Backlot archive.

### What if `.backlot` already exists?

Backlot refuses to overwrite a `.backlot` file, directory, or symlink it does
not manage. Move the existing path or choose a different attach name with
`backlot attach --link-name`.

### Can I use multiple Backlot archives?

Yes. Use `--root PATH` for one command or set `BACKLOT_ROOT` for a shell session.

### Can I choose my own structure inside `.backlot`?

Yes. Backlot creates a few starter files only when it creates a project
workspace for the first time, but it does not enforce their layout.
