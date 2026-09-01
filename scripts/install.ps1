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
& (Join-Path $dest 'zenify.exe') version

# Warn if the install dir is not on the user PATH.
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($userPath -split ';') -notcontains $dest) {
  Write-Host ""
  Write-Host "warning: $dest is not on your PATH."
  Write-Host "  add it with (then restart the shell):"
  Write-Host "    [Environment]::SetEnvironmentVariable('Path', `"`$env:Path;$dest`", 'User')"
}
