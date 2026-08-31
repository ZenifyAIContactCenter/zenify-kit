# zenify

Portable workspace toolkit for the Zenify polyrepo — a single self-contained Go binary
that ships the team's shared CLI (`version`, `doctor`, and more to come).

> **Status:** M0 Foundation. The commands available today are `version` and `doctor`.
> Onboarding, gate, and managed-file subcommands land in later milestones.

The binary is published for macOS, Linux, and Windows (amd64 + arm64) on every release.
The source contains no workspace-specific data (see [ARCHITECTURE.md](ARCHITECTURE.md)),
which is why the repository and its binaries are public and install with no authentication.

### Homebrew (macOS + Linux)

```sh
brew tap zenifyaicontactcenter/zenify-kit https://github.com/ZenifyAIContactCenter/zenify-kit
brew trust zenifyaicontactcenter/zenify-kit   # one-time: Homebrew requires trusting a third-party cask tap
brew install --cask zenify
```

Upgrade later with `brew upgrade --cask zenify`.

### Scoop (Windows)

```powershell
scoop bucket add zenify https://github.com/ZenifyAIContactCenter/zenify-kit
scoop install zenify
```

### Install script (macOS / Linux)

```sh
curl -fsSL https://raw.githubusercontent.com/ZenifyAIContactCenter/zenify-kit/main/scripts/install.sh | sh
```

Installs to `~/.local/bin/zenify`. Override with `ZENIFY_BIN=/usr/local/bin`; pin a version
with `ZENIFY_VERSION=v0.2.0`. Windows equivalent: `scripts/install.ps1`.

### Manual download

Grab the archive for your OS/arch from the
[Releases page](https://github.com/ZenifyAIContactCenter/zenify-kit/releases), extract, and
put `zenify` (or `zenify.exe`) somewhere on your `PATH`.

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
