以下に、「一押し構成（Go中心 + 安定並列 + インタラクティブ設定）」での **実装計画書（Roadmap）** を提示します。
“毎日使う道具”として運用を崩さずに育てられるよう、**価値の出る順に段階導入**する計画にしています。

---

# 実装計画書（Roadmap）

対象：既存ツール（sysup / Setup-Repository）の統合 + Bitwarden 環境変数注入の自動化を含むクロスプラットフォーム CLI

## 1. 目的とゴール

### 目的

* **開発環境運用の統合と一本化**:
  * 既存のシステム更新ツール [sysup](https://github.com/scottlz0310/sysup)
  * リポジトリ管理ツール [Setup-Repository](https://github.com/scottlz0310/Setup-Repository)
  * `bw-cli` (Bitwarden CLI) を用いた環境変数注入の自動化
  
  上記3つの機能を統合し、単一のCLIツールとして運用・保守・拡張の中心に据える。

* **学習体験と技術スタックの刷新**:
  * これまで主力であったPythonによるCLI開発から離れ、**Go**（またはRust）などの静的型付けコンパイル言語を採用する。
  * これにより、配布性・並列処理の安定性を向上させるとともに、新しい言語パラダイムごとの設計・実装パターンを習得する「学習の場」としても活用する。

* **運用体験の向上**:
  * ネットワークアクセスを伴う処理の **安定した並列実行**（高速化と失敗率の低減）。
  * 設定体験を「GUI感覚（ウィザード形式）」に近づけ、継続利用の摩擦を下げる。

* **ハイブリッド運用の実現 (New)**:
  * ローカルPC（Host）では「母艦のメンテナンス」、Codespaces（Container）では「個人環境の即時セットアップ（Personalization）」と、実行環境に応じて振る舞いを最適化する。

### v0.1 ゴール（MVP）

* 単一バイナリで動作するGo CLIが提供され、最低限の日次運用が置き換え可能
* 既存ツール（sysup/Setup-Repository）の機能が移植され、統合コマンドから呼び出せる
* 安定並列（上限・timeout・cancel・集計）が確立し、更新処理が破綻しない
* `config init`（ウィザード）で設定ファイルを作成できる
* `repo update` / `sys update` がそれぞれ動く
* `doctor` で依存コマンド（bwコマンド含む）や基本疎通を確認できる
* `Host` / `Container` モードを判別し、`sys update` 等の挙動を切り替える (New)

---

## 2. 推奨技術スタック（v0.1）

* 言語：Go
* CLIフレームワーク：Cobra
* 設定：YAML（人間が読み書きしやすい）＋構造体バインド

  * 読み書き：Viper（採用決定）
* インタラクティブ設定（初期）：survey（initウィザード）
* インタラクティブ設定（次段）：Bubble Tea（TUI編集画面）
* 並列制御：errgroup + semaphore（もしくはワーカープール）
* HTTP：net/http（Transport明示）＋ Context
* ログ：標準log/slog
* テスト：標準testing + testify
* リリース：GoReleaser（Windows/Linux向けバイナリを生成）

---

## 3. CLIのコマンド設計（v0.1）

### 基本コマンド

* `devsync tool update`

  * 内部で repo/sys の双方（または設定に応じて片方）を実行
  * `--jobs N`（並列数）
  * `--timeout 5m`（全体タイムアウト）
  * `--dry-run`（計画のみ）
  * `--yes`（確認なし）
  * `--fail-fast`（任意、デフォルトは集計）
  * `--mode [host|container]` (強制指定オプション) (New)

### Repo系

* `devsync repo update`（リポジトリの pull / submodule update 等）
* `devsync repo list`（対象一覧表示）

### System系

* `devsync sys update`
  * **Host Mode**: OSパッケージマネージャ(`winget`, `brew`, `apt`)によるシステム・アプリ更新
  * **Container Mode**: ユーザー固有ツール(`fzf`, `starship`等)やdotfilesのインストール・設定同期 (New)

### 設定系

* `devsync config init`（survey：カーソル選択→確定のウィザード）
* `devsync config show`（現在の設定表示）
* `devsync config validate`（スキーマ + 実行環境チェック）

### 診断系

* `devsync doctor`（validate + 依存コマンド検出 + 疎通テスト）

---

## 4. 設定スキーマ（YAML案）

```yaml
version: 1

concurrency:
  jobs: 6
  timeout: "5m"
  per_request_timeout: "20s"

repo:
  enabled: true
  roots:
    - path: "~/src"
      include:
        - "repoA"
        - "repoB"
      exclude:
        - "legacy-repo"
  actions:
    git_pull: true
    submodule_update: true

sys:
  enabled: true
  managers:
    - kind: "brew"      # macが増えた場合に拡張
    - kind: "apt"
    - kind: "winget"
  options:
    upgrade: true
    cleanup: false
  
  # コンテナ内でセットアップしたい個人ツール (New)
  personalization:
    packages:
      - "fzf"
      - "ripgrep"
    dotfiles: "https://github.com/user/dotfiles.git"

secrets:
  enabled: false
  provider: "bitwarden"
  # ... 省略
```

---

## 7. 実装ステップ（マイルストーン）

### Phase 1：プロジェクト骨格とCLI基盤 (完了)

* [x] プロジェクト構成作成 (cmd, internal, etc)
* [x] Go モジュール初期化
* [x] Cobra 導入・ルートコマンド作成
* [x] `doctor` コマンドスケルトン作成
* [x] ドキュメント整理 (README, AGENTS, Docs)

### Phase 2：設定管理の実装 (Next)

* [ ] `viper` 導入と設定ファイルの定義
* [ ] 環境認識ロジックの実装 (`Host` vs `Container`) (New)
* [ ] `config init` コマンド実装 (survey 使用)
* [ ] 設定ファイルの読み込み・検証ロジック

### Phase 3：実行エンジン（並列・集計）の確立

* [ ] Job インターフェース定義
* [ ] worker pool / semaphore 実装
* [ ] Context cancel / Timeout 管理

### Phase 4：主要機能の実装 (Repo / Sys)

* [ ] `sys update` 実装 (コマンド実行ラッパー)
* [ ] `repo update` 実装 (`git` コマンドラッパー)
* [ ] `tool update` (統合コマンド) 実装

### Phase 5：配布準備

* [ ] GoReleaser 設定
* [ ] バイナリビルド確認
* [ ] リリース

---
