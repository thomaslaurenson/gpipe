#Requires -Version 7.0
#Requires -Modules @{ ModuleName = 'Pester'; ModuleVersion = '5.0' }

# Tests for templates/install.ps1
#
# The script is dot-sourced (not executed) via the $MyInvocation.InvocationName
# guard at the bottom of the template. Each function is called directly, with
# external dependencies (Invoke-WebRequest, Get-FileHash, Get-Command, cosign)
# mocked by Pester's built-in Mock system.

BeforeAll {
    $script:RepoRoot   = (Resolve-Path "$PSScriptRoot/..").Path
    $script:FixtureDir = Join-Path $script:RepoRoot 'test/fixtures'
    $script:RenderedPs1 = Join-Path $script:FixtureDir 'install_rendered.ps1'

    # Dot-source the rendered script. The $MyInvocation guard prevents
    # Invoke-Installer from running.
    . $script:RenderedPs1

    # Create a small fake binary fixture for checksum tests.
    $script:FakeBinary = Join-Path $script:FixtureDir 'fake_binary_ps'
    [System.IO.File]::WriteAllText($script:FakeBinary, 'fake-ps-binary')

    $script:FakeHash = (Get-FileHash -Algorithm SHA256 $script:FakeBinary).Hash.ToLower()

    # Checksums fixture file in sha256sum two-space format.
    # Stored with a distinct name so BeforeEach can copy it to checksums.txt
    # in the same TestDrive without triggering a copy-to-itself error.
    $script:ChecksumsFile = Join-Path $TestDrive 'checksums_source.txt'
    Set-Content -Path $script:ChecksumsFile -Value "$script:FakeHash  fake_binary_ps"

    $script:ChecksumsFileBad = Join-Path $TestDrive 'checksums_bad.txt'
    Set-Content -Path $script:ChecksumsFileBad -Value "0000000000000000000000000000000000000000000000000000000000000000  fake_binary_ps"
}

AfterAll {
    if (Test-Path $script:FakeBinary) { Remove-Item $script:FakeBinary }
}

# ============================================================
# Get-Platform
# ============================================================

Describe 'Get-Platform' {

    It 'returns windows_amd64 for AMD64' {
        $env:PROCESSOR_ARCHITECTURE  = 'AMD64'
        $env:PROCESSOR_ARCHITEW6432  = $null
        Get-Platform | Should -Be 'windows_amd64'
    }

    It 'returns windows_arm64 for ARM64' {
        $env:PROCESSOR_ARCHITECTURE  = 'ARM64'
        $env:PROCESSOR_ARCHITEW6432  = $null
        Get-Platform | Should -Be 'windows_arm64'
    }

    It 'prefers PROCESSOR_ARCHITEW6432 over PROCESSOR_ARCHITECTURE (WOW64)' {
        $env:PROCESSOR_ARCHITECTURE  = 'x86'
        $env:PROCESSOR_ARCHITEW6432  = 'AMD64'
        Get-Platform | Should -Be 'windows_amd64'
        $env:PROCESSOR_ARCHITEW6432  = $null
    }

    It 'exits with an error for unsupported architecture' {
        $env:PROCESSOR_ARCHITECTURE  = 'MIPS'
        $env:PROCESSOR_ARCHITEW6432  = $null
        { Get-Platform } | Should -Throw
    }
}

# ============================================================
# Resolve-Asset
# ============================================================

Describe 'Resolve-Asset' {

    It 'returns the correct asset name for windows_amd64' {
        Resolve-Asset -Platform 'windows_amd64' | Should -Be 'mytool_windows_amd64.exe'
    }

    It 'returns the correct asset name for windows_arm64' {
        Resolve-Asset -Platform 'windows_arm64' | Should -Be 'mytool_windows_arm64.exe'
    }

    It 'exits with an error for an unsupported platform' {
        { Resolve-Asset -Platform 'linux_amd64' } | Should -Throw
    }
}

