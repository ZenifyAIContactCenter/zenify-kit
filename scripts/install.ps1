$ErrorActionPreference = 'Stop'
$repo = 'ZenifyAIContactCenter/zenify-kit'

$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$ver = if ($env:ZENIFY_VERSION) { $env:ZENIFY_VERSION } else {
  (Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest").tag_name
}
if (-not $ver) { Write-Error "zenify: could not resolve a release version" }

$asset = "zenify_windows_$arch.zip"
$base = "https://github.com/$repo/releases/download/$ver"
$tmp = Join-Path $env:TEMP "zenify-$([guid]::NewGuid())"
New-Item -ItemType Directory -Path $tmp | Out-Null
$zip = Join-Path $tmp $asset
Invoke-WebRequest "$base/$asset" -OutFile $zip
$sumsFile = Join-Path $tmp 'checksums.txt'
Invoke-WebRequest "$base/checksums.txt" -OutFile $sumsFile

# Verify the download against the release checksums before unpacking anything.
$got = (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower()
$want = $null
foreach ($line in Get-Content $sumsFile) {
  $parts = $line -split '\s+'
  if ($parts.Length -ge 2 -and $parts[1] -eq $asset) { $want = $parts[0].ToLower() }
}
if (-not $want) { Write-Error "zenify: $asset not listed in checksums.txt - cannot verify" }
if ($got -ne $want) { Write-Error "zenify: checksum mismatch for $asset - refusing to install (expected $want, got $got)" }
Write-Host "Checksum verified ($asset)"

Expand-Archive $zip -DestinationPath $tmp

$dest = Join-Path $env:LOCALAPPDATA 'Programs\zenify'
New-Item -ItemType Directory -Force -Path $dest | Out-Null
Move-Item -Force (Join-Path $tmp 'zenify.exe') (Join-Path $dest 'zenify.exe')
Remove-Item -Recurse -Force $tmp
Write-Host "Installed zenify to $dest"
$exe = Join-Path $dest 'zenify.exe'
try {
  $installedVer = (& $exe version 2>$null | Select-Object -Last 1) -replace '^.*\s', ''
} catch { $installedVer = $null }
if (-not $installedVer) { $installedVer = $ver }

# --- PATH: User scope only (FR-1.3). Read the User value back, never $env:Path,
# so Machine PATH entries are not copied into the User PATH.
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($userPath -split ';') -notcontains $dest) {
  try {
    $newUserPath = if ([string]::IsNullOrEmpty($userPath)) { $dest } else { "$userPath;$dest" }
    [Environment]::SetEnvironmentVariable('Path', $newUserPath, 'User')
    Write-Host "Added $dest to your user PATH (takes effect in new shells)"
  } catch {
    Write-Host "warning: could not update the user PATH: $_"
    Write-Host "  add it with (then restart the shell):"
    Write-Host "    [Environment]::SetEnvironmentVariable('Path', `"`$([Environment]::GetEnvironmentVariable('Path','User'));$dest`", 'User')"
  }
}
if (($env:Path -split ';') -notcontains $dest) { $env:Path = "$env:Path;$dest" }

# --- znf onboarding bootstrap (fail-open, mirrors install.sh) ---
if (-not (Get-Command gh -ErrorAction SilentlyContinue)) {
  if (Get-Command winget -ErrorAction SilentlyContinue) {
    Write-Host "Installing GitHub CLI (gh) via winget..."
    try { winget install --id GitHub.cli -e --silent --accept-source-agreements --accept-package-agreements | Out-Null }
    catch { Write-Host "note: winget install failed - install gh manually: https://cli.github.com" }
  } else {
    Write-Host "note: GitHub CLI (gh) not found and no winget - install gh: https://cli.github.com"
  }
}
Write-Host "Wiring znf skills + hooks..."
try { & $exe skills sync } catch { Write-Host "note: 'zenify skills sync' skipped (run it manually later)" }

Write-Host "Installed zenify $installedVer"
Write-Host "Next: zenify up"
