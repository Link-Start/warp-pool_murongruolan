param(
    [ValidateSet("patch", "minor", "major")]
    [string]$Part = "patch"
)

$ErrorActionPreference = "Stop"

function Fail($Message) {
    Write-Error $Message
    exit 1
}

$branch = (git branch --show-current).Trim()
if ($branch -ne "developer") {
    Fail "请在 developer 分支执行发布脚本，当前分支：$branch"
}

$status = (git status --porcelain).Trim()
if ($status -ne "") {
    Fail "工作区不干净，请先提交或暂存当前修改"
}

if (!(Test-Path VERSION)) {
    Fail "VERSION 文件不存在"
}

$version = (Get-Content VERSION -Raw).Trim()
if ($version -notmatch '^\d+\.\d+\.\d+$') {
    Fail "VERSION 必须是单行语义化版本号，例如 0.1.0"
}

$parts = $version.Split(".") | ForEach-Object { [int]$_ }
switch ($Part) {
    "major" {
        $parts[0]++
        $parts[1] = 0
        $parts[2] = 0
    }
    "minor" {
        $parts[1]++
        $parts[2] = 0
    }
    "patch" {
        $parts[2]++
    }
}

$next = "$($parts[0]).$($parts[1]).$($parts[2])"
$tag = "v$next"

if ((git tag -l $tag).Trim() -ne "") {
    Fail "本地 tag 已存在：$tag"
}

$remoteTag = (git ls-remote --tags origin "refs/tags/$tag").Trim()
if ($remoteTag -ne "") {
    Fail "远程 tag 已存在：$tag"
}

Set-Content -Path VERSION -Value $next -NoNewline
git add VERSION
git commit -m "发布版本 $tag"
git tag -a $tag -m "发布版本 $tag"

Write-Host "版本已更新：$tag"
Write-Host "推送命令：git push origin developer $tag"