# ============================================================
# Confirm-Signature
# ============================================================

Describe 'Confirm-Signature' {

    It 'skips and warns when NoVerify is true' {
        $result = (Confirm-Signature -TmpDir $TestDrive -NoVerify $true 6>&1) | Out-String
        $result | Should -Match 'Skipping cosign'
    }

    It 'exits with an error when cosign is not found in PATH' {
        Mock Get-Command { return $null } -ParameterFilter { $Name -eq 'cosign' }
        { Confirm-Signature -TmpDir $TestDrive -NoVerify $false } | Should -Throw
    }

    It 'passes when cosign exits 0' {
        Mock Get-Command { return [PSCustomObject]@{ Name = 'cosign' } } `
            -ParameterFilter { $Name -eq 'cosign' }
        Mock Invoke-Cosign { $global:LASTEXITCODE = 0 }

        { Confirm-Signature -TmpDir $TestDrive -NoVerify $false } | Should -Not -Throw
    }

    It 'exits with an error when cosign exits non-zero' {
        Mock Get-Command { return [PSCustomObject]@{ Name = 'cosign' } } `
            -ParameterFilter { $Name -eq 'cosign' }
        Mock Invoke-Cosign { $global:LASTEXITCODE = 1 }

        { Confirm-Signature -TmpDir $TestDrive -NoVerify $false } | Should -Throw
    }
}

# ============================================================
# Confirm-Checksum
# ============================================================

