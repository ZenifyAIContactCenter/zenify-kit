# zenify

Portable workspace toolkit for the Zenify polyrepo — a single self-contained Go binary
that ships the team's shared CLI: worktrees (`wt`), guards (`guard`, `secret-scan`),
onboarding (`up`), db access (`db-read`), plugin + skills sync (`skills`), and
observability (`observe`).

> **Status:** all capability milestones M0–M9 and the W0–W6 harness plan are shipped and
> released (current release: `zenify version`, or the GitHub Releases page). The
> milestone-by-milestone record lives in the team's private knowledge store (ROADMAP,
> handoffs), not in this repo. Team documentation: `website/` in this repo (VitePress) — the
> hosted URL is added here once Cloudflare Pages is connected. `zenify --help` lists every
> command.

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
The installer adds itself to your `PATH` (shell profile on macOS/Linux, user PATH on Windows), installs the GitHub CLI when missing, and wires the znf skills.

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

## After install

Three commands, in this order:

```sh
curl -fsSL https://raw.githubusercontent.com/ZenifyAIContactCenter/zenify-kit/main/scripts/install.sh | sh   # or the PowerShell line above
gh auth login
zenify up
```

Run `zenify up` from anywhere — on first run it asks where to put the workspace (default
`~/Developer/zenify` on macOS, `~/zenify` on Linux, `%USERPROFILE%\zenify` on Windows) and
whether you already have clones of the team repos to bring in. Later runs of `zenify up`,
`zenify doctor`, `zenify db-read`, `zenify docs sync` and `zenify config` find that workspace
on their own via `~/.zenify/workspace`.

### Staying current

At session start the kit checks for a newer release (at most once a day) and prints one line
when it finds one:

```
zenify: v0.18.0 is available (running 0.17.4) — upgrade: brew upgrade --cask zenify
```

Run `zenify update` to perform the upgrade for the detected install method (brew cask, scoop,
or the install script); `zenify update --check` only reports, without upgrading. Set
`ZENIFY_NO_UPDATE_CHECK=1` to silence the session-start nudge (`zenify update --check` still
checks on demand), and `ZENIFY_UPDATE_URL` to override the release lookup URL (useful for
testing against a fork or a staged release).

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
