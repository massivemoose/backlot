# Contributing to Backlot

Thanks for your interest in contributing to Backlot.

Backlot is intentionally small and opinionated. Bug reports, documentation fixes, small usability improvements, and focused patches are very welcome. Large feature additions should start with an issue or discussion first so we can decide whether they fit the project.

## What belongs in Backlot

Backlot is a CLI for creating and managing a private project-local state archive. It is meant to stay:

- simple
- local-first
- Git-based
- easy to inspect
- useful for human and AI-assisted development workflows

Changes that make Backlot harder to understand, require heavyweight infrastructure, or add broad platform behavior may be declined even if they are well implemented.

## Good contributions

Good first contributions include:

- bug fixes
- clearer help text
- documentation improvements
- small CLI ergonomics improvements
- tests for existing behavior
- examples for common workflows

## Before opening a large PR

For larger changes, please open an issue first describing:

- the problem you are trying to solve
- the proposed behavior
- why it belongs in Backlot
- any tradeoffs or alternatives considered

This helps avoid wasted work.

## Development

Backlot is written in Go.

Common commands:

```sh
go test ./...
go fmt ./...
go vet ./...
```

Before submitting a PR, please make sure tests pass and the code is formatted.

## Commit style

No special commit format is required. Clear, descriptive commit messages are preferred.

Examples:

```text
fix clone remote validation
document agent workflow example
add tests for init config handling
```

## Pull requests

A good pull request should include:

- a clear description of the change
- why the change is needed
- tests when appropriate
- documentation updates when behavior changes

Small, focused PRs are easier to review than large mixed changes.

## Licensing

By contributing to Backlot, you agree that your contribution will be licensed under the Apache License 2.0.

You represent that you have the right to submit the contribution and that it does not knowingly include code or content that you do not have permission to contribute.

## Maintainer discretion

Backlot is maintained as an opinionated project. The maintainer may decline contributions that do not fit the project direction, even if they are technically sound.

That is not a judgment on the quality of the contribution; it is just how the project stays small and coherent.
