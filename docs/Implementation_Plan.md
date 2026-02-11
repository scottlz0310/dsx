# 実装計画書（Roadmap）

対象：既存ツール（sysup / Setup-Repository）の統合 + Bitwarden 環境変数注入を含むクロスプラットフォーム CLI

## 1. 目的とゴール

### 目的

- **開発環境運用の統合と一本化**: `sysup` / `Setup-Repository` 相当の機能を単一の CLI に統合する
- **学習体験と技術スタックの刷新**: Python から Go へ移行し、配布性と並列実行の安定性を高める
- **運用体験の向上**: 「毎日使う」ことを前提に、設定ウィザード・並列実行・進捗表示を整備する

### v0.1 ゴール（MVP）

- [x] 単一バイナリで動作する Go CLI が提供され、日次運用の中核フローが置き換え可能
- [x] `config init`（ウィザード）で設定ファイルを作成できる
- [x] `sys update` / `repo update` が動作する
- [x] 安定並列（上限・timeout・cancel・集計）が確立し、更新処理が破綻しない
- [x] `doctor` で依存コマンドや基本疎通を確認できる
- [x] TUI（Bubble Tea）で並列実行の進捗を表示できる

## 2. 現状（2026-02-11 時点）

- `devsync run`（Bitwarden解錠 → env注入 → `sys update` → `repo update`）まで到達
- `repo update` は並列実行 + サブモジュール更新に対応
- `sys update` は主要マネージャを実装済み（apt/brew/go/npm/pnpm/nvm/pipx/cargo/snap/flatpak/fwupdmgr）
- 進捗UIは `--tui` で起動（設定 `ui.tui` で既定ON/OFFも可能）

## 3. CLI コマンド設計（v0.1 時点）

- 日次統合: `devsync run`
- 診断: `devsync doctor`
- システム更新: `devsync sys update`, `devsync sys list`
- リポジトリ管理: `devsync repo update`, `devsync repo list`
- 環境変数: `devsync env export`, `devsync env run`
- 設定: `devsync config init`, `devsync config uninstall`

予定機能（未実装）:

- 通知機能
- `sys` マネージャ拡張（flatpak/fwupdmgr/pnpm/nvm/uv/rustup/gem/winget/scoop 等）
- リリース/CI（GoReleaser/GitHub Actions/E2E）
- `sys update` / `repo update` の E2E テスト整備
- TUI UX 改善

## 4. 設定スキーマ（config.yaml）

```yaml
version: 1

control:
  concurrency: 8
  timeout: "10m"
  dry_run: false

ui:
  tui: false  # true の場合、--tui なしでも進捗UIを既定で有効化

repo:
  root: "~/src"
  github:
    owner: ""        # 設定時は GitHub の一覧との差分を補完（不足分は clone 導線）
    protocol: "https" # https | ssh
  sync:
    auto_stash: true
    prune: true
    submodule_update: true
  cleanup:
    enabled: true
    target: ["merged", "squashed"]
    exclude_branches: ["main", "master", "develop"]

sys:
  enable: [] # 例: ["apt", "go"]
  managers:
    apt:
      use_sudo: true

secrets:
  enabled: true
  provider: "bitwarden"
```

補足:

- `ui.tui=true` の場合でも、非TTY（リダイレクト/CI等）では自動的に通常表示へフォールバックします。
- コマンド単位で上書きしたい場合は `--tui` / `--no-tui` を使用します。

## 5. 実装ステップ（マイルストーン）

### Phase 1：プロジェクト骨格とCLI基盤（完了）

- [x] Go モジュール初期化 / Cobra 導入
- [x] ルートコマンド / `doctor` スケルトン
- [x] ドキュメント整備（README/AGENTS/Docs）

### Phase 2：設定管理（完了）

- [x] `internal/config`（Viper）で設定の読み込み/保存
- [x] `config init`（survey）で対話生成（既存設定の再編集対応を含む）
- [x] 環境認識（Host/Container 判定、推奨マネージャ提案）

### Phase 3：Secrets（完了）

- [x] Bitwarden 連携（unlock / env 読み込み / env run）
- [x] 実行前ミドルウェア（必要時に解錠・環境変数注入）

### Phase 4：並列実行エンジン（完了）

- [x] `internal/runner`（errgroup + semaphore）
- [x] timeout/cancel/集計

### Phase 5：システム更新・リポジトリ管理（完了）

- [x] `sys update` / `sys list`
- [x] `repo update` / `repo list`
- [x] 並列実行への統合

### Phase 6：TUI 進捗表示（完了）

- [x] Bubble Tea によるマルチ進捗表示 + リアルタイムログ
- [x] `sys update` / `repo update` への統合

### Phase 7：運用安全性・完成度の強化（次の優先）

- [x] `repo sync` の安全運用を段階的に強化（危険状態はスキップして理由を表示）
- [x] 判定ロジックを table-driven tests で固定（境界値・失敗系を優先）
- [x] `config show` / `config validate` の追加
- [x] テスト拡充（`internal/config` / `internal/secret` / `internal/updater`）
- [x] `repo cleanup` の移植
- [ ] `sys` マネージャ拡張（flatpak/fwupdmgr/pnpm/nvm/uv/rustup/gem/winget/scoop 等）
  - 進捗（第1弾）: `flatpak` / `fwupdmgr` を追加し、`config init` / `sys list` / README の対応マネージャ表記を更新済み
  - 進捗（第2弾）: `pnpm` / `nvm` を追加し、`config init` / `sys update` / README の対応マネージャ表記を更新済み
  - 残件（第3弾以降）: `uv tool` / `rustup` / `gem` / `winget` / `scoop` と統合テスト
- [ ] リリース/CI（GoReleaser/GitHub Actions/E2E）
