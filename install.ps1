$ErrorActionPreference = "Stop"

if ([System.Environment]::OSVersion.Platform -ne [System.PlatformID]::Win32NT) {
    throw "Dexgram is Windows-only."
}

[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

$repo = "yashau/dexgram"
$binDir = Join-Path $env:LOCALAPPDATA "Dexgram"
$configDir = Join-Path $env:APPDATA "Dexgram"
$exePath = Join-Path $binDir "dexgram.exe"
$configPath = Join-Path $configDir "dexgram.toml"
$logPath = Join-Path $configDir "dexgram.log"
$statePath = Join-Path $configDir "dexgram.db"
$tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ("dexgram-install-" + [guid]::NewGuid().ToString("N"))

Write-Host "Installing Dexgram..."

New-Item -ItemType Directory -Force -Path $binDir, $configDir | Out-Null
if (-not (Test-Path $logPath)) {
    New-Item -ItemType File -Path $logPath | Out-Null
}

New-Item -ItemType Directory -Force -Path $tmpDir | Out-Null
try {
    $headers = @{
        "Accept" = "application/vnd.github+json"
        "User-Agent" = "dexgram-installer"
    }
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -Headers $headers
    $assets = @($release.assets)
    if ($assets.Count -eq 0) {
        throw "The latest release has no downloadable assets."
    }

    $asset = $assets |
        Where-Object { $_.name -match '(?i)\.exe$' } |
        Select-Object -First 1
    if (-not $asset) {
        $asset = $assets |
            Where-Object { $_.name -match '(?i)\.zip$' -and $_.name -match '(?i)(windows|win|amd64|x64)' } |
            Select-Object -First 1
    }
    if (-not $asset) {
        $asset = $assets |
            Where-Object { $_.name -match '(?i)\.zip$' } |
            Select-Object -First 1
    }
    if (-not $asset) {
        throw "Could not find a .exe or .zip asset in the latest release."
    }

    $downloadPath = Join-Path $tmpDir $asset.name
    Write-Host "Downloading $($asset.name) from $($release.tag_name)..."
    Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $downloadPath -Headers @{ "User-Agent" = "dexgram-installer" }

    if ($asset.name -match '(?i)\.zip$') {
        $extractDir = Join-Path $tmpDir "extract"
        Expand-Archive -Path $downloadPath -DestinationPath $extractDir -Force
        $candidate = Get-ChildItem -Path $extractDir -Filter "dexgram.exe" -Recurse -File | Select-Object -First 1
        if (-not $candidate) {
            throw "Release archive did not contain dexgram.exe."
        }
        Copy-Item -Path $candidate.FullName -Destination $exePath -Force
    } else {
        Copy-Item -Path $downloadPath -Destination $exePath -Force
    }
} finally {
    Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
}

$currentUserPath = [Environment]::GetEnvironmentVariable("Path", "User")
$pathEntries = @($currentUserPath -split ';' | Where-Object { $_ })
if ($pathEntries -notcontains $binDir) {
	$newUserPath = (($pathEntries + $binDir) -join ';')
	[Environment]::SetEnvironmentVariable("Path", $newUserPath, "User")
	Write-Host "Added $binDir to your user PATH."
} else {
	Write-Host "$binDir is already on your user PATH."
}
$env:Path = (($env:Path -split ';' | Where-Object { $_ }) + $binDir | Select-Object -Unique) -join ';'

Write-Host ""
Write-Host "Starting Dexgram onboarding..."
$process = Start-Process -FilePath $exePath -ArgumentList "onboard" -NoNewWindow -Wait -PassThru
if ($process.ExitCode -ne 0) {
    throw "dexgram onboard failed with exit code $($process.ExitCode)."
}

Write-Host ""
Write-Host "Dexgram installed."
Write-Host "Next: dexgram -config `"$configPath`" -check"
