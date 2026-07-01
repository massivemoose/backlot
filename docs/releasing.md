# Releasing Backlot

This document is for Backlot maintainers. User-facing install instructions live
in the README.

## Release Pipeline

Backlot uses semantic version tags like `v0.1.0`. A tagged release builds macOS
and Linux binaries with GoReleaser and publishes archives, checksums, release
notes, and a Homebrew formula.

GitHub Releases are the canonical distribution target. The Homebrew tap is
expected at `massivemoose/homebrew-tap`.

Publishing to the tap requires a `TAP_GITHUB_TOKEN` secret with write access to
that repository.

## Maintainer Release Flow

Publish a release from the commit currently at `origin/main`:

```sh
scripts/release v0.1.2
```

Before tagging, update `CHANGELOG.md` so the `Unreleased` section is moved
under the version being tagged.

The release helper:

1. Verifies the working tree is clean.
2. Fetches tags and `origin/main`.
3. Verifies `HEAD` matches `origin/main`.
4. Runs tests.
5. Runs a local GoReleaser snapshot.
6. Creates an annotated tag.
7. Pushes the tag to trigger the GitHub Actions release workflow.

## Versioning Policy

For v0 releases:

- Patch versions are for bug fixes, docs, release pipeline fixes, and small polish.
- Minor versions are for new user-facing commands or workflows.
- `v1.0.0` waits until the core workflows are stable: init, attach, sync,
  clone/new-machine, detach/cleanup, and release/install docs.

Version selection remains manual for now. The release script automates checks,
tagging, and publishing triggers; it does not decide the next version.

## Local Checks

Run the test suite:

```sh
GOCACHE=/tmp/backlot-go-cache GOMODCACHE=/tmp/backlot-go-mod-cache go test ./...
```

Run a local GoReleaser snapshot:

```sh
TAP_GITHUB_TOKEN=${TAP_GITHUB_TOKEN:-} goreleaser release --snapshot --clean
```

`goreleaser check` may fail on the deprecated `brews` configuration in newer
GoReleaser versions even when snapshot generation succeeds. Backlot currently
uses a generated Homebrew formula rather than a cask to avoid unsigned macOS
cask Gatekeeper friction for the v0 CLI path.
