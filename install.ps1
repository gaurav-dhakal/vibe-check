# Install vibe-check from the latest GitHub release (Windows).
#   irm https://raw.githubusercontent.com/gaurav-dhakal/vibe-check/main/install.ps1 | iex
# Honors: $env:VIBE_CHECK_VERSION, $env:VIBE_CHECK_INSTALL_DIR, $env:REPO
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'   # Invoke-WebRequest is glacial otherwise

$Repo       = if ($env:REPO) { $env:REPO } else { 'gaurav-dhakal/vibe-check' }
$WantVer    = if ($env:VIBE_CHECK_VERSION) { $env:VIBE_CHECK_VERSION } else { 'latest' }
$InstallDir = if ($env:VIBE_CHECK_INSTALL_DIR) { $env:VIBE_CHECK_INSTALL_DIR } `
              else { Join-Path $env:LOCALAPPDATA 'Programs\vibe-check' }

[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

function Write-Note($m) { Write-Host "» $m" -ForegroundColor Cyan }
function Die($m)        { Write-Host "✘ $m" -ForegroundColor Red; exit 1 }

try {
  $arch = [Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
} catch {
  # Older .NET: fall back to the environment variables (ARCHITEW6432 under WoW64).
  $arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
}
switch ($arch) {
  { $_ -in 'X64','AMD64' } { $archName = 'amd64' }
  { $_ -in 'Arm64','ARM64' } { $archName = 'arm64' }
  'X86'                   { Die '32-bit Windows is not supported; use a 64-bit shell' }
  default                 { Die "unsupported CPU architecture: $arch" }
}

$api = "https://api.github.com/repos/$Repo"
$headers = @{ 'User-Agent' = 'vibe-check-installer'; 'Accept' = 'application/vnd.github+json' }

if ($WantVer -eq 'latest') {
  try {
    $rel = Invoke-RestMethod -Uri "$api/releases/latest" -Headers $headers
    $tag = $rel.tag_name
  } catch {
    Die "no published release found for $Repo. If you have Go installed: go install github.com/$Repo@latest"
  }
} else {
  $tag = if ($WantVer.StartsWith('v')) { $WantVer } else { "v$WantVer" }
}
if (-not $tag) { Die 'could not resolve a release tag' }

$ver   = $tag.TrimStart('v')
$asset = "vibe-check_${ver}_windows_${archName}.zip"
$url   = "https://github.com/$Repo/releases/download/$tag/$asset"

Write-Note "installing vibe-check $tag (windows/$archName)"

$tmp    = Join-Path ([IO.Path]::GetTempPath()) ("vibecheck-" + [guid]::NewGuid().ToString('N'))
$zip    = Join-Path $tmp $asset
$stage  = Join-Path $tmp 'x'
New-Item -ItemType Directory -Force -Path $stage | Out-Null
try {
  Invoke-WebRequest -Uri $url -OutFile $zip
} catch {
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
  Die "download failed: $url"
}

# Verify against the release checksums when reachable; never install what we cannot account for.
try {
  $sums = (Invoke-WebRequest -Uri "https://github.com/$Repo/releases/download/$tag/checksums.txt").Content -split "`n"
  $line = $sums | Where-Object { $_ -match [Regex]::Escape($asset) } | Select-Object -First 1
  if ($line) {
    $want = ($line -split '\s+')[0].ToLower()
    $got = (Get-FileHash -Algorithm SHA256 -Path $zip).Hash.ToLower()
    if ($want -ne $got) { Die "checksum mismatch for $asset — refusing to install (expected $want, got $got)" }
    Write-Note 'sha256 verified'
  } else { Write-Warning 'asset missing from checksums.txt; installed unverified' }
} catch {
  Write-Warning 'checksums.txt unavailable; installed unverified'
}

Expand-Archive -Path $zip -DestinationPath $stage -Force
$exe = Join-Path $stage 'vibe-check.exe'
if (-not (Test-Path $exe)) { Die 'archive did not contain vibe-check.exe' }

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Copy-Item $exe (Join-Path $InstallDir 'vibe-check.exe') -Force
Copy-Item $exe (Join-Path $InstallDir 'vcheck.exe') -Force
Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue

Write-Note "installed to $InstallDir"

# Put the dir on the user PATH (registry, then broadcast so new shells see it).
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not ($userPath -split ';' | Where-Object { $_ -eq $InstallDir })) {
  [Environment]::SetEnvironmentVariable('Path', "$InstallDir;$userPath", 'User')
  Write-Note 'added to your user PATH (open a new terminal to pick it up)'
}
if (-not ($env:Path -split ';' | Where-Object { $_ -eq $InstallDir })) {
  $env:Path = "$InstallDir;$env:Path"
}

& (Join-Path $InstallDir 'vibe-check.exe') --version
if ($LASTEXITCODE -ne 0) { Die 'the installed binary did not run' }
Write-Note 'done — run  vibe-check   (or  vcheck)'
