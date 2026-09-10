#Requires -Version 7
<#
.SYNOPSIS
dsx.exe から MSIX パッケージと .appinstaller を生成する。

.DESCRIPTION
Windows SDK の makeappx.exe で dsx_x64.msix を生成し、dsx.appinstaller をテンプレートから出力する。
-PfxPath を指定した場合は signtool.exe で署名し、公開証明書 dsx.cer も出力する。
-PfxPath を省略した場合は未署名の MSIX を生成する（CI でのマニフェスト検証用）。

.EXAMPLE
pwsh packaging/msix/Build-Msix.ps1 -BinaryPath dist/dsx.exe -Tag v0.9.0 -OutDir dist/msix -PfxPath artifacts/signing/dsx-signing.pfx
#>
param(
    [Parameter(Mandatory)][string]$BinaryPath,
    [Parameter(Mandatory)][string]$Tag,
    [Parameter(Mandatory)][string]$OutDir,
    [string]$PfxPath,
    [string]$PfxPassword = $env:SIGNING_CERTIFICATE_PASSWORD
)
$ErrorActionPreference = "Stop"

# MSIX のバージョンは数値 4 要素のみのため、prerelease タグは受け付けない
if ($Tag -notmatch '^v(\d+)\.(\d+)\.(\d+)$') {
    throw "タグは v<major>.<minor>.<patch> 形式であること: $Tag"
}
$version = "$($Matches[1]).$($Matches[2]).$($Matches[3]).0"

function Find-SdkTool([string]$Name) {
    $tool = Get-ChildItem "${env:ProgramFiles(x86)}\Windows Kits\10\bin\*\x64\$Name" |
        Where-Object { $_.Directory.Parent.Name -as [version] } |
        Sort-Object { [version]$_.Directory.Parent.Name } |
        Select-Object -Last 1
    if (-not $tool) {
        throw "$Name が見つかりません（Windows 10 SDK が必要です）"
    }
    $tool.FullName
}

$BinaryPath = (Resolve-Path $BinaryPath).Path
$OutDir = (New-Item -ItemType Directory -Force $OutDir).FullName
$msixPath = Join-Path $OutDir "dsx_x64.msix"

$staging = Join-Path ([IO.Path]::GetTempPath()) "dsx-msix-$([guid]::NewGuid())"
New-Item -ItemType Directory $staging | Out-Null
try {
    Copy-Item $BinaryPath (Join-Path $staging "dsx.exe")
    Copy-Item (Join-Path $PSScriptRoot "Assets") $staging -Recurse
    (Get-Content (Join-Path $PSScriptRoot "AppxManifest.xml") -Raw).Replace("{VERSION}", $version) |
        Set-Content (Join-Path $staging "AppxManifest.xml") -NoNewline -Encoding utf8

    & (Find-SdkTool "makeappx.exe") pack /o /d $staging /p $msixPath
    if ($LASTEXITCODE -ne 0) {
        throw "makeappx による MSIX の生成に失敗しました（終了コード: $LASTEXITCODE）"
    }
}
finally {
    Remove-Item $staging -Recurse -Force
}

if ($PfxPath) {
    if (-not $PfxPassword) {
        throw "署名パスワードが未指定です（-PfxPassword または環境変数 SIGNING_CERTIFICATE_PASSWORD）"
    }
    $PfxPath = (Resolve-Path $PfxPath).Path

    & (Find-SdkTool "signtool.exe") sign /fd SHA256 /f $PfxPath /p $PfxPassword $msixPath
    if ($LASTEXITCODE -ne 0) {
        throw "signtool による署名に失敗しました（終了コード: $LASTEXITCODE）。証明書の Subject と AppxManifest.xml の Publisher が一致しているか確認してください"
    }

    $pfx = [Security.Cryptography.X509Certificates.X509Certificate2]::new($PfxPath, $PfxPassword)
    [IO.File]::WriteAllBytes((Join-Path $OutDir "dsx.cer"), $pfx.Export("Cert"))
}

(Get-Content (Join-Path $PSScriptRoot "dsx.appinstaller.template") -Raw).
    Replace("{VERSION}", $version).
    Replace("{TAG}", $Tag) |
    Set-Content (Join-Path $OutDir "dsx.appinstaller") -NoNewline -Encoding utf8

Write-Host "MSIX を生成しました（バージョン: $version、署名: $(if ($PfxPath) { 'あり' } else { 'なし' })）: $OutDir"
