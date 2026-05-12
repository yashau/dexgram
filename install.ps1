$ErrorActionPreference = "Stop"

if ([System.Environment]::OSVersion.Platform -ne [System.PlatformID]::Win32NT) {
    throw "Dexgram is Windows-only."
}

[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

$repo = "yashau/dexgram"
$isUpdate = @("1", "true", "yes") -contains ([string]$env:UPDATE).ToLowerInvariant()
$binDir = Join-Path $env:LOCALAPPDATA "Dexgram"
$configDir = Join-Path $env:APPDATA "Dexgram"
$exePath = Join-Path $binDir "dexgram.exe"
$logPath = Join-Path $configDir "dexgram.log"
$tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ("dexgram-install-" + [guid]::NewGuid().ToString("N"))

function Convert-DexgramVersion {
    param([string]$Value)
    $clean = $Value.Trim()
    if ($clean.StartsWith("v", [System.StringComparison]::OrdinalIgnoreCase)) {
        $clean = $clean.Substring(1)
    }
    $clean = ($clean -split '[-+]')[0]
    try {
        return [version]$clean
    } catch {
        return $null
    }
}

function Stop-DexgramForUpdate {
    if (-not $isUpdate) {
        return
    }
    $parentPid = 0
    if (-not [string]::IsNullOrWhiteSpace($env:DEXGRAM_UPDATE_PARENT_PID)) {
        [int]::TryParse($env:DEXGRAM_UPDATE_PARENT_PID, [ref]$parentPid) | Out-Null
    }

    $targets = @(Get-Process -Name "dexgram" -ErrorAction SilentlyContinue | Where-Object {
        try {
            [string]::Equals($_.Path, $exePath, [System.StringComparison]::OrdinalIgnoreCase)
        } catch {
            $_.Id -eq $parentPid
        }
    })
    if ($targets.Count -eq 0 -and $parentPid -gt 0) {
        $parent = Get-Process -Id $parentPid -ErrorAction SilentlyContinue
        if ($parent) {
            $targets = @($parent)
        }
    }
    if ($targets.Count -eq 0) {
        return
    }

    Write-Host "Stopping running Dexgram..."
    foreach ($process in $targets) {
        Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
    }
    foreach ($process in $targets) {
        try {
            $process.WaitForExit(10000) | Out-Null
        } catch {
        }
    }
    Start-Sleep -Milliseconds 500
}

function Copy-DexgramExecutable {
    param(
        [string]$Source,
        [string]$Destination
    )
    $lastError = $null
    for ($attempt = 1; $attempt -le 20; $attempt++) {
        try {
            Copy-Item -Path $Source -Destination $Destination -Force
            return
        } catch {
            $lastError = $_
            Start-Sleep -Milliseconds 250
        }
    }
    throw $lastError
}

if ($isUpdate) {
    Write-Host "Updating Dexgram..."
} else {
    Write-Host "Installing Dexgram..."
}

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

    if ($isUpdate -and (Test-Path $exePath)) {
        $installedInfo = [System.Diagnostics.FileVersionInfo]::GetVersionInfo($exePath)
        $installedVersion = Convert-DexgramVersion $installedInfo.FileVersion
        $latestVersion = Convert-DexgramVersion $release.tag_name
        if ($installedVersion -and $latestVersion -and $installedVersion -ge $latestVersion) {
            Write-Host "Dexgram is already up to date ($($installedInfo.FileVersion))."
            return
        }
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

    $sourceExe = $downloadPath
    if ($asset.name -match '(?i)\.zip$') {
        $extractDir = Join-Path $tmpDir "extract"
        Expand-Archive -Path $downloadPath -DestinationPath $extractDir -Force
        $candidate = Get-ChildItem -Path $extractDir -Filter "dexgram.exe" -Recurse -File | Select-Object -First 1
        if (-not $candidate) {
            throw "Release archive did not contain dexgram.exe."
        }
        $sourceExe = $candidate.FullName
    }
    Stop-DexgramForUpdate
    Copy-DexgramExecutable -Source $sourceExe -Destination $exePath
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

if ($isUpdate) {
    Write-Host ""
    Write-Host "Dexgram updated."
    Write-Host "Next: dexgram -check"
    Write-Host ""
    return
}

Write-Host ""
Write-Host "Starting Dexgram onboarding..."
$process = Start-Process -FilePath $exePath -ArgumentList "onboard" -NoNewWindow -Wait -PassThru
if ($process.ExitCode -ne 0) {
    throw "dexgram onboard failed with exit code $($process.ExitCode)."
}

Write-Host ""
Write-Host "Dexgram installed."
Write-Host "Next: dexgram -check"
