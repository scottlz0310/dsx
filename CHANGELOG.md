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
  - `--tui` フラグで Bubble Tea による進捗表示（マルチ進捗バー・リアルタイムログ・失敗ハイライト）に対応
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
  - `--tui` フラグで Bubble Tea による進捗表示（マルチ進捗バー・リアルタイムログ・失敗ハイライト）に対応
  - `--submodule` / `--no-submodule` フラグで submodule 更新設定を明示上書き可能
  - DryRun 時も upstream 有無を確認し、`pull` 計画表示を実挙動と一致させるよう改善
- `devsync repo list` - 管理下リポジトリの一覧表示
  - `config.yaml` の `repo.root` 配下をスキャン
  - `--root` フラグでスキャンルートを上書き可能
  - ステータス表示（クリーン / ダーティ / 未プッシュ / 追跡なし）
- `internal/repo` パッケージを追加（検出・状態取得ロジック）
  - `.git` 判定を厳格化（ディレクトリまたは `gitdir:` 形式ファイルのみをリポジトリとして検出）
- `internal/tui` パッケージを追加（Bubble Tea ベースの進捗UI）
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

### Changed

- 初回運用時の導線を改善
  - `repo list` / `repo update` で `repo.root` が未存在かつ設定未初期化の場合、`devsync config init` を明示的に案内
  - `doctor` で「設定ファイルあり / なし（デフォルト値運用）」を区別して表示
- コマンドエラーの二重表示を解消（`rootCmd` のエラー出力ポリシーを整理）
- `sys list` の「有効」列を `✅/❌` 表示に変更
- `sys update --tui` / `repo update --tui` で対象0件時に「TUI未起動」の理由を明示
- `sys.enable` に未インストールのマネージャが含まれる場合、警告を表示してスキップし、処理を継続するよう改善
- `sys update` で sudo が必要なマネージャ（`apt` / `snap`）を実行する前に、単独フェーズ・並列フェーズごとに `sudo -v` で事前認証するよう改善
- `apt` / `snap` の manager 設定で `use_sudo` と旧キー `sudo` の両方を受け付けるよう改善
- `snap` の可用性判定を強化し、`snapd unavailable` の環境では利用不可として自動スキップするよう改善
- `config init` が生成するシェル連携スクリプトを改善
  - `devsync-load-env` が `devsync env export` の失敗時に正しく終了コードを返すよう修正
  - `dev-sync` 互換関数を `Bitwarden 解錠 → 環境変数を親シェルへ読み込み → devsync run` の順で実行するよう改善
  - 設定された実行パスが無効な場合に `command -v devsync`（PowerShell は `Get-Command`）へフォールバック
- `config init` で指定した `repo.root` が未存在の場合に作成確認を追加（拒否時は保存せず終了）
- README の初回セットアップ手順に `devsync config init` 必須を明記

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
- README.md に利用者向けインストール手順と日常運用（マニュアルテスト）手順を追記
- Implementation_Plan.md - 設計ドキュメント
- Legacy_Migration_Analysis.md - 旧ツールからの移行分析
- AGENTS.md - AIエージェント運用ガイドライン
- CONTRIBUTING.md - コントリビューションガイド
- SECURITY.md - セキュリティポリシー

---

[Unreleased]: https://github.com/scottlz0310/devsync/compare/main...HEAD
