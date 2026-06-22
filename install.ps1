$ErrorActionPreference = 'Stop'

Write-Host "=== Quest Installer (Windows PowerShell Version) ===" -ForegroundColor Green

$questHome = Join-Path $env:USERPROFILE ".quest"
$binDir = Join-Path $questHome "bin"
$appsDir = Join-Path $questHome "apps"
$cacheDir = Join-Path $questHome "cache"
$manifestsDir = Join-Path $questHome "manifests"
$receiptsDir = Join-Path $questHome "receipts"

Write-Host "Creating Quest directories..."
New-Item -ItemType Directory -Force -Path $binDir | Out-Null
New-Item -ItemType Directory -Force -Path $appsDir | Out-Null
New-Item -ItemType Directory -Force -Path $cacheDir | Out-Null
New-Item -ItemType Directory -Force -Path $manifestsDir | Out-Null
New-Item -ItemType Directory -Force -Path $receiptsDir | Out-Null

$exeDest = Join-Path $binDir "quest.exe"

if (Test-Path "quest.exe") {
    Write-Host "Copying local quest.exe to installation directory..." -ForegroundColor Cyan
    Copy-Item "quest.exe" $exeDest -Force
} elseif (Test-Path "bin\quest.exe") {
    Write-Host "Copying local bin\quest.exe to installation directory..." -ForegroundColor Cyan
    Copy-Item "bin\quest.exe" $exeDest -Force
} else {
    $downloadUrl = "https://github.com/akhilesh/quest/releases/latest/download/quest.exe"
    Write-Host "No local quest.exe found. To download the latest release, run:" -ForegroundColor Yellow
    Write-Host "Invoke-WebRequest -Uri '$downloadUrl' -OutFile '$exeDest'" -ForegroundColor Cyan
    Write-Host "For development, compile it on Mac and copy quest.exe to your Windows PC." -ForegroundColor Yellow
}

Write-Host "Checking PATH environment variable..."
$userPath = [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::User)
if ($userPath -split ';' -contains $binDir) {
    Write-Host "Quest bin directory is already in your PATH." -ForegroundColor Green
} else {
    Write-Host "Adding Quest bin directory to User PATH..." -ForegroundColor Cyan
    $newPath = $userPath
    if ($newPath -and !$newPath.EndsWith(';')) {
         $newPath += ';'
    }
    $newPath += $binDir
    [Environment]::SetEnvironmentVariable("Path", $newPath, [EnvironmentVariableTarget]::User)
    Write-Host "Successfully added Quest to User PATH!" -ForegroundColor Green
    Write-Host "Note: Please restart your terminal/PowerShell window for the PATH changes to take effect." -ForegroundColor Yellow
}

Write-Host "=== Quest Setup Complete ===" -ForegroundColor Green
Write-Host "Verify installation by opening a NEW PowerShell window and running:"
Write-Host "  quest search jq" -ForegroundColor Cyan
