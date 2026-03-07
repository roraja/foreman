# Foreman installer for Windows — downloads the latest binary to ~/.foreman/bin
$ErrorActionPreference = "Stop"

$Repo = "roraja/foreman"
$BaseUrl = "https://github.com/$Repo/releases/download"

Write-Host "==> Starting foreman installer"

# Resolve latest version from GitHub if not explicitly set
if (-not $env:FOREMAN_VERSION) {
    Write-Host "==> Detecting latest version from GitHub..."
    try {
        # Follow the /releases/latest redirect to get the version tag
        $response = Invoke-WebRequest -Uri "https://github.com/$Repo/releases/latest" -MaximumRedirection 0 -ErrorAction SilentlyContinue -UseBasicParsing
    } catch {
        $response = $_.Exception.Response
    }
    if ($response.Headers.Location) {
        $location = $response.Headers.Location
        if ($location -is [System.Collections.IEnumerable] -and $location -isnot [string]) {
            $location = $location[0]
        }
        $Version = ($location -split '/')[-1]
    } elseif ($response.StatusCode -ge 300 -and $response.StatusCode -lt 400) {
        $location = $response.Headers["Location"]
        $Version = ($location -split '/')[-1]
    } else {
        # Fallback: parse the HTML for the latest release tag
        try {
            $html = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
            $Version = $html.tag_name
        } catch {
            Write-Host "Error: could not determine latest version. Set FOREMAN_VERSION explicitly."
            exit 1
        }
    }
    if (-not $Version) {
        Write-Host "Error: could not determine latest version. Set FOREMAN_VERSION explicitly."
        exit 1
    }
    Write-Host "    Detected version: $Version"
} else {
    $Version = $env:FOREMAN_VERSION
    Write-Host "==> Using provided version: $Version"
}

# Determine install directory
$InstallDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { Join-Path $HOME ".foreman\bin" }

# Detect architecture
$Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64"  { "amd64" }
    "x86"    { "amd64" }  # 32-bit process on 64-bit OS still reports x86
    "ARM64"  { "arm64" }
    default  {
        Write-Host "Error: unsupported architecture: $env:PROCESSOR_ARCHITECTURE"
        exit 1
    }
}

# Handle 32-bit process on 64-bit Windows
if ($env:PROCESSOR_ARCHITECTURE -eq "x86" -and $env:PROCESSOR_ARCHITEW6432 -eq "AMD64") {
    $Arch = "amd64"
} elseif ($env:PROCESSOR_ARCHITECTURE -eq "x86") {
    Write-Host "Error: 32-bit Windows is not supported"
    exit 1
}

$Binary = "foreman-windows-${Arch}.exe"
$Url = "$BaseUrl/$Version/$Binary"

Write-Host "==> Downloading foreman $Version (windows/$Arch)..."
Write-Host "    URL: $Url"

# Create install directory
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

$OutFile = Join-Path $InstallDir "foreman.exe"

try {
    Invoke-WebRequest -Uri $Url -OutFile $OutFile -UseBasicParsing
} catch {
    Write-Host "Error: download failed. Check that version $Version exists and the URL is correct."
    Write-Host "       $_"
    exit 1
}

Write-Host "==> Installed foreman to $OutFile"

# Verify the binary
try {
    $null = & $OutFile -h 2>&1
    Write-Host "    Verified: binary is valid and executable"
} catch {
    Write-Host "    Warning: could not verify the binary. It may not be compatible with this platform."
}

# Check if InstallDir is in PATH
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -split ";" | Where-Object { $_ -eq $InstallDir }) {
    Write-Host "==> $InstallDir is already in your PATH. You're all set!"
} else {
    Write-Host "==> Adding $InstallDir to your user PATH..."
    $NewPath = "$UserPath;$InstallDir"
    [Environment]::SetEnvironmentVariable("Path", $NewPath, "User")
    Write-Host "    Done. Restart your terminal for the change to take effect."
}
