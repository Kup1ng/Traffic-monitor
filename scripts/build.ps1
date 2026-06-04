# Build the single static Traffic-monitor-amd64 binary (linux/amd64) with the
# Nuxt frontend embedded. Usage: scripts\build.ps1 [version]
#
# Note: this avoids $ErrorActionPreference='Stop' because Windows PowerShell 5.1
# treats a native command's stderr (e.g. npm warnings) as a terminating error.
# Failures are detected via exit codes and artifact checks instead.
#requires -Version 5

$root = Split-Path -Parent $PSScriptRoot
$version = if ($args.Count -ge 1) { $args[0] } else { 'dev' }
$embed = Join-Path $root 'backend\web\public'
$out = Join-Path $root 'Traffic-monitor-amd64'

function Clear-Embed {
  Get-ChildItem $embed -Exclude 'index.html' -ErrorAction SilentlyContinue |
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Host '==> Building frontend (nuxt generate)'
Push-Location (Join-Path $root 'frontend')
try {
  if (-not (Test-Path 'node_modules')) { npm install --no-audit --no-fund }
  npm run generate
} finally { Pop-Location }
if (-not (Test-Path (Join-Path $root 'frontend\.output\public\_nuxt'))) {
  throw 'frontend build failed: .output/public/_nuxt not found'
}

Write-Host '==> Staging embedded assets into backend/web/public'
Clear-Embed
Copy-Item -Path (Join-Path $root 'frontend\.output\public\*') -Destination $embed -Recurse -Force
if (-not (Test-Path (Join-Path $embed '_nuxt'))) {
  throw 'embed staging failed: _nuxt not copied'
}

Write-Host '==> Building Go binary (linux/amd64, CGO disabled, static)'
$env:CGO_ENABLED = '0'; $env:GOOS = 'linux'; $env:GOARCH = 'amd64'
go -C (Join-Path $root 'backend') build -trimpath -ldflags "-s -w -X main.version=$version" -o $out ./cmd/traffic-monitor
$code = $LASTEXITCODE

Write-Host '==> Restoring working tree'
Clear-Embed
git -C $root checkout -- backend/web/public/index.html 2>$null

if ($code -ne 0) { throw "go build failed (exit $code)" }
Write-Host "Built $out (version $version)"
