# Changelog

このプロジェクトのすべての重要な変更はこのファイルに記録されます。

フォーマットは [Keep a Changelog](https://keepachangelog.com/ja/1.0.0/) に基づいています。

## [Unreleased]

### Added

- gitleaks によるシークレット混入チェックを追加（GitHub Actions / `task secrets`）
- `config.yaml` に `ui.tui` を追加し、`--tui` なしでも進捗TUIを既定で有効化できるように改善
- `sys update` / `repo update` に `--no-tui` を追加（設定より優先してTUIを無効化）
- `config show` / `config validate` を追加（設定の表示と妥当性チェック）
- `repo cleanup` を追加（マージ済みローカルブランチの整理、squashed 判定対応）
- `sys update` の対応マネージャに `flatpak` / `fwupdmgr` を追加
- `sys update` の対応マネージャに `pnpm` / `nvm` を追加

### Changed

- `README.md` に Alpha の既知の制約、`setup-repo` 併用の推奨運用、復旧手順（`config init` 再実行 / `repo.root` 見直し）を追記
- `README.md` に `setup-repo` との比較手動チェック（移行期間の確認観点）を追記
- `README.md` にアンインストール手順を追記
- `docs/Implementation_Plan.md` を現状の実装進捗に合わせて更新
- `config init` のシステムマネージャ選択肢に `flatpak` / `fwupdmgr` を追加
- `config init` のシステムマネージャ選択肢に `pnpm` / `nvm` を追加
- CLI バージョン番号を `v0.1.0-alpha` に設定し、`devsync --version` で確認可能に変更
- PowerShell 連携スクリプトの `devsync-load-env` で `env export` の複数行出力を正しく連結して `Invoke-Expression` に渡すよう修正（`System.Object[]` 型エラーを解消）
- PowerShell 連携スクリプトの `dev-sync` で `devsync-unlock` / `devsync-load-env` の成否判定を `$LASTEXITCODE` 依存から関数戻り値判定に変更し、失敗時に後続処理へ進まないよう修正
- `repo update` のジョブ表示名を Windows でも `/` 区切りで表示するよう統一
- `repo update` で未コミット変更（tracked/untracked）/stash 残存/detached HEAD を検出した場合、pull/submodule を行わず安全側にスキップして理由を表示するよう改善
- `repo update` でデフォルトブランチ以外を追跡している場合、pull/submodule を行わず安全側にスキップするよう改善
### Fixed

- Windows 環境で `go test ./...` が失敗する問題を修正（ホームディレクトリ環境変数のテスト分離、`gh` 実行モックの Windows 互換化など）
- Windows/PowerShell 環境で `config init` が PowerShell プロファイルパスを文字化けして誤ったフォルダを作成する問題を修正（Base64 経由で取得）
- Windows 環境で Git の `core.autocrlf` により Go ファイルが CRLF になり `task lint` の gofmt チェックが失敗する問題を回避（`.gitattributes` で LF 固定）
- Bitwarden CLI に未ログインの状態で `dev-sync` を実行すると、タイムアウトまで待って失敗する問題を修正（未ログインを即検知し、`bw login` を案内して終了）
- GitHub のレート制限（`429 Too Many Requests` / `secondary rate limit`）発生時に `gh` 呼び出しをリトライ/スロットリングし、`repo update` の GitHub 補完はレート制限時にスキップして処理を継続するよう改善

### Infrastructure

- GitHub Actions CI に Windows ランナーでのテスト実行を追加（`go test ./...`）

## [v0.1.0-alpha] - 2026-02-08

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
- `config init` の GitHub オーナー入力で `gh auth` のログインユーザーを自動補完するよう改善（手動入力は上書き可能）
- `config init` 実行時に既存 `config.yaml` があれば現在値を初期値として再編集できるよう改善
- `devsync run` の `sys` / `repo` セクションをプレースホルダーから実処理に置き換え、`sys update` → `repo update` を順次実行するよう改善
- `repo update` で `repo.github.owner` を参照し、`repo.root` 配下で不足しているリポジトリを clone したうえで更新を継続するよう改善（dry-run は計画表示のみ）
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

[Unreleased]: https://github.com/scottlz0310/devsync/compare/v0.1.0-alpha...HEAD
[v0.1.0-alpha]: https://github.com/scottlz0310/devsync/releases/tag/v0.1.0-alpha
