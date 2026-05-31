param(
    [Parameter(Position = 0)]
    [ValidateSet("patch", "minor", "major")]
    [string]$Bump = "patch",

    [switch]$DryRun
)

$ErrorActionPreference = "Stop"

function Fail($Message) {
    Write-Error "[WarpPool][release] $Message"
    exit 1
}

function RunGit($Arguments) {
    $output = & git @Arguments
    if ($LASTEXITCODE -ne 0) {
        Fail "git $($Arguments -join ' ') failed"
    }
    return $output
}

function Parse-Version($Value) {
    $parts = $Value.Trim() -split "\."
    if ($parts.Count -ne 3) {
        Fail "VERSION must be in MAJOR.MINOR.PATCH format, got: $Value"
    }
    return [int[]]$parts
}

if (-not (Test-Path "VERSION")) {
    Fail "VERSION file not found"
}

$branch = (RunGit @("branch", "--show-current")).Trim()
if ($branch -ne "developer") {
    Fail "release command must run on developer branch, current branch: $branch"
}

$status = RunGit @("status", "--porcelain")
if ($status) {
    Fail "working tree is not clean; commit or stash changes first"
}

RunGit @("fetch", "--tags", "origin") | Out-Null

$currentVersion = (Get-Content -Raw "VERSION").Trim()
$parts = Parse-Version $currentVersion

switch ($Bump) {
    "major" {
        $parts[0]++
        $parts[1] = 0
        $parts[2] = 0
    }
    "minor" {
        $parts[1]++
        $parts[2] = 0
    }
    default {
        $parts[2]++
    }
}

$nextVersion = "$($parts[0]).$($parts[1]).$($parts[2])"
$tag = "v$nextVersion"

$localTag = & git tag --list $tag
if ($localTag) {
    Fail "local tag already exists: $tag"
}

$remoteTag = & git ls-remote --tags origin "refs/tags/$tag"
if ($LASTEXITCODE -ne 0) {
    Fail "failed to query remote tag: $tag"
}
if ($remoteTag) {
    Fail "remote tag already exists: $tag"
}

if ($DryRun) {
    Write-Host "[WarpPool][release] dry-run: $currentVersion -> $nextVersion ($tag)"
    Write-Host "[WarpPool][release] dry-run: would update VERSION, commit, tag, and push developer + tag"
    exit 0
}

Set-Content -Path "VERSION" -Value $nextVersion -NoNewline

RunGit @("add", "VERSION") | Out-Null
RunGit @("commit", "-m", "发布 $tag") | Out-Null
RunGit @("tag", "-a", $tag, "-m", "发布 $tag") | Out-Null
RunGit @("push", "origin", "developer") | Out-Null
RunGit @("push", "origin", $tag) | Out-Null

Write-Host "[WarpPool][release] published $tag"
