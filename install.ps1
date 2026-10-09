$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function Fail([string]$Message) {
    throw "godex installer: $Message"
}

$Repository = if ($env:GODEX_REPOSITORY) { $env:GODEX_REPOSITORY } else { "christiandoxa/godex" }
$InstallDir = if ($env:GODEX_INSTALL_DIR) {
    $env:GODEX_INSTALL_DIR
} elseif ($env:LOCALAPPDATA) {
    Join-Path $env:LOCALAPPDATA "Programs\Godex\bin"
} else {
    Join-Path $HOME ".godex\bin"
}
$ReleaseBase = if ($env:GODEX_RELEASE_BASE_URL) {
    $env:GODEX_RELEASE_BASE_URL.TrimEnd("/").TrimEnd("\")
} else {
    "https://github.com/$Repository/releases/download"
}

if ($env:GODEX_VERSION) {
    $Tag = if ($env:GODEX_VERSION.StartsWith("v")) { $env:GODEX_VERSION } else { "v$($env:GODEX_VERSION)" }
} else {
    try {
        $Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repository/releases/latest"
        $Tag = [string]$Release.tag_name
    } catch {
        Fail "cannot resolve latest release: $($_.Exception.Message)"
    }
}
if ([string]::IsNullOrWhiteSpace($Tag)) { Fail "latest release did not contain a tag" }
if ($Tag -cnotmatch '^v[0-9][0-9A-Za-z.+-]*$') { Fail "release tag is invalid" }
$Version = $Tag.Substring(1)

$Architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
switch ($Architecture) {
    "X64" { $Arch = "amd64" }
    "Arm64" { $Arch = "arm64" }
    default { Fail "unsupported architecture: $Architecture" }
}

$Archive = "godex_${Version}_windows_${Arch}.zip"
$Temporary = Join-Path ([System.IO.Path]::GetTempPath()) ("godex-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $Temporary | Out-Null
$StagedDestination = $null

try {
    $ArchivePath = Join-Path $Temporary $Archive
    $ChecksumsPath = Join-Path $Temporary "checksums.txt"
    Invoke-WebRequest -Uri "$ReleaseBase/$Tag/$Archive" -OutFile $ArchivePath -UseBasicParsing
    Invoke-WebRequest -Uri "$ReleaseBase/$Tag/checksums.txt" -OutFile $ChecksumsPath -UseBasicParsing

    $Expected = $null
    foreach ($Line in Get-Content $ChecksumsPath) {
        if ($Line -match "^([0-9a-fA-F]{64})\s+(.+)$" -and $Matches[2] -eq $Archive) {
            $Expected = $Matches[1].ToLowerInvariant()
            break
        }
    }
    if (-not $Expected) { Fail "checksum for $Archive was not found" }

    $Actual = (Get-FileHash -Algorithm SHA256 -Path $ArchivePath).Hash.ToLowerInvariant()
    if ($Actual -ne $Expected) { Fail "SHA-256 mismatch for $Archive" }

    $Extracted = Join-Path $Temporary "extracted"
    Expand-Archive -Path $ArchivePath -DestinationPath $Extracted
    $Source = Join-Path $Extracted "godex.exe"
    if (-not (Test-Path $Source -PathType Leaf)) { Fail "archive does not contain godex.exe" }
    $VersionOutput = (& $Source --version | Out-String).Trim()
    if ($LASTEXITCODE -ne 0) { Fail "downloaded binary failed its version check" }
    if ($VersionOutput -notmatch "^godex $([regex]::Escape($Version))(?:\s|$)") {
        Fail "downloaded binary reported an unexpected version"
    }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $Destination = Join-Path $InstallDir "godex.exe"
    $StagedDestination = "$Destination.tmp.$PID"
    Copy-Item -Path $Source -Destination $StagedDestination -Force
    Move-Item -Path $StagedDestination -Destination $Destination -Force
    $StagedDestination = $null

    $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $PathEntries = @()
    if ($UserPath) { $PathEntries = $UserPath.Split(";", [System.StringSplitOptions]::RemoveEmptyEntries) }
    if (-not ($PathEntries | Where-Object { $_.TrimEnd("\") -ieq $InstallDir.TrimEnd("\") })) {
        $NewPath = if ($UserPath) { "$UserPath;$InstallDir" } else { $InstallDir }
        [Environment]::SetEnvironmentVariable("Path", $NewPath, "User")
        $env:Path = "$env:Path;$InstallDir"
    }

    $InstalledVersion = (& $Destination --version | Out-String).Trim()
    Write-Host "Installed $InstalledVersion to $Destination"
} finally {
    if ($StagedDestination -and (Test-Path $StagedDestination -PathType Leaf)) {
        Remove-Item -Force $StagedDestination -ErrorAction SilentlyContinue
    }
    Remove-Item -Recurse -Force $Temporary -ErrorAction SilentlyContinue
}
