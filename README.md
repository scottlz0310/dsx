# DevSync

DevSync は、開発環境の運用作業を統合・一元化するためのクロスプラットフォーム CLI ツールです。
既存の `sysup` および `Setup-Repository` を置き換え、Bitwarden を利用した環境変数注入の自動化を目指します。

## 🚀 目的

- **運用の統合**: リポジトリ管理、システム更新、セキュアな環境変数注入を単一の CLI に集約します。
- **技術スタックの刷新**: Go 言語を採用し、安定性、配布の容易さ、信頼性の高い並列実行を実現します。
- **体験の向上**: 日本語によるインタラクティブな設定ウィザードや、安定した並列制御を提供します。

## 🛠 技術スタック

- **言語**: Go
- **CLI フレームワーク**: Cobra
- **設定**: YAML + Viper (または独自実装)
- **インタラクティブ UI**: Survey (ウィザード) / Bubble Tea (TUI)
- **並列制御**: errgroup + semaphore

## 📋 機能 (v0.1 計画)

### コアコマンド
- `devsync run`: 設定ファイルによるカスタム化されたコマンドを実行します。（毎日の実行用）
- `devsync tool update`: システムおよびリポジトリの更新を一括実行します。
- `devsync doctor`: 依存関係や潜在的な問題を診断します。

### リポジトリ管理
- `devsync repo update`: 管理下のリポジトリを更新します。
- `devsync repo list`: 管理下のリポジトリ一覧を表示します。

### システム管理
- `devsync sys update`: システム更新タスクを実行します。

### 設定
- `devsync config init`: 対話形式のウィザードで設定ファイルを生成します。
- `devsync config show`: 現在の設定を表示します。
### 環境変数
- `devsync env export`: Bitwardenから環境変数を取得し、シェルにエクスポートします。
- `devsync env run`: Bitwardenから環境変数を注入してコマンドを実行します。

## 🔑 環境変数の使用

### 方法1: シェルに環境変数を読み込む（eval）

Bitwardenから環境変数を現在のシェルに読み込むには：

```bash
# Bitwardenから環境変数をエクスポート
eval "$(devsync env export)"

# 確認
echo $GPAT
```

**PowerShell:**
```powershell
& devsync env export | Invoke-Expression
```

### 方法2: サブプロセスに環境変数を注入する（推奨）

`eval` を使わずに安全にコマンドを実行できます：

```bash
# 環境変数を注入してコマンドを実行
devsync env run npm run build
devsync env run go test ./...
```

この方法は以下の利点があります：
- `eval` のリスクを回避
- コマンドの終了コードを保持
- 親シェルに影響を与えない

**注意**: `devsync run` コマンド内では環境変数は自動的に注入されますが、親シェルには反映されません。シェルで環境変数を使用したい場合は上記のコマンドを使用してください。
## � 開発

### ビルド

**Linux / macOS:**
```bash
go build -o devsync ./cmd/devsync
```

**Windows (PowerShell):**
```powershell
$env:GOOS="windows"; $env:GOARCH="amd64"; go build -o devsync.exe ./cmd/devsync
```

**Windows (クロスコンパイル - Linux/macOS から):**
```bash
GOOS=windows GOARCH=amd64 go build -o devsync.exe ./cmd/devsync
```

### 実行

```bash
./devsync --help
```

## �📅 ステータス

現在 **v0.1 計画 / 初期開発** フェーズです。
詳細なロードマップについては [docs/Implementation_Plan.md](docs/Implementation_Plan.md) を参照してください。

## 📄 ライセンス

[LICENSE](LICENSE) を参照してください。
