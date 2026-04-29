# {{GPIPE_GENERATED_BY}}

param(
    [switch]$User,
    [switch]$Help
)

$ErrorActionPreference = "Stop"
$ProgressPreference    = "SilentlyContinue"

# Constants: baked in at generation time
$GithubRepo  = "{{GITHUB_REPO}}"
$Version     = "{{VERSION}}"
$Binary      = "{{BINARY}}"
$InstallName = "{{INSTALL_NAME}}"

# Output helpers
$NoColor = ($null -ne $env:NO_COLOR)

function Write-Info  { param($Msg) if ($script:NoColor) { Write-Host "[INFO]  $Msg"  } else { Write-Host "[INFO]  " -ForegroundColor Green  -NoNewline; Write-Host $Msg } }
function Write-Warn  { param($Msg) if ($script:NoColor) { Write-Host "[WARN]  $Msg"  } else { Write-Host "[WARN]  " -ForegroundColor Yellow -NoNewline; Write-Host $Msg } }
function Write-Step  { param($Msg) if ($script:NoColor) { Write-Host "  > $Msg"      } else { Write-Host "  > "     -ForegroundColor Cyan   -NoNewline; Write-Host $Msg } }
function Exit-Error  {
    param($Msg)
    if ($script:NoColor) { Write-Host "[ERROR] $Msg" } else { Write-Host "[ERROR] " -ForegroundColor Red -NoNewline; Write-Host $Msg }
    exit 1
}

# Usage
function Show-Help {
    Write-Host @"
${InstallName} installer

USAGE:
  .\install.ps1 [-User] [-Help]

OPTIONS:
  -User    Install to user directory (no elevation required)
           Default path: $env:LOCALAPPDATA\Programs\${InstallName}
  -Help    Show this help message

EXAMPLES:
  # System-wide install (default, prompts for elevation if not Administrator)
  .\install.ps1

  # User install, no elevation needed
  .\install.ps1 -User

  # Piped user install
  Invoke-WebRequest -Uri "https://github.com/${Repo}/releases/download/${Version}/install.ps1" ``
    -OutFile install.ps1; .\install.ps1 -User

"@
}

if ($Help) { Show-Help; exit 0 }

# Platform detection
# Use PROCESSOR_ARCHITECTURE env vars for compatibility with PowerShell 5.1+
# PROCESSOR_ARCHITEW6432 is set when a 32-bit process runs on a 64-bit OS
$OSArch = $env:PROCESSOR_ARCHITECTURE
if ($env:PROCESSOR_ARCHITEW6432) { $OSArch = $env:PROCESSOR_ARCHITEW6432 }

$NormArch = switch ($OSArch) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default { Exit-Error "Unsupported architecture: $OSArch" }
}

$Platform = "windows_${NormArch}"

# Platform validation: baked in at generation time
{{SUPPORTED_PLATFORMS_BLOCK}}

# Asset map: baked in at generation time: platform -> release filename
{{ASSET_MAP_BLOCK}}

$AssetName    = $AssetNames[$Platform]
$DownloadUrl  = "https://github.com/$GithubRepo/releases/download/$Version/$AssetName"
$ChecksumsUrl = "https://github.com/$GithubRepo/releases/download/$Version/checksums.txt"

# Pre-install hook
{{PRE_INSTALL_HOOK}}

