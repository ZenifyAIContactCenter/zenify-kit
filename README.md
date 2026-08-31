# zenify

Portable workspace toolkit for the Zenify polyrepo — a single self-contained Go binary
that ships the team's shared CLI (`version`, `doctor`, and more to come).

> **Status:** M0 Foundation. The commands available today are `version` and `doctor`.
> Onboarding, gate, and managed-file subcommands land in later milestones.

## Install

The binary is published for macOS, Linux, and Windows (amd64 + arm64) on every release.

### macOS / Linux

```sh
curl -fsSL https://raw.githubusercontent.com/ZenifyAIContactCenter/zenify-kit/main/scripts/install.sh | sh
```

Installs to `~/.local/bin/zenify` by default. Override with `ZENIFY_BIN=/usr/local/bin`,
or pin a version with `ZENIFY_VERSION=v0.1.0`. Make sure the install dir is on your `PATH`.

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/ZenifyAIContactCenter/zenify-kit/main/scripts/install.ps1 | iex
```

Installs to `%LOCALAPPDATA%\Programs\zenify`. Pin a version with `$env:ZENIFY_VERSION = 'v0.1.0'`.

### Manual download

Grab the archive for your OS/arch from the
[Releases page](https://github.com/ZenifyAIContactCenter/zenify-kit/releases), extract, and
put `zenify` (or `zenify.exe`) somewhere on your `PATH`.

### Package managers (coming soon)

`brew` (macOS + Linux), `scoop` and `winget` (Windows) are configured but not yet live —
they need their tap/bucket repositories created first. Until then, use the install scripts above.

## Usage

```sh
zenify --version    # print the binary version
zenify version      # same, as a subcommand
zenify doctor       # run environment diagnostics
```

## Build from source

Requires Go 1.27+.

```sh
go build -o zenify ./cmd/zenify
```

## Releasing

Releases are cut with [GoReleaser](https://goreleaser.com) on a `v*` tag. See
`.goreleaser.yaml` and `.github/workflows/release.yml`.