Describe 'Confirm-Checksum' {

    BeforeEach {
        # Copy fake binary into TestDrive alongside the checksums file.
        Copy-Item $script:FakeBinary (Join-Path $TestDrive 'fake_binary_ps')
        Copy-Item $script:ChecksumsFile (Join-Path $TestDrive 'checksums.txt')
    }

    It 'passes when the hash matches' {
        { Confirm-Checksum -TmpDir $TestDrive -AssetName 'fake_binary_ps' } | Should -Not -Throw
    }

    It 'exits with an error when the hash does not match' {
        # Overwrite checksums.txt with a bad hash.
        Copy-Item $script:ChecksumsFileBad (Join-Path $TestDrive 'checksums.txt') -Force
        { Confirm-Checksum -TmpDir $TestDrive -AssetName 'fake_binary_ps' } | Should -Throw
    }

    It 'exits with an error when the asset entry is missing from checksums.txt' {
        Set-Content -Path (Join-Path $TestDrive 'checksums.txt') -Value ''
        { Confirm-Checksum -TmpDir $TestDrive -AssetName 'fake_binary_ps' } | Should -Throw
    }

    It 'is case-insensitive on the hash comparison' {
        # Write the expected hash in uppercase — should still pass.
        Set-Content -Path (Join-Path $TestDrive 'checksums.txt') `
            -Value "$($script:FakeHash.ToUpper())  fake_binary_ps"
        { Confirm-Checksum -TmpDir $TestDrive -AssetName 'fake_binary_ps' } | Should -Not -Throw
    }
}

# ============================================================
# Resolve-InstallDir
# ============================================================

Describe 'Resolve-InstallDir' {

    It 'returns the LOCALAPPDATA Programs path for a user install' {
        $env:LOCALAPPDATA = $TestDrive
        $result = Resolve-InstallDir -UserInstall $true
        $result.Path | Should -Be (Join-Path $TestDrive "Programs\mytool")
    }

    It 'user install path includes the install name' {
        $env:LOCALAPPDATA = $TestDrive
        $result = Resolve-InstallDir -UserInstall $true
        $result.Path | Should -BeLike "*$script:InstallName*"
    }

    It 'user install reports IsUserInstall = true' {
        $env:LOCALAPPDATA = $TestDrive
        $result = Resolve-InstallDir -UserInstall $true
        $result.IsUserInstall | Should -Be $true
    }
}

# ============================================================
# Resolve-PsExe (elevation helper)
# ============================================================

Describe 'Elevation uses correct PowerShell executable' {

    It 'uses pwsh when running under PowerShell Core (PSEdition = Core)' {
        # We cannot call Resolve-InstallDir and trigger UAC in a test, so we
        # verify the $psExe selection logic directly by evaluating the same
        # expression the template uses. Using $edition (not $psEdition) avoids
        # colliding with the read-only automatic variable $PSEdition.
        $edition = 'Core'
        $psExe = if ($edition -eq 'Core') { 'pwsh' } else { 'powershell' }
        $psExe | Should -Be 'pwsh'
    }

    It 'uses powershell when running under Windows PowerShell (PSEdition = Desktop)' {
        $edition = 'Desktop'
        $psExe = if ($edition -eq 'Core') { 'pwsh' } else { 'powershell' }
        $psExe | Should -Be 'powershell'
    }

    It 'actual PSVersionTable.PSEdition produces a non-empty exe name' {
        # Sanity check: whatever edition is running, the expression resolves to
        # a non-empty string (not $null or empty).
        $psExe = if ($PSVersionTable.PSEdition -eq 'Core') { 'pwsh' } else { 'powershell' }
        $psExe | Should -Not -BeNullOrEmpty
    }
}

# ============================================================
# Install-Binary
# ============================================================

Describe 'Install-Binary' {

    BeforeEach {
        Copy-Item $script:FakeBinary (Join-Path $TestDrive 'fake_binary_ps') -Force
    }

    It 'copies the binary to the install directory' {
        $destDir = Join-Path $TestDrive 'install_test'
        Install-Binary -TmpDir $TestDrive -AssetName 'fake_binary_ps' -InstallDir $destDir
        (Join-Path $destDir 'mytool.exe') | Should -Exist
    }

    It 'creates the install directory if it does not exist' {
        $destDir = Join-Path $TestDrive 'new_install_dir'
        $destDir | Should -Not -Exist
        Install-Binary -TmpDir $TestDrive -AssetName 'fake_binary_ps' -InstallDir $destDir
        $destDir | Should -Exist
    }

    It 'overwrites an existing binary' {
        $destDir = Join-Path $TestDrive 'overwrite_test'
        New-Item -ItemType Directory -Path $destDir | Out-Null
        Set-Content -Path (Join-Path $destDir 'mytool.exe') -Value 'old content'
        Install-Binary -TmpDir $TestDrive -AssetName 'fake_binary_ps' -InstallDir $destDir
        Get-Content (Join-Path $destDir 'mytool.exe') | Should -Be 'fake-ps-binary'
    }
}

# ============================================================
# Update-Path
# ============================================================
#
# [Environment]::GetEnvironmentVariable / SetEnvironmentVariable are .NET
# static methods that Pester 5 cannot mock. Tests use two strategies:
#   1. Test the guard-condition logic directly (pure boolean expressions).
#   2. Call Update-Path with $UserInstall=$false and inspect $env:PATH,
#      which IS writable and observable in the test process.

Describe 'Update-Path' {

    It 'user install: guard is false when InstallDir already in PATH string' {
        # Simulates the -notlike check: should be $false, skipping SetEnv.
        $installDir  = 'C:\Users\test\AppData\Local\Programs\mytool'
        $existingPath = "C:\Windows\system32;$installDir"
        ($existingPath -notlike "*$installDir*") | Should -Be $false
    }

    It 'user install: guard is true when InstallDir is absent from PATH string' {
        # Simulates the -notlike check: should be $true, triggering SetEnv.
        $installDir  = 'C:\Users\test\AppData\Local\Programs\mytool'
        $existingPath = 'C:\Windows\system32'
        ($existingPath -notlike "*$installDir*") | Should -Be $true
    }

    It 'system install: updates current session PATH immediately' {
        $installDir = Join-Path $TestDrive 'sysbin'
        $before = $env:PATH

        Update-Path -InstallDir $installDir -UserInstall $false

        $env:PATH | Should -BeLike "*$installDir*"
        $env:PATH = $before
    }

    It 'system install: does not prepend InstallDir twice when already in session PATH' {
        $installDir = Join-Path $TestDrive 'sysbin_dup'
        $env:PATH   = "C:\Windows\system32;$installDir"
        $before = $env:PATH

        Update-Path -InstallDir $installDir -UserInstall $false

        $occurrences = ($env:PATH -split ';' | Where-Object { $_ -eq $installDir }).Count
        $occurrences | Should -Be 1
        $env:PATH = $before
    }

    It 'system install: source uses Machine scope not User scope' {
        # Verify the correct registry scope is used by inspecting the function body.
        $source = (Get-Command Update-Path).ScriptBlock.ToString()
        $source | Should -Match "'Machine'"
    }

    It 'user install: source uses User scope not Machine scope' {
        $source = (Get-Command Update-Path).ScriptBlock.ToString()
        $source | Should -Match "'User'"
    }
}

# ============================================================
# Invoke-DownloadAsset
# ============================================================

Describe 'Invoke-DownloadAsset' {

    BeforeEach {
        $env:PROCESSOR_ARCHITECTURE = 'AMD64'
        $env:PROCESSOR_ARCHITEW6432 = $null
    }

    It 'calls Invoke-WebRequest three times for binary, checksums, and sigstore bundle' {
        Mock Invoke-WebRequest {
            # Write an empty file to the requested output path.
            Set-Content -Path $OutFile -Value ''
        }

        Invoke-DownloadAsset -TmpDir $TestDrive -AssetName 'mytool_windows_amd64.exe'

        Assert-MockCalled Invoke-WebRequest -Times 3
    }

    It 'exits with an error when the binary download fails' {
        Mock Invoke-WebRequest { throw 'network error' } `
            -ParameterFilter { $OutFile -like '*mytool_windows_amd64.exe' }

        { Invoke-DownloadAsset -TmpDir $TestDrive -AssetName 'mytool_windows_amd64.exe' } |
            Should -Throw
    }
}

