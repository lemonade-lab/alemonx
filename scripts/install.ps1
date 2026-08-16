<##
.SYNOPSIS
Installs the latest ALemonX release for the current Windows architecture.

.EXAMPLE
irm https://raw.githubusercontent.com/lemonade-lab/alemonx/main/scripts/install.ps1 | iex
#>

[CmdletBinding()]
param(
    [string]$Repository = $(if ($env:ALX_REPOSITORY) { $env:ALX_REPOSITORY } else { 'lemonade-lab/alemonx' }),
    [string]$InstallDir = $(if ($env:ALX_INSTALL_DIR) { $env:ALX_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\ALemonX' }),
    [string]$DownloadBase = $env:ALX_DOWNLOAD_BASE,
    [bool]$PreferMirror = ($env:ALX_PREFER_MIRROR -eq '1')
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Get-Architecture {
    $value = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    switch ($value.ToUpperInvariant()) {
        'AMD64' { return 'amd64' }
        'ARM64' { return 'arm64' }
        'X86' { return '386' }
        default { throw "暂不支持 $value 架构。" }
    }
}

function Add-UserPath([string]$Directory) {
    $current = [Environment]::GetEnvironmentVariable('Path', 'User')
    $items = @($current -split ';' | Where-Object { $_ })
    if ($items -notcontains $Directory) {
        [Environment]::SetEnvironmentVariable('Path', (($items + $Directory) -join ';'), 'User')
    }
    if ((@($env:Path -split ';') -notcontains $Directory)) {
        $env:Path = "$env:Path;$Directory"
    }
}

$architecture = Get-Architecture
$asset = "alx-windows-$architecture.zip"
$officialDownloadBase = "https://github.com/$Repository/releases/latest/download"
$mirrorDownloadBases = @(
    "https://ghfast.top/https://github.com/$Repository/releases/latest/download"
    "https://ghproxy.net/https://github.com/$Repository/releases/latest/download"
    "https://gh-proxy.com/https://github.com/$Repository/releases/latest/download"
)
$downloadSources = @(
    if ($DownloadBase) { $DownloadBase.TrimEnd('/') }
    if ($PreferMirror) { $mirrorDownloadBases; $officialDownloadBase }
    else { $officialDownloadBase; $mirrorDownloadBases }
) | Select-Object -Unique
if ($PreferMirror) { Write-Host '国内镜像优先模式：将优先尝试 ghfast.top 等镜像源，官方源作为兜底。' }

foreach ($downloadSource in $downloadSources) {
    try {
        $downloadSourceUri = [uri]$downloadSource
        if (-not $downloadSourceUri.IsAbsoluteUri -or $downloadSourceUri.Scheme -ne 'https') { throw 'not HTTPS' }
    }
    catch {
        throw "下载源必须是 HTTPS 地址：$downloadSource"
    }
}
$temporary = Join-Path ([IO.Path]::GetTempPath()) ("alx-install-" + [guid]::NewGuid())

try {
    New-Item -ItemType Directory -Path $temporary | Out-Null
    $archive = Join-Path $temporary $asset
    $checksums = Join-Path $temporary 'SHA256SUMS'
    Write-Host "正在下载 ALemonX 最新版（windows/$architecture）…"
    $selectedDownloadSource = $null
    foreach ($downloadSource in $downloadSources) {
        try {
            Write-Host "尝试下载源：$downloadSource"
            Remove-Item -LiteralPath $archive, $checksums -Force -ErrorAction SilentlyContinue
            Invoke-WebRequest -Uri "$downloadSource/$asset" -OutFile $archive
            Invoke-WebRequest -Uri "$downloadSource/SHA256SUMS" -OutFile $checksums

            $checksumLine = Get-Content $checksums | Where-Object { $_ -match ("\s" + [regex]::Escape($asset) + '$') } | Select-Object -First 1
            if (-not $checksumLine) { throw "校验文件中未找到 $asset。" }
            $expected = $checksumLine.Split()[0]
            $actual = (Get-FileHash -Algorithm SHA256 -Path $archive).Hash.ToLowerInvariant()
            if ($expected.ToLowerInvariant() -ne $actual) { throw '安装包校验失败。' }

            $selectedDownloadSource = $downloadSource
            break
        }
        catch {
            Write-Warning "下载源不可用，尝试下一个：$downloadSource"
        }
    }
    if (-not $selectedDownloadSource) { throw "官方下载源和备用镜像均不可用，或未找到适用于 windows/$architecture 的校验安装包。" }

    $unpacked = Join-Path $temporary 'package'
    Expand-Archive -Path $archive -DestinationPath $unpacked
    $binary = Join-Path $unpacked 'alx.exe'
    if (-not (Test-Path -LiteralPath $binary)) { throw '安装包中缺少 alx.exe。' }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item -LiteralPath $binary -Destination (Join-Path $InstallDir 'alx.exe') -Force
    Add-UserPath $InstallDir
    Write-Host "ALemonX 已安装到 $(Join-Path $InstallDir 'alx.exe')"
    Write-Host '请重新打开 PowerShell 后运行：alx'
}
finally {
    if (Test-Path -LiteralPath $temporary) { Remove-Item -LiteralPath $temporary -Recurse -Force }
}
