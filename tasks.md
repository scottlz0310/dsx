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

## Phase 6: TUI 進捗表示

ユーザー体験の向上。並列実行と組み合わせて効果を発揮。

- [ ] **Bubble Tea による進捗UI**
    - [ ] マルチプログレスバー表示
    - [ ] リアルタイムログ出力
    - [ ] エラー時のハイライト表示
- [ ] **既存コマンドへの統合**
    - [ ] `sys update` への適用
    - [ ] `repo update` への適用

## Backlog / 改善

- [x] README.md と --help の更新（実装済みコマンドに合わせた使用方法の記載）
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
- [x] `repo update` のレビュー指摘対応（DryRun計画整合 / `.git` 判定厳格化 / submoduleフラグ整理）
- [ ] テストカバレッジ向上（現状18.5% → 目標50%）
    - [ ] `internal/config` のテスト追加
    - [ ] `internal/updater` のテスト追加（モック使用）
    - [ ] `internal/secret` のテスト追加（Bitwarden部分はモック）
- [ ] `repo cleanup` (マージ済みブランチ削除) の移植
- [ ] `config show` / `config validate` コマンド
- [ ] 通知機能の実装
- [ ] Windows (Winget/Scoop) 対応検証
- [ ] GoReleaser によるリリース自動化
- [ ] GitHub Actions CI/CD 設定
