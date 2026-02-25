# レガシーツール移行分析・要件定義書

本ドキュメントでは、既存ツール `sysup` および `Setup-Repository` の機能を分析し、`dsx` への統合・移行方針を定義します。

## 1. 移行対象ツールの分析

### 1.1 sysup (System Update Tool)
システムおよび各種パッケージマネージャを統合的に更新するPython製ツール。

| 機能カテゴリ | 主要機能 | dsx での対応方針 |
| :--- | :--- | :--- |
| **対応パッケージマネージャ** | **Linux**: APT, Snap, Flatpak, Firmware | `sys` コマンドで実装 (Hostモード) |
| | **macOS/Linux**: Homebrew | 同上 |
| | **Windows**: Scoop | 同上 |
| | **Cross**: npm, pnpm, nvm, pipx, uv tool, Rustup, Cargo, Gem | 同上。各コマンドの有無で自動検出 |
| **実行制御** | 並列実行 (Parallel updates) | Go の Goroutine/WorkerPool で実装 (優先度高) |
| | ドライラン (Dry-run) | 全コマンドに `--dry-run` オプション実装 |
| | タイムアウト制御 | Context による全体/個別プロセスのキャンセル |
| **付加機能** | 通知 (Desktop Notification) | v1.0 以降で検討 (優先度低) |
| | バックアップ (Package list dump) | ログ機能の一部として実装検討 |
| | 自動実行 (WSL Integration) | v1.0 で検討 (シェル設定の生成機能など) |

### 1.2 Setup-Repository
GitHubリポジトリの同期・管理ツール。

| 機能カテゴリ | 主要機能 | dsx での対応方針 |
| :--- | :--- | :--- |
| **同期機能** | GitHub全リポジトリ取得 (My repos) | `repo update` で実装。GitHub API連携必須 |
| | `git clone` / `git pull` | 並列実行により高速化 |
| **整理機能** | マージ済みローカルブランチ削除 (Cleanup) | `repo cleanup` コマンドまたはオプションとして実装 |
| | Squash Merge されたブランチの検出 | GitHub API を用いた高度な判定ロジックの移植 |
| **設定** | `gh` CLI 認証連携 | `gh auth token` などを利用し、トークン管理を委譲 |
| | Bitwarden 連携 (New) | **優先:** `bw` CLI 経由で GPAT 等を取得し、環境変数に注入してから同期処理を実行する |

---

## 2. 統合設計 (dsx)

両ツールの機能を単一のバイナリ `dsx` に統合するための機能マッピングです。

### 2.1 コマンド体系

```bash
dsx
├── tool (ALIAS: update)
│   ├── update      # sys と repo の一括実行 (sysup機能 + repo sync機能)
│   └── cleanup     # 掃除系タスクの一括実行 (sysupのcache clear + repo cleanup)
├── sys
│   ├── update      # システムパッケージ更新 (旧 sysup update)
│   └── doctor      # 依存コマンド診断
├── repo
│   ├── update      # リポジトリ同期 (旧 setup-repo sync)
│   ├── cleanup     # ブランチ掃除 (旧 setup-repo cleanup)
│   └── list        # 対象リポジトリ一覧
└── config
    ├── init        # ウィザード形式での設定ファイル生成
    └── edit        # 設定ファイルの直接編集 (Editor起動)
```

### 2.2 設定ファイル (config.yaml) 統合案

旧ツールの設定項目を網羅しつつ、Go (Viper) で扱いやすい構造にします。

```yaml
version: 1

# [共通] 並列・実行制御
control:
  concurrency: 8          # 並列数
  timeout: "10m"          # 全体タイムアウト
  dry_run: false

# [Repo] Setup-Repository 由来
repo:
  root: "~/src"           # ベースディレクトリ
  # GitHub連携 (gh CLI利用を基本とするためトークン設定は省略可)
  github:
    owner: ""             # 自動検出 or 指定
    protocol: "https"     # https | ssh
  
  sync:
    auto_stash: true      # pull前の退避
    prune: true
  
  cleanup:
    enabled: true
    target: ["merged", "squashed"] # 削除対象
    exclude_branches: ["main", "develop", "master"]

# [Sys] sysup 由来
sys:
  # 有効化するマネージャ (指定しなければ auto-detect)
  enable: 
    - apt
    - brew
    - go
    - npm
  
  # マネージャごとの個別設定
  managers:
    apt:
      sudo: true
    npm:
      global: true
```

## 3. 移行・実装ロードマップへの反映

### Phase 1: 基盤整備 (完了済)
* Cobra + Viper 構成
* ロギング基盤

### Phase 2.1: `sys` 機能の移植 (優先)
* `sysup` の主要なUpdater (apt, brew, go, npm 等) を `dsx sys update` に実装。
* インターフェース更新: `Updater` インターフェースを定義し、各マネージャを実装。
* 並列実行機能の実装。

### Phase 2.2: `repo` 機能の移植
* GitHub API クライアントの実装 (または `gh` コマンドラッパー)。
* `git` コマンド実行ラッパーの実装。
* `repo update` (clone/pull) の実装。

### Phase 3: 高度な機能の移植
* `repo cleanup` (ブランチ削除) のロジック移植。
* インタラクティブな `config init` (survey利用)。

### Phase 4: 統合と洗練
* `tool update` (全実行) の実装。
* TUI (Bubble Tea) による進捗表示の改善。
