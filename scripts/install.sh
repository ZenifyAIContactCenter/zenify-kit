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

ver="${ZENIFY_VERSION:-latest}"
if [ "$ver" = latest ]; then
  ver=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
        | grep '"tag_name"' | head -1 | cut -d'"' -f4)
fi
if [ -z "$ver" ]; then
  echo "zenify: could not resolve the latest release version" >&2
  exit 1
fi

url="https://github.com/$REPO/releases/download/$ver/zenify_${os}_${arch}.tar.gz"
tmp=$(mktemp -d)
echo "Downloading $url"
curl -fsSL "$url" | tar -xz -C "$tmp"

dest="${ZENIFY_BIN:-$HOME/.local/bin}"
mkdir -p "$dest"
mv "$tmp/zenify" "$dest/zenify"
chmod +x "$dest/zenify"
rm -rf "$tmp"
echo "Installed zenify to $dest/zenify"
"$dest/zenify" version
