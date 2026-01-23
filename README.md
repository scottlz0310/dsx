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

## 📅 ステータス

現在 **v0.1 計画 / 初期開発** フェーズです。
詳細なロードマップについては [docs/Implementation_Plan.md](docs/Implementation_Plan.md) を参照してください。

## 📄 ライセンス

[LICENSE](LICENSE) を参照してください。
