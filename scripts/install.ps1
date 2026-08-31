$ErrorActionPreference = 'Stop'
$repo = 'ZenifyAIContactCenter/zenify-kit'
$arch = if ([Environment]::Is64BitOperatingSystem) { 'amd64' } else { 'arm64' }
if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { $arch = 'arm64' }
$ver = if ($env:ZENIFY_VERSION) { $env:ZENIFY_VERSION } else {
  (Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest").tag_name
}
$url = "https://github.com/$repo/releases/download/$ver/zenify_windows_$arch.zip"
$tmp = Join-Path $env:TEMP "zenify-$([guid]::NewGuid())"
New-Item -ItemType Directory -Path $tmp | Out-Null
$zip = Join-Path $tmp 'zenify.zip'
Invoke-WebRequest $url -OutFile $zip
Expand-Archive $zip -DestinationPath $tmp
$dest = Join-Path $env:LOCALAPPDATA 'Programs\zenify'
New-Item -ItemType Directory -Force -Path $dest | Out-Null
Move-Item -Force (Join-Path $tmp 'zenify.exe') (Join-Path $dest 'zenify.exe')
Write-Host "Installed zenify to $dest"
& (Join-Path $dest 'zenify.exe') version
