# Install the lrok CLI on Windows, configured for the STAGING environment.
#
# Same Go binary as production. Installed under the name `staging-lrok.exe`
# so the env package detects "staging" from argv[0] and points at:
#   - https://api.staging.lrok.io
#   - tunnel.lrok.io:7001  (port 7001 is the staging differentiator;
#     hostname reuses the prod wildcard TLS cert)
#   - %USERPROFILE%\.lrok-staging\config
#
# Usage:
#   irm https://raw.githubusercontent.com/orcs-to/lrok.io-cli/main/scripts/staging-install.ps1 | iex
#
# Optional env (mirrors install.ps1):
#   $env:LROK_VERSION       pin a release tag (default: latest)
#   $env:LROK_INSTALL_DIR   install path (default: $env:LOCALAPPDATA\lrok)
#   $env:LROK_TELEMETRY=0   disable install lifecycle beacons

[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

$Repo = "orcs-to/lrok.io-cli"
$Version = if ($env:LROK_VERSION) { $env:LROK_VERSION } else { "latest" }
$InstallDir = if ($env:LROK_INSTALL_DIR) { $env:LROK_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "lrok" }

function Write-Info($msg) { Write-Host "staging-lrok-install: $msg" }
function Fail($msg) { Send-Beacon failed; Write-Error "staging-lrok-install: $msg"; exit 1 }

# Lifecycle beacon → staging tracker so install funnel is observable
# independently of prod. Disable with $env:LROK_TELEMETRY=0.
function Send-Beacon($stage) {
  if ($env:LROK_TELEMETRY -eq '0') { return }
  $archForBeacon = if ($script:arch) { $script:arch } else { 'unknown' }
  $body = @{ channel = 'ps1-staging'; arch = $archForBeacon; stage = $stage } | ConvertTo-Json -Compress
  try {
    Invoke-RestMethod -Uri 'https://api.staging.lrok.io/api/v1/track/install' `
      -Method Post -ContentType 'application/json' -Body $body `
      -TimeoutSec 3 -ErrorAction SilentlyContinue | Out-Null
  } catch {
    # telemetry must never break the install
  }
}

# --- detect arch ---
$archStr = $null
try {
    $archStr = [string][System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
} catch { }
if (-not $archStr) {
    $archStr = $env:PROCESSOR_ARCHITEW6432
    if (-not $archStr) { $archStr = $env:PROCESSOR_ARCHITECTURE }
}
switch -Regex ($archStr) {
    '^(X64|AMD64|x86_64)$'    { $arch = "amd64" }
    '^(Arm64|ARM64|aarch64)$' { $arch = "arm64" }
    default { Fail "unsupported arch: '$archStr' (set `$env:PROCESSOR_ARCHITECTURE manually if this is wrong)" }
}

$asset = "lrok-windows-$arch.exe"

Send-Beacon started

if ($Version -eq "latest") {
    $baseUrl = "https://github.com/$Repo/releases/latest/download"
} else {
    $baseUrl = "https://github.com/$Repo/releases/download/$Version"
}
$binUrl  = "$baseUrl/$asset"
$sumsUrl = "$baseUrl/checksums.txt"

$tmpDir = New-Item -ItemType Directory -Path (Join-Path $env:TEMP ("staging-lrok-" + [System.Guid]::NewGuid())) | Select-Object -ExpandProperty FullName
try {
    Write-Info "downloading $asset (staging)"
    $binPath = Join-Path $tmpDir $asset
    Invoke-WebRequest -UseBasicParsing -Uri $binUrl -OutFile $binPath

    Write-Info "downloading checksums.txt"
    $sumsPath = Join-Path $tmpDir "checksums.txt"
    Invoke-WebRequest -UseBasicParsing -Uri $sumsUrl -OutFile $sumsPath

    $line = Get-Content $sumsPath | Where-Object { $_ -match "  $([regex]::Escape($asset))$" } | Select-Object -First 1
    if (-not $line) { Fail "no checksum entry for $asset" }
    $expected = ($line -split "\s+")[0].ToLower()
    $actual = (Get-FileHash -Algorithm SHA256 -Path $binPath).Hash.ToLower()
    if ($expected -ne $actual) {
        Fail "checksum mismatch: expected $expected, got $actual"
    }
    Write-Info "checksum OK"

    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir | Out-Null
    }
    # NOTE: dest filename is staging-lrok.exe so internal/env detects
    # the staging environment from argv[0].
    $dest = Join-Path $InstallDir "staging-lrok.exe"
    Copy-Item -Force $binPath $dest
    Write-Info "installed to $dest"

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if (-not $userPath) { $userPath = "" }
    $segments = $userPath.Split(";", [System.StringSplitOptions]::RemoveEmptyEntries)
    if (-not ($segments -contains $InstallDir)) {
        $newPath = if ($userPath) { "$userPath;$InstallDir" } else { $InstallDir }
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        Write-Info "added $InstallDir to user PATH (open a new terminal to pick it up)"
    }

    if (-not ($env:Path.Split(";") -contains $InstallDir)) {
        $env:Path = "$env:Path;$InstallDir"
    }

    & $dest version

    Send-Beacon ok
}
finally {
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $tmpDir
}
