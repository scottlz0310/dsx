#Requires -Version 7
<#
.SYNOPSIS
MSIX 署名用の自己署名証明書を生成し、GitHub Secrets 登録用の値を出力する。

.DESCRIPTION
AppxManifest.xml の Publisher と一致する Subject の自己署名コード署名証明書を生成し、
artifacts/signing/ に pfx（秘密鍵つき・パスワード保護）・cer（公開証明書）・
pfx の Base64 文字列を出力する。生成後、証明書は証明書ストアから削除される（pfx が唯一の保管物）。

出力された値を GitHub リポジトリの Secrets に登録する:
  - SIGNING_CERTIFICATE_BASE64   … pfx-base64.txt の内容
  - SIGNING_CERTIFICATE_PASSWORD … コンソールに表示されるパスワード

.NOTES
リリースワークフロー（.github/workflows/release.yml）がこれらの Secrets を使って MSIX を署名する。
artifacts/ は .gitignore 対象。pfx とパスワードはリポジトリにコミットしないこと。
dist/ は goreleaser --clean で削除されるため、出力先に使わないこと。
#>
param(
    [string]$Subject = "CN=scottlz0310",
    [string]$OutDir = (Join-Path $PSScriptRoot "..\..\artifacts\signing"),
    [int]$ValidYears = 5
)
$ErrorActionPreference = "Stop"

$OutDir = (New-Item -ItemType Directory -Force $OutDir).FullName

$passwordBytes = [byte[]]::new(24)
[System.Security.Cryptography.RandomNumberGenerator]::Fill($passwordBytes)
$password = [Convert]::ToBase64String($passwordBytes)
$securePassword = ConvertTo-SecureString $password -AsPlainText -Force

$cert = New-SelfSignedCertificate `
    -Type Custom `
    -Subject $Subject `
    -KeyUsage DigitalSignature `
    -FriendlyName "dsx MSIX signing" `
    -CertStoreLocation "Cert:\CurrentUser\My" `
    -TextExtension @("2.5.29.37={text}1.3.6.1.5.5.7.3.3", "2.5.29.19={text}") `
    -NotAfter (Get-Date).AddYears($ValidYears)

try {
    $pfxPath = Join-Path $OutDir "dsx-signing.pfx"
    $cerPath = Join-Path $OutDir "dsx.cer"

    Export-PfxCertificate -Cert $cert -FilePath $pfxPath -Password $securePassword | Out-Null
    Export-Certificate -Cert $cert -FilePath $cerPath | Out-Null
    [Convert]::ToBase64String([IO.File]::ReadAllBytes($pfxPath)) |
        Set-Content (Join-Path $OutDir "pfx-base64.txt") -NoNewline
}
finally {
    # pfx を唯一の保管物にするため、ストアには残さない
    Remove-Item "Cert:\CurrentUser\My\$($cert.Thumbprint)" -Force
}

Write-Output ""
Write-Output "証明書を生成しました（有効期限: $($cert.NotAfter.ToString('yyyy-MM-dd'))、Subject: $Subject、Thumbprint: $($cert.Thumbprint)）"
Write-Output ""
Write-Output "出力先: $OutDir"
Write-Output "  dsx-signing.pfx … 秘密鍵つき証明書（要保管・コミット禁止）"
Write-Output "  dsx.cer         … 公開証明書"
Write-Output "  pfx-base64.txt  … Secrets 登録用 Base64"
Write-Output ""
Write-Output "GitHub Secrets に以下を登録してください:"
Write-Output "  SIGNING_CERTIFICATE_BASE64   = pfx-base64.txt の内容"
Write-Output "  SIGNING_CERTIFICATE_PASSWORD = $password"
Write-Output ""
Write-Output "登録例:"
Write-Output '  Get-Content artifacts\signing\pfx-base64.txt -Raw | gh secret set SIGNING_CERTIFICATE_BASE64 -R scottlz0310/dsx'
Write-Output '  gh secret set SIGNING_CERTIFICATE_PASSWORD -R scottlz0310/dsx  # 対話入力で上記パスワードを貼り付け'
