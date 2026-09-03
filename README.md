# zenify

Portable workspace toolkit for the Zenify polyrepo — a single self-contained Go binary
that ships the team's shared CLI: worktrees (`wt`), guards (`guard`, `secret-scan`),
onboarding (`up`), db access (`db-read`), plugin + skills sync (`skills`), and
observability (`observe`).

> **Status:** M0–M3 shipped. Run `zenify --help` for the full command list, and
> `zenify <command> --help` for any command's flags and behaviour.

The binary is published for macOS, Linux, and Windows (amd64 + arm64) on every release.
The source contains no workspace-specific data (see [ARCHITECTURE.md](ARCHITECTURE.md)),
which is why the repository and its binaries are public and install with no authentication.

## Install

### Install script (recommended)

A one-line installer — no package manager required. It verifies the download against the
release SHA-256 checksums before installing, and (on macOS) avoids the Gatekeeper quarantine
that can otherwise block a freshly downloaded binary.

**macOS / Linux**

```sh
curl -fsSL https://raw.githubusercontent.com/ZenifyAIContactCenter/zenify-kit/main/scripts/install.sh | sh
```

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/ZenifyAIContactCenter/zenify-kit/main/scripts/install.ps1 | iex
```

Installs to `~/.local/bin/zenify` (macOS / Linux) or `%LOCALAPPDATA%\Programs\zenify`
(Windows). Override the location with `ZENIFY_BIN`; pin a version with `ZENIFY_VERSION=v0.5.0`.
The installer warns if the target directory is not on your `PATH`.

### Alternatives

<details>
<summary>Homebrew (macOS + Linux)</summary>

```sh
brew tap zenifyaicontactcenter/zenify-kit https://github.com/ZenifyAIContactCenter/zenify-kit
brew trust zenifyaicontactcenter/zenify-kit   # one-time: Homebrew requires trusting a third-party cask tap
brew install --cask zenify
```

Upgrade later with `brew upgrade --cask zenify`.
</details>

<details>
<summary>Scoop (Windows)</summary>

```powershell
scoop bucket add zenify https://github.com/ZenifyAIContactCenter/zenify-kit
scoop install zenify
```
</details>

<details>
<summary>Manual download</summary>

Grab the archive for your OS/arch from the
[Releases page](https://github.com/ZenifyAIContactCenter/zenify-kit/releases), extract, and
put `zenify` (or `zenify.exe`) somewhere on your `PATH`.
</details>

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
