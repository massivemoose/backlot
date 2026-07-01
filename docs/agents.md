# Using Backlot With Coding Agents

Use this when a coding agent can edit your project but asks for permission
before reading or writing `.backlot/`. Backlot can preview and, for simple
configs, apply the persistent sandbox grants for Codex CLI and Claude Code.

Backlot exposes private project state through a `.backlot` symlink inside your
project repo:

```txt
~/code/my-project/.backlot -> ~/.backlot/github.com/you/my-project
```

Some coding agents enforce filesystem permissions on the resolved symlink
target, not just the `.backlot` path inside the repo. If an agent can read or
edit normal project files but asks for permission when touching `.backlot/`, add
your Backlot archive root to that agent's trusted workspace directories.

The recommended grant is the archive root, usually `~/.backlot`, because that
works for every project attached to the same Backlot archive.

Find your current Backlot root:

```sh
backlot status
```

Or run:

```sh
backlot agents setup
```

## Codex CLI

For one session:

```sh
codex --cd /path/to/project --add-dir ~/.backlot
```

For persistent setup, prefer a named permission profile in
`~/.codex/config.toml`:

```toml
default_permissions = "workspace-backlot"

[permissions.workspace-backlot.workspace_roots]
"/Users/YOU/.backlot" = true

[permissions.workspace-backlot.filesystem]
":minimal" = "read"
":tmpdir" = "write"
":slash_tmp" = "write"

[permissions.workspace-backlot.filesystem.":workspace_roots"]
"." = "write"
".git" = "read"
".codex" = "read"
".agents" = "read"
```

Older Codex configs may use `[sandbox_workspace_write]` with `writable_roots`.
Backlot still recognizes that as a valid grant, but new config should use the
permission profile form.

Backlot can preview the Codex setup:

```sh
backlot agents setup --tool codex
```

Backlot can also apply the persistent Codex config when the file is simple
enough to edit safely:

```sh
backlot agents setup --tool codex --apply
```

If Backlot refuses to edit the config, use the snippet it prints and paste it
manually.

## Claude Code

For one session:

```sh
claude --add-dir ~/.backlot
```

For persistent setup, add this to `~/.claude/settings.json`:

```json
{
  "permissions": {
    "additionalDirectories": [
      "/Users/YOU/.backlot"
    ]
  }
}
```

If your settings already have a `permissions` object, add only the
`additionalDirectories` entry inside it.

Backlot can preview the Claude setup:

```sh
backlot agents setup --tool claude
```

Backlot can also apply the persistent Claude config:

```sh
backlot agents setup --tool claude --apply
```

## Manual Validation

Start a fresh agent session after changing persistent settings so the agent
reloads its config. Then ask it:

```txt
Please create `.backlot/plans/agent-permission-test.md` with the text
`This agent can write through the Backlot symlink.` Then read the file back.
Do not edit any public repo files.
```

Success means the agent writes and reads the file without a filesystem
permission prompt. Your public repo should remain clean because `.backlot` is
ignored locally.
