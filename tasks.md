# Current Tasks

現在進行中のタスクリストです。開発の進捗に合わせて随時更新してください。

## Phase 2: 設定管理の実装 (Completed)

優先度: 高。設定ファイル (`config.yaml`) を読み書きできる基盤を作ります。

- [x] **設定パッケージ (`internal/config`) の作成**
    - [x] 設定構造体 (`Config`, `RepoConfig`, `SysConfig`) の定義
    - [x] `viper` を使用したデフォルト値設定とファイル読み込み処理
    - [x] 環境変数 (`DEVSYNC_*`) とのバインディング
- [x] **設定ファイル生成 (`config init`)**
    - [x] `survey` を導入し、対話形式でユーザー入力を受け付ける
    - [x] 入力内容を `yaml` ファイルとして保存する
- [x] **環境認識**
    - [x] コンテナ内 (`IsContainer`) かホストか判定するロジックの実装
    - [x] 推奨パッケージマネージャのリコメンドロジック実装

## Phase 3: システム更新機能 (Sys) - 基礎 (Completed)

設定ができたら、実際にコマンドを動かす部分を作ります。

- [x] **Secrets管理 (Bitwarden連携)**
    - [x] `config` に Secrets 設定を追加
    - [x] `bw` コマンドラッパー実装 (Unlock / Get Item)
    - [x] コマンド実行前の環境変数注入ロジック (Middleware/PreRun)

- [x] **Updater インターフェース定義**
    - [x] `Check()`, `Update()`, `Name()` などの共通メソッド定義
    - [x] レジストリパターンによる拡張可能な設計
    - [x] `GetEnabled()` で設定に基づくマネージャ取得
- [x] **主要マネージャの実装**
    - [x] `apt` (Debian/Ubuntu)
    - [x] `brew` (macOS/Linux)
    - [x] `go` (Go binaries)
    - [x] `npm` (Node.js グローバルパッケージ)
    - [x] `pipx` (Python CLI ツール)
    - [x] `cargo` (Rust ツール)
    - [x] `snap` (Snap パッケージ)
- [x] **`sys update` コマンド実装**
    - [x] 設定に基づいて有効なマネージャをリストアップ
    - [x] 順次実行 (まずは並列なしで確実に動くもの)
    - [x] `sys list` コマンドで利用可能マネージャを一覧表示

## Phase 4: 並列実行エンジン (Completed)

優先度: 高。`sys update` の高速化と、今後の `repo update` への基盤。

- [x] **並列実行基盤 (`internal/runner`)**
    - [x] `errgroup` + semaphore による並列制御
    - [x] Context cancel / Timeout 管理
    - [x] 結果集計（成功/失敗/スキップ）
- [x] **`sys update` への組み込み**
    - [x] `--jobs N` フラグで並列数を指定
    - [x] マネージャごとの依存関係考慮（apt は単独実行など）

## Phase 5: リポジトリ管理 (Completed)

旧ツール `Setup-Repository` の機能移植。

- [x] **リポジトリスキャン**
    - [x] 設定の `repo.root` からリポジトリを検出
    - [x] `.git` ディレクトリ（および worktree の `.git` ファイル）存在確認
- [x] **`repo update` コマンド**
    - [x] `git fetch` + `git pull --rebase` の実行
    - [x] サブモジュール更新対応
    - [x] 並列実行エンジンとの統合
- [x] **`repo list` コマンド**
    - [x] 管理下リポジトリの一覧表示
    - [x] ステータス表示（クリーン/ダーティ/未プッシュ/追跡なし）

## Phase 6: TUI 進捗表示 (Completed)

ユーザー体験の向上。並列実行と組み合わせて効果を発揮。

- [x] **Bubble Tea による進捗UI**
    - [x] マルチプログレスバー表示
    - [x] リアルタイムログ出力
    - [x] エラー時のハイライト表示
- [x] **既存コマンドへの統合**
    - [x] `sys update` への適用
    - [x] `repo update` への適用

## Backlog / 改善

- [x] README.md と --help の更新（実装済みコマンドに合わせた使用方法の記載）
- [x] README.md にインストール手順と日常運用（マニュアルテスト）手順を追記
- [x] README.md にアンインストール手順を追記
- [x] GitHub レート制限（429/secondary rate limit）時の `gh` リトライ/スロットリングと、`repo update` の GitHub 補完スキップによる復帰導線を追加
- [x] CHANGELOG.md の作成
- [x] 開発タスクランナー整備（Makefile → Taskfile.yml に移行）
- [x] golangci-lint 設定強化（複雑度チェック追加）
- [x] golangci-lint 警告対応（100+ → 0）
    - [x] errcheck 警告の修正
    - [x] gocognit: LoadEnv リファクタ（複雑度26→<20）
    - [x] errorlint: errors.As/Is への移行
    - [x] gocritic: octalLiteral, assignOp, paramTypeCombine など
    - [x] 不要な警告のconfig除外（gosec, fieldalignment, shadow, prealloc）
    - [x] `wsl` から `wsl_v5` へ移行（非推奨警告対応）
