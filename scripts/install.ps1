# Install the lrok CLI on Windows.
#
# Usage:
#   irm https://raw.githubusercontent.com/orcs-to/lrok.io-cli/main/scripts/install.ps1 | iex
#
# Optional env:
#   $env:LROK_VERSION       pin a release tag (default: latest)
#   $env:LROK_INSTALL_DIR   install path (default: $env:LOCALAPPDATA\lrok)

[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

$Repo = "orcs-to/lrok.io-cli"
$Version = if ($env:LROK_VERSION) { $env:LROK_VERSION } else { "latest" }
$InstallDir = if ($env:LROK_INSTALL_DIR) { $env:LROK_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "lrok" }

function Write-Info($msg) { Write-Host "lrok-install: $msg" }
function Fail($msg) { Write-Error "lrok-install: $msg"; exit 1 }

# --- detect arch ---
# Try modern .NET first; fall back to env vars on older PowerShell where
# RuntimeInformation can return an unrecognized or empty value.
$archStr = $null
try {
    $archStr = [string][System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
} catch { }
if (-not $archStr) {
    # PROCESSOR_ARCHITEW6432 is set inside a 32-bit process running on a
    # 64-bit OS; PROCESSOR_ARCHITECTURE is the running process arch. Either
    # way the OS arch is the canonical answer.
    $archStr = $env:PROCESSOR_ARCHITEW6432
    if (-not $archStr) { $archStr = $env:PROCESSOR_ARCHITECTURE }
}
switch -Regex ($archStr) {
    '^(X64|AMD64|x86_64)$'    { $arch = "amd64" }
    '^(Arm64|ARM64|aarch64)$' { $arch = "arm64" }
    default { Fail "unsupported arch: '$archStr' (set `$env:PROCESSOR_ARCHITECTURE manually if this is wrong)" }
}

$asset = "lrok-windows-$arch.exe"

if ($Version -eq "latest") {
    $baseUrl = "https://github.com/$Repo/releases/latest/download"
} else {
    $baseUrl = "https://github.com/$Repo/releases/download/$Version"
}
$binUrl  = "$baseUrl/$asset"
$sumsUrl = "$baseUrl/checksums.txt"

# --- download ---
$tmpDir = New-Item -ItemType Directory -Path (Join-Path $env:TEMP ("lrok-" + [System.Guid]::NewGuid())) | Select-Object -ExpandProperty FullName
try {
    Write-Info "downloading $asset"
    $binPath = Join-Path $tmpDir $asset
    Invoke-WebRequest -UseBasicParsing -Uri $binUrl -OutFile $binPath

    Write-Info "downloading checksums.txt"
    $sumsPath = Join-Path $tmpDir "checksums.txt"
    Invoke-WebRequest -UseBasicParsing -Uri $sumsUrl -OutFile $sumsPath

    # --- verify SHA256 ---
    $line = Get-Content $sumsPath | Where-Object { $_ -match "  $([regex]::Escape($asset))$" } | Select-Object -First 1
    if (-not $line) { Fail "no checksum entry for $asset" }
    $expected = ($line -split "\s+")[0].ToLower()
    $actual = (Get-FileHash -Algorithm SHA256 -Path $binPath).Hash.ToLower()
    if ($expected -ne $actual) {
        Fail "checksum mismatch: expected $expected, got $actual"
    }
    Write-Info "checksum OK"

    # --- install ---
    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir | Out-Null
    }
    $dest = Join-Path $InstallDir "lrok.exe"
    Copy-Item -Force $binPath $dest
    Write-Info "installed to $dest"

    # --- ensure on PATH (User scope) ---
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if (-not $userPath) { $userPath = "" }
    $segments = $userPath.Split(";", [System.StringSplitOptions]::RemoveEmptyEntries)
    if (-not ($segments -contains $InstallDir)) {
        $newPath = if ($userPath) { "$userPath;$InstallDir" } else { $InstallDir }
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        Write-Info "added $InstallDir to user PATH (open a new terminal to pick it up)"
    }

    # Also make it work in the current session.
    if (-not ($env:Path.Split(";") -contains $InstallDir)) {
        $env:Path = "$env:Path;$InstallDir"
    }

    & $dest version
}
finally {
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $tmpDir
}