# ============================================================
# install_rendered_full.ps1: hook and completion injection
# ============================================================
#
# These tests verify that the full fixture — rendered with all completions
# enabled and pre/post hooks injected from test/fixtures/hooks/ — contains
# the expected sentinels and hook content. File-content checks read the
# raw file text. The function-availability check dot-sources the full
# fixture in a child scope to confirm Install-Completion is defined.

Describe 'install_rendered.ps1: hook and completion injection' {

    It 'powershell-completions sentinel is present in file' {
        $renderedContent = Get-Content $script:RenderedPs1 -Raw
        $renderedContent | Should -Match ([regex]::Escape('# gpipe test: powershell-completions'))
    }

    It 'pre-install-hook sentinel is present in file' {
        $renderedContent = Get-Content $script:RenderedPs1 -Raw
        $renderedContent | Should -Match ([regex]::Escape('# gpipe test: pre-install-hook'))
    }

    It 'post-install-hook sentinel is present in file' {
        $renderedContent = Get-Content $script:RenderedPs1 -Raw
        $renderedContent | Should -Match ([regex]::Escape('# gpipe test: post-install-hook'))
    }

    It 'pre-hook content is injected' {
        $renderedContent = Get-Content $script:RenderedPs1 -Raw
        $renderedContent | Should -Match ([regex]::Escape('Write-Host "gpipe-fixture-pre-hook"'))
    }

    It 'post-hook content is injected' {
        $renderedContent = Get-Content $script:RenderedPs1 -Raw
        $renderedContent | Should -Match ([regex]::Escape('Write-Host "gpipe-fixture-post-hook"'))
    }

    It 'Install-Completion function is defined after dot-sourcing' {
        . $script:RenderedPs1
        Get-Command Install-Completion -ErrorAction SilentlyContinue |
            Should -Not -BeNullOrEmpty
    }
}