- [x] 日常運用コマンドを `task check` に統一（`task daily` 追加）
- [x] gitleaks によるシークレット混入チェックを追加（GitHub Actions / `task secrets`）
- [x] Windows 環境で `go test ./...` が通るようにテストをクロスプラットフォーム化（HOME/USERPROFILE 分離など）
- [x] Windows 環境で Git の `core.autocrlf` により Go ファイルが CRLF になり `task lint` の gofmt チェックが失敗する問題を回避（`.gitattributes` で LF 固定）
- [x] GitHub Actions CI に Windows ランナーでのテスト実行を追加（`go test ./...`）
- [x] `repo update` のレビュー指摘対応（DryRun計画整合 / `.git` 判定厳格化 / submoduleフラグ整理）
- [x] 初回運用導線の改善（`config init` 誘導 / `doctor` 設定表示の明確化 / エラー重複解消 / `sys list` 有効列の明示 / `--tui` 対象0件メッセージ）
- [x] `docs/Implementation_Plan.md` の進捗チェックを現状に合わせて更新
- [x] TUI の既定有効化設定を追加（`ui.tui` / `--no-tui`）
- [x] `sys.enable` に未インストールのマネージャが含まれる場合の警告スキップ動作を整備（処理継続）
- [x] `config init` で `repo.root` 未存在時の作成確認導線を追加（拒否時は終了）
- [x] `sys update` 実行前の sudo 事前認証（単独フェーズ前/並列フェーズ前）を追加し、`use_sudo`/`sudo` の互換設定を整備
- [x] `snapd unavailable` 環境で `snap` を利用不可として自動スキップする判定を追加
- [x] シェル連携スクリプトを改善（`devsync-load-env` の終了コード伝播 / `dev-sync` の `Bitwarden 解錠 → 環境変数読込 → devsync run` 導線化 / 実行パスのフォールバック）
- [x] PowerShell 連携スクリプトの不具合修正（`devsync-load-env` の配列連結不足による `Invoke-Expression` 型エラー解消 / `dev-sync` の関数失敗判定を `$LASTEXITCODE` 依存から修正）
- [x] `config init` の GitHub オーナー入力を `gh auth` ログイン情報で自動補完（必要時のみ手動上書き）
- [x] `config init` の再実行時に既存 `config.yaml` の値を初期値として再編集できるよう改善
- [x] `devsync run` の `sys` / `repo` フローをプレースホルダーから実処理へ置換（`sys update` → `repo update`）
- [x] `repo update` で `repo.github.owner` 一覧との差分を補完し、不足リポジトリを clone する導線を追加（dry-run計画表示対応）
- [x] `repo sync` 安全運用の段階的強化（`setup-repo` 併用期間）
    - [x] 基本方針の明文化（破壊的操作は行わず、危険状態はスキップして理由を表示）
    - [x] 判定ロジックを table-driven tests で追加（境界値・失敗系を優先）
    - [x] 未コミット変更あり（tracked/untracked）時の更新スキップと理由表示
    - [x] stash 残存時の更新スキップと理由表示
    - [x] detached HEAD 時の更新スキップと理由表示
    - [x] upstream 未設定時の更新スキップ理由の統一（既存挙動の回帰テスト化）
    - [x] デフォルト以外の追跡ブランチ運用時の更新可否ルールを明文化し実装
    - [x] `setup-repo` との比較マニュアルテスト項目を固定化（差分が出るケースを回帰テスト候補へ追加 / README.md に追記）
- [x] テストカバレッジ向上（現状50.9% → 目標50%）
    - [x] `internal/config` のテスト追加
    - [x] `internal/updater` のテスト追加（モック使用）
    - [x] `internal/secret` のテスト追加（Bitwarden部分はモック）
- [x] `repo cleanup` (マージ済みブランチ削除) の移植
- [x] `config show` / `config validate` コマンド
- [ ] `sys` パッケージマネージャ対応を `sysup` 同等以上へ拡張
    - [ ] 既存実装（`apt` / `brew` / `go` / `npm` / `snap` / `pipx` / `cargo`）との差分棚卸し
    - [ ] Linux向け `flatpak` / `fwupdmgr` Updater の実装
    - [ ] Node系 `pnpm` / `nvm` Updater の実装
    - [ ] 追加ツール `uv tool` / `rustup` / `gem` Updater の実装
    - [ ] Windows向け `winget` / `scoop` Updater の実装と実機検証
    - [ ] `sys list` / `README.md` / `config init` への反映
    - [ ] 各Updaterの dry-run / timeout / エラー分類の統合テスト
- [ ] 通知機能の実装
- [x] Windows/PowerShell で `config init` がプロファイルパスを文字化けして誤ったフォルダを作成する問題を修正
- [ ] Windows (Winget/Scoop) 対応検証
- [ ] GoReleaser によるリリース自動化
- [ ] GitHub Actions CI/CD 設定
- [ ] `sys update` / `repo update` の E2E テスト整備
    - [ ] `--tui` 指定時の終了コードとサマリー整合を確認
    - [ ] 非TTY環境での `--tui` 自動フォールバック挙動を確認
- [ ] TUI UX 改善
    - [ ] 失敗ジョブの詳細エラー表示（展開表示）を追加
    - [ ] 長時間実行向けにログ保存オプション（ファイル出力）を追加
- [ ] runner イベント基盤の活用拡張
    - [ ] `devsync run`（将来 `tool update`）への進捗UI適用
    - [ ] 通知機能向けイベントフックの追加
