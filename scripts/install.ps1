$ErrorActionPreference = 'Stop'
$repo = 'ZenifyAIContactCenter/zenify-kit'

# Private repo: release assets require GitHub auth. Use the GitHub CLI (gh) so
# each teammate reuses their own gh login instead of a raw token.
if (-not (Get-Command gh -ErrorAction SilentlyContinue)) {
  Write-Error "zenify: 'gh' (GitHub CLI) is required - install from https://cli.github.com"
}
gh auth status *> $null
if ($LASTEXITCODE -ne 0) {
  Write-Error "zenify: not logged in to GitHub - run 'gh auth login' first"
}

$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$ver = if ($env:ZENIFY_VERSION) { $env:ZENIFY_VERSION } else {
  gh release view --repo $repo --json tagName --jq .tagName
}
if (-not $ver) { Write-Error "zenify: could not resolve a release version (no releases yet?)" }

$asset = "zenify_windows_$arch.zip"
$tmp = Join-Path $env:TEMP "zenify-$([guid]::NewGuid())"
New-Item -ItemType Directory -Path $tmp | Out-Null
gh release download $ver --repo $repo --pattern $asset --dir $tmp --clobber
Expand-Archive (Join-Path $tmp $asset) -DestinationPath $tmp

$dest = Join-Path $env:LOCALAPPDATA 'Programs\zenify'
New-Item -ItemType Directory -Force -Path $dest | Out-Null
Move-Item -Force (Join-Path $tmp 'zenify.exe') (Join-Path $dest 'zenify.exe')
Remove-Item -Recurse -Force $tmp
Write-Host "Installed zenify to $dest"
& (Join-Path $dest 'zenify.exe') version
