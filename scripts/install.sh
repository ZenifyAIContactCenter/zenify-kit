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

asset="zenify_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$ver"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

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

dest="${ZENIFY_BIN:-$HOME/.local/bin}"
mkdir -p "$dest"
mv "$tmp/zenify" "$dest/zenify"
chmod +x "$dest/zenify"
echo "Installed zenify to $dest/zenify"
"$dest/zenify" version

# Warn if the install dir is not on PATH, so the user does not hit a confusing
# "command not found" after a successful install.
case ":$PATH:" in
  *":$dest:"*) ;;
  *)
    echo ""
    echo "warning: $dest is not on your PATH."
    echo "  add this line to your shell profile (~/.zshrc or ~/.bashrc), then restart the shell:"
    echo "    export PATH=\"$dest:\$PATH\""
    ;;
esac
