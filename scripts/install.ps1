$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repo = 'Dante983/larascope'
$baseUrl = "https://github.com/$repo/releases"
$versionPattern = '^v[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.]+)?$'

if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -ne [System.Runtime.InteropServices.Architecture]::X64) {
    throw 'unsupported architecture (only windows/amd64 is released)'
}

[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

function Get-LatestTag {
    try {
        $response = Invoke-WebRequest -Uri "$baseUrl/latest" -MaximumRedirection 0 -ErrorAction Stop
        $location = $response.Headers.Location
    } catch {
        if ($null -eq $_.Exception.Response) { throw 'unable to find the latest Larascope release' }
        $location = $_.Exception.Response.Headers.Location
    }
    if ([string]::IsNullOrWhiteSpace($location)) { throw 'unable to find the latest Larascope release' }
    $tag = ($location.ToString().TrimEnd('/') -split '/')[-1]
    if ($tag -notmatch $versionPattern) { throw 'latest Larascope release is unavailable or has an invalid tag' }
    return $tag
}

if ($env:LARASCOPE_VERSION) {
    $tag = $env:LARASCOPE_VERSION
    if ($tag -notmatch $versionPattern) { throw "invalid LARASCOPE_VERSION: $tag" }
} else {
    $tag = Get-LatestTag
}

if ($env:LARASCOPE_INSTALL_DIR) {
    $installDir = $env:LARASCOPE_INSTALL_DIR
} else {
    $installDir = Join-Path $env:LOCALAPPDATA 'Programs\larascope\bin'
}

$version = $tag.Substring(1)
$archive = "larascope_${version}_windows_amd64.zip"
$tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("larascope-" + [guid]::NewGuid())

try {
    New-Item -ItemType Directory -Path $tempDir | Out-Null
    $archivePath = Join-Path $tempDir $archive
    $checksumsPath = Join-Path $tempDir 'checksums.txt'
    Invoke-WebRequest -Uri "$baseUrl/download/$tag/$archive" -OutFile $archivePath
    Invoke-WebRequest -Uri "$baseUrl/download/$tag/checksums.txt" -OutFile $checksumsPath

    $checksumLines = @(Get-Content $checksumsPath | Where-Object { ($_ -split '\s+')[1] -eq $archive })
    if ($checksumLines.Count -ne 1) { throw "checksum for $archive is missing or ambiguous" }
    $expected = ($checksumLines[0] -split '\s+')[0]
    $actual = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash
    if (-not $expected.Equals($actual, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "checksum mismatch for $archive (expected $expected, got $actual)"
    }

    $extractDir = Join-Path $tempDir 'extract'
    Expand-Archive -Path $archivePath -DestinationPath $extractDir
    $binary = Join-Path $extractDir 'larascope.exe'
    if (-not (Test-Path -LiteralPath $binary -PathType Leaf)) { throw 'archive does not contain larascope.exe' }
    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
    $temporaryBinary = Join-Path $installDir ('.larascope.new.' + $PID + '.exe')
    Copy-Item -LiteralPath $binary -Destination $temporaryBinary -Force
    Move-Item -LiteralPath $temporaryBinary -Destination (Join-Path $installDir 'larascope.exe') -Force
    $installed = Join-Path $installDir 'larascope.exe'
    Write-Host "Installed Larascope to $installed"
    & $installed --version
    Write-Host "To add it to your user PATH, run: [Environment]::SetEnvironmentVariable('Path', `"`$env:Path;$installDir`", 'User')"
} finally {
    if (Test-Path -LiteralPath $tempDir) { Remove-Item -LiteralPath $tempDir -Recurse -Force }
}
