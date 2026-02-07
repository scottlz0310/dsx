# Changelog

このプロジェクトのすべての重要な変更はこのファイルに記録されます。

フォーマットは [Keep a Changelog](https://keepachangelog.com/ja/1.0.0/) に基づいています。

## [Unreleased] - v1.0.0

### Added

#### コアコマンド
- `devsync run` - 日次の統合タスク実行（Bitwarden解錠→環境変数読込→更新処理）
- `devsync doctor` - 依存ツール（git, gh, bw）と環境設定の診断機能

#### システム更新機能 (`sys`)
- `devsync sys update` - パッケージマネージャによる一括更新
  - `--dry-run` / `-n` フラグでドライラン対応
  - `--verbose` / `-v` フラグで詳細ログ出力
  - `--jobs` / `-j` フラグで並列実行数の指定に対応
  - `--timeout` / `-t` フラグでタイムアウト設定
  - `apt` を単独実行に分離し、他マネージャは並列実行可能に改善
- `devsync sys list` - 利用可能なパッケージマネージャの一覧表示
- 対応パッケージマネージャ:
  - `apt` (Debian/Ubuntu)
  - `brew` (macOS/Linux Homebrew)
  - `go` (Go ツール)
  - `npm` (Node.js グローバルパッケージ)
  - `pipx` (Python CLI ツール)
  - `cargo` (Rust ツール)
  - `snap` (Snap パッケージ)
- 拡張可能な Updater インターフェースとレジストリパターンの採用

#### リポジトリ管理機能 (`repo`)
- `devsync repo update` - 管理下リポジトリを一括更新
  - `git fetch --all` / `git pull --rebase` を実行
  - `--jobs` / `-j` フラグで並列更新に対応
  - `--dry-run` / `-n` フラグで更新計画の確認に対応
  - `--submodule` / `--no-submodule` フラグで submodule 更新設定を明示上書き可能
  - DryRun 時も upstream 有無を確認し、`pull` 計画表示を実挙動と一致させるよう改善
- `devsync repo list` - 管理下リポジトリの一覧表示
  - `config.yaml` の `repo.root` 配下をスキャン
  - `--root` フラグでスキャンルートを上書き可能
  - ステータス表示（クリーン / ダーティ / 未プッシュ / 追跡なし）
- `internal/repo` パッケージを追加（検出・状態取得ロジック）
  - `.git` 判定を厳格化（ディレクトリまたは `gitdir:` 形式ファイルのみをリポジトリとして検出）
- `repo.sync.submodule_update` 設定を追加（デフォルト: true）
- `internal/runner` を追加（`errgroup + semaphore` による並列実行・結果集計）

#### 環境変数機能 (`env`)
- `devsync env export` - Bitwardenから環境変数をシェル形式でエクスポート
  - bash/zsh: `eval "$(devsync env export)"`
  - PowerShell: `& devsync env export | Invoke-Expression`
  - 安全なクオート/エスケープ処理
- `devsync env run` - 環境変数を注入してサブプロセスでコマンド実行
  - `eval` を使わない安全な実行方式
  - 終了コードの正確な伝播

#### 設定管理機能 (`config`)
- `devsync config init` - 対話形式（survey）による設定ファイル生成ウィザード
  - リポジトリルートディレクトリ
  - GitHubオーナー名
  - 並列実行数
  - 有効化するパッケージマネージャの選択
  - シェル初期化スクリプトの自動設定
- `devsync config uninstall` - シェル設定からdevsyncを削除
- YAML形式の設定ファイル (`~/.config/devsync/config.yaml`)
- 環境変数 (`DEVSYNC_*`) によるオーバーライド対応

#### Bitwarden連携 (`internal/secret`)
- Bitwarden CLI (`bw`) ラッパーの実装
- セッショントークン管理（Unlock/Lock）
- 環境変数アイテムの取得と解析
- シェル形式でのエクスポートフォーマッター

#### 環境認識 (`internal/env`)
- コンテナ内実行の自動検出 (`IsContainer`)
- OS/環境に応じた推奨パッケージマネージャのリコメンド

### Infrastructure

- Go 1.25 による開発環境
- Cobra CLI フレームワークの採用
- Viper による設定管理
- Taskfile.yml によるタスクランナー（Windows互換）
- `task daily` を追加し、日常運用の標準コマンドを `task check` に統一
- golangci-lint による静的解析
  - 循環的複雑度 (gocyclo)
  - 認知複雑度 (gocognit)
  - 重複コード検出 (dupl)
  - エラーハンドリング (errorlint)
  - その他品質チェック
- `wsl` から `wsl_v5` へ移行（非推奨警告を解消）
- GitHub Actions CI/CD（予定）
- DevContainer 対応

### Documentation

- README.md - プロジェクト概要と使用方法
- Implementation_Plan.md - 設計ドキュメント
- Legacy_Migration_Analysis.md - 旧ツールからの移行分析
- AGENTS.md - AIエージェント運用ガイドライン
- CONTRIBUTING.md - コントリビューションガイド
- SECURITY.md - セキュリティポリシー

---

[Unreleased]: https://github.com/scottlz0310/devsync/compare/main...HEAD
