#!/bin/sh
set -eu
REPO="ZenifyAIContactCenter/zenify-kit"

# This repo is private: release assets require GitHub auth. We use the GitHub
# CLI (gh) rather than a raw token so each teammate reuses their own gh login.
if ! command -v gh >/dev/null 2>&1; then
  echo "zenify: 'gh' (GitHub CLI) is required — install from https://cli.github.com" >&2
  exit 1
fi
if ! gh auth status >/dev/null 2>&1; then
  echo "zenify: not logged in to GitHub — run 'gh auth login' first" >&2
  exit 1
fi

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "unsupported arch: $arch" >&2; exit 1 ;;
esac

ver="${ZENIFY_VERSION:-}"
if [ -z "$ver" ]; then
  ver=$(gh release view --repo "$REPO" --json tagName --jq .tagName)
fi
if [ -z "$ver" ]; then
  echo "zenify: could not resolve a release version (no releases yet?)" >&2
  exit 1
fi

asset="zenify_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
echo "Downloading $asset ($ver) via gh"
gh release download "$ver" --repo "$REPO" --pattern "$asset" --dir "$tmp" --clobber
tar -xz -C "$tmp" -f "$tmp/$asset"

dest="${ZENIFY_BIN:-$HOME/.local/bin}"
mkdir -p "$dest"
mv "$tmp/zenify" "$dest/zenify"
chmod +x "$dest/zenify"
rm -rf "$tmp"
echo "Installed zenify to $dest/zenify"
"$dest/zenify" version
