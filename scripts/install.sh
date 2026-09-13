#!/bin/sh
set -eu
REPO="ZenifyAIContactCenter/zenify-kit"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "unsupported arch: $arch" >&2; exit 1 ;;
esac

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

if [ -n "${ZENIFY_INSTALL_FROM:-}" ]; then
  # test-only, undocumented: install from a local tarball, skip download + checksum
  ver="local"
  tar -xzf "$ZENIFY_INSTALL_FROM" -C "$tmp"
else
  ver="${ZENIFY_VERSION:-latest}"
  if [ "$ver" = latest ]; then
    ver=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
          | grep '"tag_name"' | head -1 | cut -d'"' -f4)
  fi
  if [ -z "$ver" ]; then
    echo "zenify: could not resolve the latest release version" >&2
    exit 1
  fi

  asset="zenify_${os}_${arch}.tar.gz"
  base="https://github.com/$REPO/releases/download/$ver"

  echo "Downloading $base/$asset"
  curl -fsSL "$base/$asset" -o "$tmp/$asset"
  curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"

  # Verify the download against the release's signed checksums.txt before
  # unpacking anything. A corrupted or tampered asset must never be installed.
  if command -v sha256sum >/dev/null 2>&1; then
    got=$(sha256sum "$tmp/$asset" | cut -d' ' -f1)
  elif command -v shasum >/dev/null 2>&1; then
    got=$(shasum -a 256 "$tmp/$asset" | cut -d' ' -f1)
  else
    echo "zenify: no sha256 tool (sha256sum or shasum) found to verify the download" >&2
    exit 1
  fi
  want=$(awk -v a="$asset" '$2 == a { print $1 }' "$tmp/checksums.txt")
  if [ -z "$want" ]; then
    echo "zenify: $asset not listed in checksums.txt — cannot verify" >&2
    exit 1
  fi
  if [ "$got" != "$want" ]; then
    echo "zenify: checksum mismatch for $asset — refusing to install" >&2
    echo "  expected $want" >&2
    echo "  got      $got" >&2
    exit 1
  fi
  echo "Checksum verified ($asset)"

  tar -xzf "$tmp/$asset" -C "$tmp"
fi

dest="${ZENIFY_BIN:-$HOME/.local/bin}"
mkdir -p "$dest"
mv "$tmp/zenify" "$dest/zenify"
chmod +x "$dest/zenify"

"$dest/zenify" version >/dev/null 2>&1 || true
installed_ver=$("$dest/zenify" version 2>/dev/null | awk '{print $NF}')
[ -n "$installed_ver" ] || installed_ver="$ver"

# --- PATH: write it ourselves (FR-1.1), idempotent via the marker comment ---
path_line="export PATH=\"$dest:\$PATH\" # zenify-kit"
case ":$PATH:" in
  *":$dest:"*) ;;
  *)
    profile=""
    case "${SHELL:-}" in
      */zsh)  profile="$HOME/.zshrc" ;;
      */bash) if [ -f "$HOME/.bashrc" ]; then profile="$HOME/.bashrc"; else profile="$HOME/.profile"; fi ;;
    esac
    if [ -n "$profile" ]; then
      if ! grep -qF '# zenify-kit' "$profile" 2>/dev/null; then
        printf '\n%s\n' "$path_line" >> "$profile"
        echo "Added $dest to PATH in $profile (takes effect in new shells)"
      fi
    else
      echo "warning: $dest is not on your PATH and your shell was not recognised."
      echo "  add this line to your shell profile, then restart the shell:"
      echo "    $path_line"
    fi
    ;;
esac
export PATH="$dest:$PATH"   # FR-1.2: this script's own remaining steps

# --- znf onboarding bootstrap (fail-open) ---
# 1) ensure gh is present (device-flow login in `zenify up` needs it)
if ! command -v gh >/dev/null 2>&1; then
  if command -v brew >/dev/null 2>&1; then
    echo "Installing GitHub CLI (gh) via brew..."
    brew install gh || echo "note: 'brew install gh' failed — install gh manually: https://cli.github.com"
  else
    echo "note: GitHub CLI (gh) not found and no brew detected — install gh: https://cli.github.com"
  fi
fi
# 2) wire znf skills + hooks
echo "Wiring znf skills + hooks..."
"$dest/zenify" skills sync || echo "note: 'zenify skills sync' skipped (run it manually later)"

echo "Installed zenify $installed_ver"
echo "Next: zenify up"
