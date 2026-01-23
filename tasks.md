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

## Phase 3: システム更新機能 (Sys) - 基礎

設定ができたら、実際にコマンドを動かす部分を作ります。

- [x] **Secrets管理 (Bitwarden連携)**
    - [x] `config` に Secrets 設定を追加
    - [x] `bw` コマンドラッパー実装 (Unlock / Get Item)
    - [x] コマンド実行前の環境変数注入ロジック (Middleware/PreRun)

- [ ] **Updater インターフェース定義**
    - [ ] `Check()`, `Update()`, `Name()` などの共通メソッド定義
- [ ] **主要マネージャの実装 (PoC)**
    - [ ] `apt` (Debian/Ubuntu)
    - [ ] `brew` (macOS/Linux)
    - [ ] `go` (Go binaries)
- [ ] **`sys update` コマンド実装**
    - [ ] 設定に基づいて有効なマネージャをリストアップ
    - [ ] 順次実行 (まずは並列なしで確実に動くもの)

## Phase 4: 並列実行とリポジトリ機能

- [ ] **並列実行エンジン**
    - [ ] `errgroup` または WorkerPool による並列制御
    - [ ] プログレスバー表示 (Bubble Tea または mpb)
- [ ] **リポジトリ管理 (Repo)**
    - [ ] `gh` CLI または GitHub API との連携
    - [ ] `repo update` (Clone/Pull)

## Backlog / 改善

- [ ] TUI (Bubble Tea) によるリッチな進捗表示
- [ ] `repo cleanup` (マージ済みブランチ削除) の移植
- [ ] 通知機能の実装
- [ ] Windows (Winget/Scoop) 対応検証
