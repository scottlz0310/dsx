# Pester 5+ ではトップレベル変数は It から見えないため、AST 解析と関数定義の読み込みを BeforeAll で行う
BeforeAll {
    $scriptPath = (Resolve-Path "$PSScriptRoot/../scripts/install.ps1").Path
    $tokens = $null
    $parseErrors = $null
    $scriptAst = [System.Management.Automation.Language.Parser]::ParseFile(
        $scriptPath,
        [ref]$tokens,
        [ref]$parseErrors
    )

    if ($parseErrors.Count -gt 0) {
        throw "install.ps1 の構文解析に失敗しました: $($parseErrors -join '; ')"
    }

    $functionAsts = $scriptAst.FindAll(
        {
            param($node)
            $node -is [System.Management.Automation.Language.FunctionDefinitionAst]
        },
        $true
    )
    foreach ($functionAst in $functionAsts) {
        Invoke-Expression $functionAst.Extent.Text
    }

    function New-TestInstallHarness {
        param(
            [bool]$InitiallyTrusted = $false,
            [bool]$LegacyExists = $false
        )

        $state = [PSCustomObject]@{
            Trusted               = $InitiallyTrusted
            LegacyExists          = $LegacyExists
            DownloadUrls          = [System.Collections.Generic.List[string]]::new()
            StartProcessCalls     = 0
            StartProcessFilePath  = $null
            StartProcessVerb      = $null
            StartProcessArguments = $null
            InstallAppxCalls      = 0
            InstalledPath         = $null
            AppInstallerFile      = $false
        }

        $downloadStep = {
            param([string]$Uri, [string]$OutFile)
            $null = $state.DownloadUrls.Add($Uri)
            [IO.File]::WriteAllText($OutFile, "test")
        }.GetNewClosure()

        $certificateThumbprintStep = {
            param([string]$Path)
            return "aa bb cc"
        }.GetNewClosure()

        $certificateStoreStep = {
            if ($state.Trusted) {
                return [PSCustomObject]@{ Thumbprint = "AABBCC" }
            }
        }.GetNewClosure()

        $startProcessStep = {
            param(
                [string]$FilePath,
                [string]$Verb,
                [string[]]$ArgumentList,
                [switch]$Wait,
                [switch]$PassThru
            )
            $state.StartProcessCalls++
            $state.StartProcessFilePath = $FilePath
            $state.StartProcessVerb = $Verb
            $state.StartProcessArguments = $ArgumentList
            $state.Trusted = $true
            return [PSCustomObject]@{ ExitCode = 0 }
        }.GetNewClosure()

        $installAppxStep = {
            param([string]$Path, [switch]$AppInstallerFile)
            $state.InstallAppxCalls++
            $state.InstalledPath = $Path
            $state.AppInstallerFile = [bool]$AppInstallerFile
        }.GetNewClosure()

        $pathExistsStep = {
            param([string]$LiteralPath)
            return $state.LegacyExists
        }.GetNewClosure()

        return [PSCustomObject]@{
            State                     = $state
            DownloadStep              = $downloadStep
            CertificateThumbprintStep = $certificateThumbprintStep
            CertificateStoreStep      = $certificateStoreStep
            StartProcessStep          = $startProcessStep
            InstallAppxStep           = $installAppxStep
            PathExistsStep            = $pathExistsStep
        }
    }
}

