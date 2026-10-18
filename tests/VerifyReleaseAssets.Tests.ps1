# scripts/verify-release-assets.sh が、欠落・改ざん・別版の成果物を公開前に拒否することを固定する。
# アーカイブと MSIX の中身は任意のバイト列でよい（checksum とファイル名・版の整合だけを検証するため）。
# MSIX 本体の署名・Identity は TestReleaseMsix.Tests.ps1 が検証する。
BeforeDiscovery {
    $script:shAvailable = [bool](Get-Command sh -ErrorAction SilentlyContinue) -and
        [bool](Get-Command sha256sum -ErrorAction SilentlyContinue)
}

Describe "verify-release-assets.sh" -Skip:(-not $script:shAvailable) {
    BeforeAll {
        $script:verifyScript = (Resolve-Path "$PSScriptRoot/../scripts/verify-release-assets.sh").Path.Replace('\', '/')
        $script:version = "0.11.0"

        function Write-Checksums([string]$Dir, [string]$ListName, [string[]]$Names) {
            $lines = foreach ($name in $Names) {
                "{0}  {1}" -f (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $Dir $name)).Hash.ToLowerInvariant(), $name
            }
            [IO.File]::WriteAllText((Join-Path $Dir $ListName), ($lines -join "`n") + "`n", [Text.UTF8Encoding]::new($false))
        }

        function New-AssetDir([string]$Version, [switch]$WithMsix) {
            $dir = Join-Path ([IO.Path]::GetTempPath()) "verify-assets-$([guid]::NewGuid())"
            New-Item -ItemType Directory $dir | Out-Null

            $archives = @(
                "dsx_${Version}_linux_amd64.tar.gz", "dsx_${Version}_linux_arm64.tar.gz",
                "dsx_${Version}_darwin_amd64.tar.gz", "dsx_${Version}_darwin_arm64.tar.gz",
                "dsx_${Version}_windows_amd64.zip"
            )
            foreach ($name in $archives) {
                Set-Content -LiteralPath (Join-Path $dir $name) "archive $name"
            }
            Write-Checksums $dir "checksums.txt" $archives

            if ($WithMsix) {
                $msixFiles = @("dsx_x64.msix", "dsx.appinstaller", "dsx.cer", "install.ps1")
                Set-Content -LiteralPath (Join-Path $dir "dsx_x64.msix") "msix"
                Set-Content -LiteralPath (Join-Path $dir "dsx.cer") "cer"
                Set-Content -LiteralPath (Join-Path $dir "install.ps1") "install"
                Set-Content -LiteralPath (Join-Path $dir "dsx.appinstaller") @"
<AppInstaller Version="$Version.0">
  <MainPackage Version="$Version.0" Uri="https://github.com/scottlz0310/dsx/releases/download/v$Version/dsx_x64.msix" />
</AppInstaller>
"@
                Write-Checksums $dir "checksums-msix.txt" $msixFiles
            }

            $dir
        }

        function Invoke-Verify([string]$Dir, [string]$Version, [switch]$WithMsix) {
            $arguments = @($script:verifyScript, $Dir.Replace('\', '/'), $Version)
            if ($WithMsix) {
                $arguments += "--with-msix"
            }

            # sh は UTF-8 で日本語のメッセージを出す。既定のコンソール文字コード（Windows では CP932）のままだと文字化けして照合できない
            $previousEncoding = [Console]::OutputEncoding
            [Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
            try {
                $output = & sh @arguments 2>&1
                $exitCode = $LASTEXITCODE
            }
            finally {
                [Console]::OutputEncoding = $previousEncoding
            }

            [PSCustomObject]@{ ExitCode = $exitCode; Output = ($output -join "`n") }
        }
    }

    AfterAll {
        Get-ChildItem ([IO.Path]::GetTempPath()) -Directory -Filter "verify-assets-*" |
            Remove-Item -Recurse -Force -ErrorAction SilentlyContinue
    }

    Context "正常な成果物" {
        It "アーカイブだけを検証して成功する" {
            $dir = New-AssetDir $script:version
            (Invoke-Verify $dir $script:version).ExitCode | Should -Be 0
        }

        It "MSIX 一式も検証して成功する" {
            $dir = New-AssetDir $script:version -WithMsix
            (Invoke-Verify $dir $script:version -WithMsix).ExitCode | Should -Be 0
        }
    }

    Context "アーカイブの拒否" {
        It "<Name>" -ForEach @(
            @{ Name = "アーカイブが欠落している"; Mutate = { param($d, $v) Remove-Item (Join-Path $d "dsx_${v}_linux_arm64.tar.gz") }; Expected = "asset が無いか空です" }
            @{ Name = "アーカイブが空である"; Mutate = { param($d, $v) Set-Content -LiteralPath (Join-Path $d "dsx_${v}_darwin_amd64.tar.gz") -Value $null }; Expected = "asset が無いか空です" }
            @{ Name = "checksums.txt が無い"; Mutate = { param($d, $v) Remove-Item (Join-Path $d "checksums.txt") }; Expected = "asset が無いか空です" }
            @{ Name = "アーカイブが checksum 記録後に書き換えられた"; Mutate = { param($d, $v) Add-Content -LiteralPath (Join-Path $d "dsx_${v}_windows_amd64.zip") "tampered" }; Expected = "checksum が一致しません" }
            @{ Name = "別の版を指定した"; Mutate = { param($d, $v) }; Version = "9.9.9"; Expected = "asset が無いか空です" }
        ) {
            $dir = New-AssetDir $script:version
            & $Mutate $dir $script:version
            $checkVersion = if ($Version) { $Version } else { $script:version }

            $result = Invoke-Verify $dir $checkVersion
            $result.ExitCode | Should -Not -Be 0
            $result.Output | Should -Match $Expected
        }
    }

    Context "MSIX 一式の拒否" {
        It "<Name>" -ForEach @(
            @{ Name = "checksums-msix.txt が無い"; Mutate = { param($d) Remove-Item (Join-Path $d "checksums-msix.txt") }; Expected = "asset が無いか空です" }
            @{ Name = "MSIX が生成後に差し替えられた"; Mutate = { param($d) Set-Content -LiteralPath (Join-Path $d "dsx_x64.msix") "replaced" }; Expected = "MSIX 一式の checksum が一致しません" }
            @{ Name = "公開証明書が生成後に差し替えられた"; Mutate = { param($d) Set-Content -LiteralPath (Join-Path $d "dsx.cer") "replaced" }; Expected = "MSIX 一式の checksum が一致しません" }
            @{ Name = "install.ps1 が生成後に書き換えられた"; Mutate = { param($d) Add-Content -LiteralPath (Join-Path $d "install.ps1") "tampered" }; Expected = "MSIX 一式の checksum が一致しません" }
            @{ Name = "checksums-msix.txt に MSIX の記載が無い"; Mutate = { param($d) Set-Content -LiteralPath (Join-Path $d "checksums-msix.txt") (Get-Content (Join-Path $d "checksums-msix.txt") | Where-Object { $_ -notmatch "dsx_x64.msix" }) }; Expected = "checksums-msix.txt に記載がありません" }
            @{ Name = "App Installer が別の版を指している"; Mutate = { param($d) Set-Content -LiteralPath (Join-Path $d "dsx.appinstaller") '<AppInstaller Version="0.10.2.0"><MainPackage Version="0.10.2.0" Uri="https://example.invalid/v0.10.2/dsx_x64.msix" /></AppInstaller>'; $names = "dsx_x64.msix", "dsx.appinstaller", "dsx.cer", "install.ps1"; Write-Checksums $d "checksums-msix.txt" $names }; Expected = "dsx.appinstaller" }
        ) {
            $dir = New-AssetDir $script:version -WithMsix
            & $Mutate $dir

            $result = Invoke-Verify $dir $script:version -WithMsix
            $result.ExitCode | Should -Not -Be 0
            $result.Output | Should -Match $Expected
        }
    }
}
