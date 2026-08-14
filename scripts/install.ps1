<##
.SYNOPSIS
Installs the latest ALemonX release for the current Windows architecture.

.EXAMPLE
irm https://raw.githubusercontent.com/lemonade-lab/alemonjs-setup/main/scripts/install.ps1 | iex
#>

[CmdletBinding()]
param(
    [string]$Repository = $(if ($env:ALX_REPOSITORY) { $env:ALX_REPOSITORY } else { 'lemonade-lab/alemonx' }),
    [string]$InstallDir = $(if ($env:ALX_INSTALL_DIR) { $env:ALX_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\ALemonX' })
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
$downloadBase = "https://github.com/$Repository/releases/latest/download"
$temporary = Join-Path ([IO.Path]::GetTempPath()) ("alx-install-" + [guid]::NewGuid())

try {
    New-Item -ItemType Directory -Path $temporary | Out-Null
    $archive = Join-Path $temporary $asset
    $checksums = Join-Path $temporary 'SHA256SUMS'
    Write-Host "正在下载 ALemonX 最新版（windows/$architecture）…"
    Invoke-WebRequest -Uri "$downloadBase/$asset" -OutFile $archive
    Invoke-WebRequest -Uri "$downloadBase/SHA256SUMS" -OutFile $checksums

    $checksumLine = Get-Content $checksums | Where-Object { $_ -match ("\s" + [regex]::Escape($asset) + '$') } | Select-Object -First 1
    if (-not $checksumLine) { throw "校验文件中未找到 $asset。" }
    $expected = $checksumLine.Split()[0]
    $actual = (Get-FileHash -Algorithm SHA256 -Path $archive).Hash.ToLowerInvariant()
    if ($expected.ToLowerInvariant() -ne $actual) { throw '安装包校验失败，已取消安装。' }

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
