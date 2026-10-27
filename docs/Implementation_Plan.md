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

- `dsx run`（Bitwarden解錠 → env注入 → `sys update` → `repo update`）まで到達
- `repo update` は並列実行 + サブモジュール更新に対応
- `sys update` は主要マネージャを実装済み（apt/brew/go/npm/pnpm/nvm/pipx/cargo/snap/flatpak/fwupdmgr/uv/rustup/gem）
- 進捗UIは `--tui` で起動（設定 `ui.tui` で既定ON/OFFも可能）

## 3. CLI コマンド設計（v0.1 時点）

- 日次統合: `dsx run`
- 診断: `dsx doctor`
- システム更新: `dsx sys update`, `dsx sys list`
- リポジトリ管理: `dsx repo update`, `dsx repo list`
- 環境変数: `dsx env export`, `dsx env run`
- 設定: `dsx config init`, `dsx config uninstall`

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
  - 進捗（第3弾）: `uv tool` / `rustup` / `gem` を追加し、`config init` / `sys list` / README の対応マネージャ表記を更新済み
  - 進捗（第4弾）: `sys update` にマネージャ本体更新フェーズを追加し、`uv self update` と `pnpm update -g --latest` に対応
  - 進捗（第5弾）: `bun` を追加し、グローバルパッケージ更新とインストール経路に応じた Bun 本体更新に対応
  - 進捗（第6弾）: `pnpm self-update` による本体更新に対応。DryRun は prerelease を含む SemVer 順序で読み取り専用の候補確認に留め、pnpm 設定と `COREPACK_ENABLE_PROJECT_SPEC=0` でプロジェクト pin を避けながら、NVM Shim の作業ディレクトリ選択を維持
  - 進捗（第7弾）: NVM for Windows v2 Shim mode では選択 Node.js のグローバル pnpm を npm で更新し、`nvm reshim` と更新後の版確認を実施。Issue #122 の実機確認でプロジェクト pin / lockfile の不変と standalone 版への影響がないことを確認
  - 残件（第7弾以降）: 統合テスト
- [ ] リリース/CI（GoReleaser/GitHub Actions/E2E）

## 6. グローバルツールのスナップショット（#91）

### Phase 1: 構成の保存

- `dsx sys snapshot --output dsx-tools.yaml` で、Go / npm / pnpm / pipx / uv / Cargo / RubyGems / winget の一覧を保存する。
- `Updater` の必須契約を変えず、任意の `SnapshotProvider` を追加する。各マネージャは一覧の取得・解析を担当し、中央処理は利用可否判定・集約・エラー集計、`internal/snapshot` は共通スキーマの検証・ソート・保存を担当する。
- スキーマは `version: 1` と `tools.<manager>[]`。全マネージャで `package`、任意の `version`、winget の任意の `source` に統一する。既存の YAML ライブラリを使用し、新規依存は追加しない。
- 更新対象の選択と現在構成の記録は目的が異なるため、`sys.enable` / `go.targets` に依存せず対応マネージャを検出する。マネージャ固有の更新設定は適用しない。
- 取得・解析・検証の失敗は部分成功として保存しない。未対応・利用不可は日本語で区別する。対応対象がないときもエラー。出力順は安定化し、完全重複を除去する。
- 必要な識別子・版・source 名だけを許可し、取得元 URL・認証情報・ローカルパスを保存しない。CLI の生データや生エラー出力をログに流さない。
- 既存ファイルの上書きには `--force` が必要。新規作成は排他的に開き、上書き時は同じディレクトリの一時ファイルから置換する。取得・検証失敗時は既存内容を保護する。
- 外部 CLI は関数注入で fake に差し替え、parser・正規化・空一覧・不正形式・失敗集計・上書き保護を table-driven tests で検証する。

### Phase 2: 復元計画

- `sys restore --file <path> --dry-run` を追加する。現段階では dry-run が必須で、インストールを実行する経路を持たない。
- `snapshot.Decode` で未知フィールド・重複キー・複数文書・不正な識別子を拒否する。既存のスキーマ version 1 とフィールドを維持する。
- `internal/restore` は注入された一覧取得・提供版照会を使って、既存一致・追加候補・解決不能を判定する。`Updater` の必須契約は変更しない。
- `internal/updater` で初期8マネージャの一覧取得（既存 SnapshotProvider）とメタデータ照会を接続する。照会は CLI の view/show または公開レジストリの HTTP GET に限定し、インストールコマンドは表示用データとしてだけ生成する。
- マネージャ未導入・OS 非互換・source/指定版不在・取得失敗は解決不能として項目を残し、コマンド全体は失敗とする。未指定のマネージャは調べない。
- 既存の版・source は変更しない。RubyGems の複数版の併存は追加として扱う。矛盾する指定は解決不能にする。
- 提供版照会の詳細・制約は README の表を正とする。メタデータの確認をビルドや実インストールの成功保証として扱わない。
- すべての外部依存を関数・HTTP client で差し替え、正常系に加えて失敗・キャンセル・不正 manifest・既存保護を table-driven tests で固定する。

### 後続: Phase 3（復元実行、未実装）
- Phase 3: 明示的な復元実行。既存ツールの削除・ダウングレードは自動実行しない。
- manifest は lockfile ではなく再構築の入力とし、生成後の手動編集を正式なフローとする。取得経路・追加オプション・推移的依存などの完全再現は保証しない。