# Download to temp directory
$TmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ([System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $TmpDir | Out-Null

try {
    Write-Info "Downloading $Binary $Version for $Platform..."
    Write-Step  $DownloadUrl

    try {
        Invoke-WebRequest -Uri $DownloadUrl  -OutFile "$TmpDir\$AssetName"  -UseBasicParsing
    } catch {
        Exit-Error "Failed to download ${AssetName}: $_"
    }

    try {
        Invoke-WebRequest -Uri $ChecksumsUrl -OutFile "$TmpDir\checksums.txt" -UseBasicParsing
    } catch {
        Exit-Error "Failed to download checksums.txt: $_"
    }

    # Checksum verification
    Write-Info "Verifying checksum..."
    $ChecksumLine = Get-Content "$TmpDir\checksums.txt" |
        Where-Object { $_ -match "  $([regex]::Escape($AssetName))$" }

    if (-not $ChecksumLine) {
        Exit-Error "Checksum not found for $AssetName in checksums.txt"
    }

    $ExpectedHash = ($ChecksumLine -split "\s+")[0]
    $ActualHash   = (Get-FileHash -Algorithm SHA256 "$TmpDir\$AssetName").Hash.ToLower()

    if ($ExpectedHash -ne $ActualHash) {
        Exit-Error "Checksum mismatch for ${AssetName}:`n  expected: $ExpectedHash`n  actual:   $ActualHash"
    }
    Write-Step "Checksum OK"

    # Determine install location; handle elevation when needed
    $UserInstall = $User.IsPresent
    $InstallDir  = ""

    if ($UserInstall) {
        $InstallDir = "$env:LOCALAPPDATA\Programs\$InstallName"
    } else {
        $SystemDir = "$env:ProgramFiles\$InstallName"
        $IsAdmin   = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
                        [Security.Principal.WindowsBuiltInRole]::Administrator)

        if ($IsAdmin) {
            $InstallDir = $SystemDir
        } else {
            # Not running as Administrator: offer escalation options
            if ([Environment]::UserInteractive) {
                Write-Host ""
                Write-Host "Insufficient permissions to install to $SystemDir" -ForegroundColor Yellow
                Write-Host ""
                Write-Host "  1) Re-run as Administrator (opens elevated prompt)"
                Write-Host "  2) Install to $env:LOCALAPPDATA\Programs\$InstallName (no elevation)"
                Write-Host "  3) Quit"
                Write-Host ""
                $Choice = Read-Host "Choose [1/2/3]"

                switch ($Choice.Trim()) {
                    "1" {
                        if (-not [string]::IsNullOrEmpty($PSCommandPath)) {
                            Write-Info "Launching elevated session..."
                            $ArgList = "-NoProfile -ExecutionPolicy Bypass -File `"$PSCommandPath`""
                            Start-Process powershell -Verb RunAs -ArgumentList $ArgList -Wait
                            exit 0
                        } else {
                            Write-Host "[ERROR] Cannot auto-elevate: script was not run from a file." -ForegroundColor Red
                            Write-Host "        Save install.ps1 to disk, then run in an elevated prompt:"
                            Write-Host "          powershell -File .\install.ps1"
                            exit 1
                        }
                    }
                    "2" {
                        $UserInstall = $true
                        $InstallDir  = "$env:LOCALAPPDATA\Programs\$InstallName"
                    }
                    default { Exit-Error "Installation aborted." }
                }
            } else {
                # Non-interactive (CI/piped): print clear instructions and exit
                Write-Host "[ERROR] Insufficient permissions to install to $SystemDir" -ForegroundColor Red
                Write-Host ""
                Write-Host "To install in an elevated PowerShell session, download and run:"
                Write-Host "  Invoke-WebRequest -Uri `"https://github.com/$GithubRepo/releases/download/$Version/install.ps1`" -OutFile install.ps1"
                Write-Host "  Start-Process powershell -Verb RunAs -ArgumentList `"-File .\install.ps1`" -Wait"
                Write-Host ""
                Write-Host "To install without elevation (user install):"
                Write-Host "  Invoke-WebRequest -Uri `"https://github.com/$GithubRepo/releases/download/$Version/install.ps1`" -OutFile install.ps1"
                Write-Host "  .\install.ps1 -User"
                exit 1
            }
        }
    }

    # Copy binary to install directory
    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir | Out-Null
    }
    Copy-Item "$TmpDir\$AssetName" "$InstallDir\$InstallName.exe" -Force
    Write-Info "Installed $InstallName to $InstallDir\$InstallName.exe"

# BEGIN_BLOCK:COMPLETION_POWERSHELL
    # PowerShell completions
    try {
        $CompletionOutput = & "$InstallDir\$InstallName.exe" completion powershell 2>&1
        if ($LASTEXITCODE -eq 0) {
            $ProfileDir = Split-Path $PROFILE
            if (-not (Test-Path $ProfileDir)) { New-Item -ItemType Directory -Path $ProfileDir | Out-Null }
            if (-not (Test-Path $PROFILE))    { New-Item -ItemType File      -Path $PROFILE    | Out-Null }
            $Marker  = "# $Binary completions (added by gpipe)"
            $Content = Get-Content $PROFILE -Raw -ErrorAction SilentlyContinue
            if (-not $Content -or $Content -notmatch [regex]::Escape($Marker)) {
                Add-Content -Path $PROFILE -Value "`n$Marker`n$CompletionOutput"
                Write-Step "PowerShell completions → $PROFILE"
            }
        } else {
            Write-Warn "PowerShell completion generation failed, skipping"
        }
    } catch {
        Write-Warn "PowerShell completion generation failed, skipping: $_"
    }
# END_BLOCK:COMPLETION_POWERSHELL

    # Post-install hook
    {{POST_INSTALL_HOOK}}

    # PATH management
    if ($UserInstall) {
        $UserPath = [Environment]::GetEnvironmentVariable("PATH", "User")
        if ($UserPath -notlike "*$InstallDir*") {
            [Environment]::SetEnvironmentVariable("PATH", "$InstallDir;$UserPath", "User")
            Write-Warn "Added $InstallDir to user PATH. Restart your terminal for the change to take effect."
        }
    } else {
        # Refresh PATH for the current session so the binary is findable immediately
        $env:PATH = "$InstallDir;$env:PATH"
        if (-not (Get-Command $InstallName -ErrorAction SilentlyContinue)) {
            Write-Warn "$InstallName is not reachable via PATH: $InstallDir may need to be added manually"
        }
    }

    Write-Info "Successfully installed $InstallName $Version"

} finally {
    Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue
}