Describe "scripts/install.ps1" {
    It "スクリプトに直接の exit 呼び出しを含めない" {
        $scriptText = Get-Content -Raw $scriptPath
        $scriptText -notmatch "(?im)^\s*exit(?:\s|$)" | Should -BeTrue
    }

    It "アセット URL の末尾スラッシュを正規化する" -TestCases @(
        @{
            BaseUrl   = "https://example.test/releases/latest/download"
            AssetName = "dsx.cer"
            Expected  = "https://example.test/releases/latest/download/dsx.cer"
        },
        @{
            BaseUrl   = "https://example.test/releases/latest/download/"
            AssetName = "dsx.appinstaller"
            Expected  = "https://example.test/releases/latest/download/dsx.appinstaller"
        }
    ) {
        param($BaseUrl, $AssetName, $Expected)
        Get-DsxAssetUrl -BaseUrl $BaseUrl -AssetName $AssetName | Should -Be $Expected
    }

    It "証明書が未信頼の場合だけ昇格し、MSIX はローカルファイルを元ユーザーで渡す" -TestCases @(
        @{
            InitiallyTrusted  = $false
            ExpectedElevation = 1
        },
        @{
            InitiallyTrusted  = $true
            ExpectedElevation = 0
        }
    ) {
        param($InitiallyTrusted, $ExpectedElevation)
        $harness = New-TestInstallHarness -InitiallyTrusted $InitiallyTrusted
        $workDir = Join-Path $TestDrive "install"

        $null = Install-Dsx -BaseUrl "https://example.test/releases/latest/download/" -WorkDir $workDir -DownloadStep $harness.DownloadStep -CertificateThumbprintStep $harness.CertificateThumbprintStep -CertificateStoreStep $harness.CertificateStoreStep -StartProcessStep $harness.StartProcessStep -InstallAppxStep $harness.InstallAppxStep -PathExistsStep $harness.PathExistsStep -LegacyExecutablePath (Join-Path $TestDrive "go\bin\dsx.exe")

        $harness.State.StartProcessCalls | Should -Be $ExpectedElevation
        $harness.State.DownloadUrls | Should -HaveCount 2
        $harness.State.DownloadUrls[0] | Should -Be "https://example.test/releases/latest/download/dsx.cer"
        $harness.State.DownloadUrls[1] | Should -Be "https://example.test/releases/latest/download/dsx.appinstaller"
        $harness.State.InstallAppxCalls | Should -Be 1
        $harness.State.InstalledPath | Should -Be (Join-Path $workDir "dsx.appinstaller")
        $harness.State.AppInstallerFile | Should -BeTrue
    }

    It "昇格した子プロセスには引用符をエスケープした UTF-16LE のコマンドを渡す" {
        $harness = New-TestInstallHarness
        $certificatePath = "C:\Users\O'Brien\dsx.cer"

        Invoke-DsxElevatedCertificateImport -CertificatePath $certificatePath -Thumbprint "AABBCC" -StartProcessStep $harness.StartProcessStep

        $harness.State.StartProcessFilePath | Should -Be "powershell.exe"
        $harness.State.StartProcessVerb | Should -Be "RunAs"
        $harness.State.StartProcessArguments[3] | Should -Be "-EncodedCommand"
        $encoded = $harness.State.StartProcessArguments[4]
        $decoded = [Text.Encoding]::Unicode.GetString([Convert]::FromBase64String($encoded))
        $decoded.Contains("C:\Users\O''Brien\dsx.cer") | Should -BeTrue
        $decoded | Should -Match "Import-Certificate"
        $decoded | Should -Match "Cert:\\LocalMachine\\TrustedPeople"
    }

    It "管理者プロセスの失敗を証明書登録エラーとして伝播する" {
        $failedStartProcess = {
            param(
                [string]$FilePath,
                [string]$Verb,
                [string[]]$ArgumentList,
                [switch]$Wait,
                [switch]$PassThru
            )
            return [PSCustomObject]@{ ExitCode = 1 }
        }

        {
            Invoke-DsxElevatedCertificateImport -CertificatePath "C:\temp\dsx.cer" -Thumbprint "AABBCC" -StartProcessStep $failedStartProcess
        } | Should -Throw "*証明書の信頼登録に失敗しました*"
    }

    It "証明書の thumbprint を大文字小文字や空白に依存せず比較する" -TestCases @(
        @{
            Thumbprint   = "aa bb cc"
            Certificates = @([PSCustomObject]@{ Thumbprint = "AABBCC" })
            Expected     = $true
        },
        @{
            Thumbprint   = "AABBCC"
            Certificates = @([PSCustomObject]@{ Thumbprint = "DDEEFF" })
            Expected     = $false
        },
        @{
            Thumbprint   = "AABBCC"
            Certificates = @()
            Expected     = $false
        }
    ) {
        param($Thumbprint, $Certificates, $Expected)
        $storeStep = {
            return $Certificates
        }.GetNewClosure()

        Test-DsxCertificateTrusted -Thumbprint $Thumbprint -CertificateStoreStep $storeStep | Should -Be $Expected
    }

    It "go install 版の残存を警告する" {
        $harness = New-TestInstallHarness -InitiallyTrusted $true -LegacyExists $true
        $warning = $null

        $null = Install-Dsx -BaseUrl "https://example.test/releases/latest/download" -WorkDir (Join-Path $TestDrive "install") -DownloadStep $harness.DownloadStep -CertificateThumbprintStep $harness.CertificateThumbprintStep -CertificateStoreStep $harness.CertificateStoreStep -StartProcessStep $harness.StartProcessStep -InstallAppxStep $harness.InstallAppxStep -PathExistsStep $harness.PathExistsStep -LegacyExecutablePath (Join-Path $TestDrive "go\bin\dsx.exe") -WarningVariable warning -WarningAction Continue

        ($warning -join [Environment]::NewLine) | Should -Match "go install"
    }
}
