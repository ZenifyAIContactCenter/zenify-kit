# zenify

Portable workspace toolkit for the Zenify polyrepo — a single self-contained Go binary
that ships the team's shared CLI (`version`, `doctor`, and more to come).

> **Status:** M0 Foundation. The commands available today are `version` and `doctor`.
> Onboarding, gate, and managed-file subcommands land in later milestones.

The binary is published for macOS, Linux, and Windows (amd64 + arm64) on every release.

> **This repository is private.** Release assets require GitHub authentication with access
> to the `ZenifyAIContactCenter` org. Unauthenticated `curl` / `irm` downloads return 404.
> The method below uses the [GitHub CLI](https://cli.github.com) (`gh`), which every
> teammate already has authenticated.

### macOS / Linux / Windows (via `gh`)

```sh
# darwin_arm64 | darwin_amd64 | linux_amd64 | linux_arm64 | windows_amd64 | windows_arm64
gh release download v0.1.0 --repo ZenifyAIContactCenter/zenify-kit \
  --pattern 'zenify_darwin_arm64.tar.gz'
tar xzf zenify_darwin_arm64.tar.gz          # or unzip the .zip on Windows
install -m 0755 zenify ~/.local/bin/zenify  # somewhere on your PATH
zenify --version
```

### Install scripts (`scripts/install.sh`, `install.ps1`)

These exist but assume **public** release assets — they do not authenticate, so they will
404 against this private repo as-is. A `gh`-authenticated installer is planned for the
onboarding milestone. Until then use the `gh release download` method above.

### Package managers (not yet live)

`brew` (macOS + Linux), `scoop` and `winget` (Windows) are configured in `.goreleaser.yaml`
but disabled. They need their tap/bucket repositories created first, and public package
catalogs (winget especially) do not accept a private tool — so these depend on the
distribution decision taken during onboarding.

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
