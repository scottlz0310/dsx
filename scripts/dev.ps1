<#
.SYNOPSIS
    DevSync 開発タスクランナー

.DESCRIPTION
    Go プロジェクトの品質チェック、テスト、ビルドを実行します。

.EXAMPLE
    .\scripts\dev.ps1 -Task test
    .\scripts\dev.ps1 -Task check
    .\scripts\dev.ps1 -Task coverage
#>

param(
    [Parameter(Position=0)]
    [ValidateSet("help", "build", "test", "test-verbose", "coverage", "coverage-check", 
                 "fmt", "fmt-check", "vet", "lint", "check", "dev", "pre-commit", "clean")]
    [string]$Task = "help"
)

# 設定
$BinaryName = "devsync.exe"
$CoverageFile = "coverage.out"
$CoverageHtml = "coverage.html"
$CoverageThreshold = 30  # 現状に合わせた閾値（徐々に上げる）
$GoPackages = @("./cmd/...", "./internal/...")

function Show-Help {
    Write-Host "DevSync 開発コマンド" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "使用方法: .\scripts\dev.ps1 -Task <タスク名>" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "ビルド:" -ForegroundColor Green
    Write-Host "  build       - バイナリをビルド"
    Write-Host "  clean       - ビルド成果物を削除"
    Write-Host ""
    Write-Host "テスト:" -ForegroundColor Green
    Write-Host "  test        - 全テストを実行"
    Write-Host "  test-verbose- 詳細出力でテスト実行"
    Write-Host "  coverage    - カバレッジレポート生成"
    Write-Host "  coverage-check - カバレッジ閾値チェック ($CoverageThreshold%)"
    Write-Host ""
    Write-Host "品質チェック:" -ForegroundColor Green
    Write-Host "  fmt         - コードフォーマット (gofmt)"
    Write-Host "  fmt-check   - フォーマットチェック"
    Write-Host "  vet         - 静的解析 (go vet)"
    Write-Host "  lint        - リンター実行 (golangci-lint)"
    Write-Host "  check       - 全品質チェック (CI相当)"
    Write-Host ""
    Write-Host "開発サイクル:" -ForegroundColor Green
    Write-Host "  dev         - フォーマット→テスト→ビルド"
    Write-Host "  pre-commit  - コミット前チェック"
}

function Invoke-Build {
    Write-Host "🔨 ビルド中..." -ForegroundColor Cyan
    go build -o $BinaryName ./cmd/devsync
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✅ ビルド完了: $BinaryName" -ForegroundColor Green
    } else {
        Write-Host "❌ ビルド失敗" -ForegroundColor Red
        exit 1
    }
}

function Invoke-Clean {
    Write-Host "🧹 クリーンアップ中..." -ForegroundColor Cyan
    Remove-Item -Path $BinaryName, $CoverageFile, $CoverageHtml -ErrorAction SilentlyContinue
    Write-Host "✅ クリーンアップ完了" -ForegroundColor Green
}

function Invoke-Test {
    param([switch]$Verbose)
    
    Write-Host "🧪 テスト実行中..." -ForegroundColor Cyan
    
    # Windowsではraceフラグをスキップ（CGOが必要なため）
    $raceFlag = ""
    if ($env:CGO_ENABLED -eq "1" -or (-not ($IsWindows -or $env:OS -eq "Windows_NT"))) {
        $raceFlag = "-race"
    }
    
    if ($Verbose) {
        go test $GoPackages $raceFlag -shuffle=on -v
    } else {
        go test $GoPackages $raceFlag -shuffle=on
    }
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✅ 全テストパス" -ForegroundColor Green
    } else {
        Write-Host "❌ テスト失敗" -ForegroundColor Red
        exit 1
    }
}

