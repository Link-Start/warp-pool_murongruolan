param(
    [ValidateSet("amd64", "arm64")]
    [string]$Arch = "amd64",

    [string]$OutputDir = "dist"
)

$ErrorActionPreference = "Stop"

function Fail($Message) {
    Write-Error $Message
    exit 1
}

if (!(Test-Path VERSION)) {
    Fail "VERSION 文件不存在"
}

if (!(Test-Path assets)) {
    Fail "assets 目录不存在"
}

$version = (Get-Content VERSION -Raw).Trim()
if ($version -notmatch '^\d+\.\d+\.\d+$') {
    Fail "VERSION 必须是单行语义化版本号，例如 0.1.0"
}

$commit = (git rev-parse HEAD).Trim()
$date = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$packageName = "warppool-linux-$Arch"
$workDir = Join-Path $OutputDir $packageName
$packagePath = Join-Path $OutputDir "$packageName.tar.gz"

if (Test-Path $workDir) {
    $resolvedOutput = [System.IO.Path]::GetFullPath($OutputDir)
    $resolvedWork = [System.IO.Path]::GetFullPath($workDir)
    if (!$resolvedWork.StartsWith($resolvedOutput)) {
        Fail "refusing to remove unexpected work dir: $workDir"
    }
    Remove-Item -Recurse -Force $workDir
}
New-Item -ItemType Directory -Force (Join-Path $workDir "assets") | Out-Null

$env:GOOS = "linux"
$env:GOARCH = $Arch
$env:CGO_ENABLED = "0"

go build `
    -ldflags "-s -w -X github.com/murongruolan/warp-pool/internal/cli.version=v$version -X github.com/murongruolan/warp-pool/internal/cli.commit=$commit -X github.com/murongruolan/warp-pool/internal/cli.date=$date" `
    -o (Join-Path $workDir "warppool") `
    ./cmd/warppool

Copy-Item -Recurse -Force "assets\*" (Join-Path $workDir "assets")
Set-Content -Path (Join-Path $workDir "VERSION") -Value $version -NoNewline

if (Test-Path $packagePath) {
    Remove-Item -Force $packagePath
}

tar -C $OutputDir -czf $packagePath $packageName

Write-Host "local package created: $packagePath"