function Invoke-Coverage {
    param([switch]$CheckThreshold)
    
    Write-Host "📊 カバレッジ計測中..." -ForegroundColor Cyan
    
    # カバレッジファイルのフルパス
    $coverFile = Join-Path $PSScriptRoot "..\coverage.out"
    $coverHtml = Join-Path $PSScriptRoot "..\coverage.html"
    
    # テストがあるパッケージのみ計測
    go test "./internal/env" "./internal/secret" -coverprofile="$coverFile" -covermode=atomic
    
    if ($LASTEXITCODE -ne 0) {
        Write-Host "❌ テスト失敗" -ForegroundColor Red
        exit 1
    }
    
    Write-Host ""
    Write-Host "パッケージ別カバレッジ:" -ForegroundColor Yellow
    go tool cover -func "$coverFile"
    
    # HTMLレポート生成
    go tool cover -html="$coverFile" -o "$coverHtml"
    Write-Host ""
    Write-Host "📄 HTMLレポート: $coverHtml" -ForegroundColor Cyan
    
    if ($CheckThreshold) {
        Write-Host ""
        Write-Host "🎯 カバレッジ閾値チェック (最低 $CoverageThreshold%)..." -ForegroundColor Yellow
        
        $totalLine = go tool cover -func "$coverFile" | Select-String "total:"
        if ($totalLine -match "(\d+\.\d+)%") {
            $coverage = [double]$Matches[1]
            if ($coverage -lt $CoverageThreshold) {
                Write-Host "❌ カバレッジが閾値未満: $coverage% < $CoverageThreshold%" -ForegroundColor Red
                exit 1
            } else {
                Write-Host "✅ カバレッジOK: $coverage% >= $CoverageThreshold%" -ForegroundColor Green
            }
        }
    }
}

function Invoke-Format {
    param([switch]$CheckOnly)
    
    if ($CheckOnly) {
        Write-Host "📝 フォーマットチェック中..." -ForegroundColor Cyan
        $unformatted = gofmt -l .
        if ($unformatted) {
            Write-Host "❌ 以下のファイルがフォーマットされていません:" -ForegroundColor Red
            $unformatted | ForEach-Object { Write-Host "  $_" }
            exit 1
        }
        Write-Host "✅ フォーマットOK" -ForegroundColor Green
    } else {
        Write-Host "📝 フォーマット中..." -ForegroundColor Cyan
        gofmt -s -w .
        Write-Host "✅ フォーマット完了" -ForegroundColor Green
    }
}

function Invoke-Vet {
    Write-Host "🔍 静的解析 (go vet)..." -ForegroundColor Cyan
    go vet $GoPackages
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✅ go vet 完了" -ForegroundColor Green
    } else {
        Write-Host "❌ go vet で問題が見つかりました" -ForegroundColor Red
        exit 1
    }
}

function Invoke-Lint {
    Write-Host "🔍 リンター実行 (golangci-lint)..." -ForegroundColor Cyan
    
    # golangci-lint がインストールされているか確認
    if (-not (Get-Command golangci-lint -ErrorAction SilentlyContinue)) {
        Write-Host "⚠️  golangci-lint がインストールされていません" -ForegroundColor Yellow
        Write-Host "インストール: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" -ForegroundColor Yellow
        exit 1
    }
    
    golangci-lint run $GoPackages
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✅ lint 完了" -ForegroundColor Green
    } else {
        Write-Host "❌ lint で問題が見つかりました" -ForegroundColor Red
        exit 1
    }
}

function Invoke-Check {
    Write-Host "=========================================" -ForegroundColor Cyan
    Write-Host "🔄 全品質チェック開始 (CI相当)" -ForegroundColor Cyan
    Write-Host "=========================================" -ForegroundColor Cyan
    Write-Host ""
    
    Invoke-Format -CheckOnly
    Write-Host ""
    Invoke-Vet
    Write-Host ""
    Invoke-Test
    Write-Host ""
    Invoke-Coverage -CheckThreshold
    
    Write-Host ""
    Write-Host "=========================================" -ForegroundColor Green
    Write-Host "✅ 全品質チェック完了" -ForegroundColor Green
    Write-Host "=========================================" -ForegroundColor Green
}

function Invoke-Dev {
    Invoke-Format
    Write-Host ""
    Invoke-Test
    Write-Host ""
    Invoke-Build
}

function Invoke-PreCommit {
    Write-Host "🔄 コミット前チェック..." -ForegroundColor Cyan
    Write-Host ""
    Invoke-Format
    Write-Host ""
    Invoke-Vet
    Write-Host ""
    Invoke-Test
    Write-Host ""
    Write-Host "✅ コミット前チェック完了" -ForegroundColor Green
}

# メイン処理
switch ($Task) {
    "help"          { Show-Help }
    "build"         { Invoke-Build }
    "clean"         { Invoke-Clean }
    "test"          { Invoke-Test }
    "test-verbose"  { Invoke-Test -Verbose }
    "coverage"      { Invoke-Coverage }
    "coverage-check"{ Invoke-Coverage -CheckThreshold }
    "fmt"           { Invoke-Format }
    "fmt-check"     { Invoke-Format -CheckOnly }
    "vet"           { Invoke-Vet }
    "lint"          { Invoke-Lint }
    "check"         { Invoke-Check }
    "dev"           { Invoke-Dev }
    "pre-commit"    { Invoke-PreCommit }
}
