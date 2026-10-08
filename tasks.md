# Current Tasks

現在進行中のタスクリストです。開発の進捗に合わせて随時更新してください。

> 過去の完了タスク履歴は [docs/archive/tasks_v0.2.3.md](docs/archive/tasks_v0.2.3.md) を参照してください。

## CI 保守: golangci-lint 更新と Renovate 自動追従

- [x] Go 1.27.2 更新に伴う `golangci-lint` の export data version 不整合エラーを特定
- [x] `.github/workflows/ci.yml` の `golangci-lint` を `v2.14.0` に更新
- [x] `lefthook.yml` の pre-push lint を `task lint` に委譲する
- [x] Taskfile の gitleaks インストールバージョンを CI と同じ `v8.30.1` に揃える
- [x] 共有 Go プリセットの regex manager が workflow 内 Go CLI ツールを検出することを検証
- [ ] PR を作成し、レビューサイクルを完了して main にマージ
- [ ] PR #129 を main と同期させて CI パスを確認

---

## Issue #131: Renovate 設定を共有プリセットに集約

- [x] `renovate-config#273` の対応が共有 Go プリセットにマージ済みであることを確認
- [x] `renovate.json` からローカル `customManagers` を削除し、`golangci-lint` と `gitleaks` が共有設定の対象になることを確認
- [x] `pnpm dlx --package renovate renovate-config-validator` で設定を検証
- [ ] PR のレビューサイクルを完了して main にマージ

---

## v0.10.1 リリース準備

- [x] PR #125 を main にマージし、Issue #122 がクローズしたことを確認
- [x] `task check` / `go build ./...` / `go run ./cmd/dsx --help` 通過（PR CI を含む）
- [x] MSIX 署名用 GitHub Secrets の登録を確認
- [x] CHANGELOG.md を v0.10.1 として更新
- [x] README.md の最新バージョン表記を v0.10.1 に更新
- [x] `task release:check` / `task snapshot` 通過
- [x] リリース準備 PR #127 を作成し、レビュー後にマージ
- [x] `v0.10.1` タグを push。Release workflow #36820719995 が成功し、GitHub Release と署名済み MSIX を公開

---

## v0.10.0 リリース準備

- [x] PR #121 を main にマージ
- [x] `task check` / `go build ./...` / `go run ./cmd/dsx --help` 通過（PR CI を含む）
- [x] MSIX 署名用 GitHub Secrets の登録を確認
- [x] CHANGELOG.md を v0.10.0 として更新
- [x] README.md の最新バージョン表記を v0.10.0 に更新
- [x] `task release:check` / `task snapshot` 通過
- [x] リリース準備 PR #123 を作成
- [x] PR #123 をレビュー後にマージ（merge commit `9f29cb4`）
- [x] `v0.10.0` タグ発行・push。Release workflow #36796377654 が成功し、GitHub Release と MSIX の公開を確認

---

## v0.9.0 リリース準備

- [x] PR #113 / #114 が main にマージ済みであることを確認
- [x] `SIGNING_CERTIFICATE_BASE64` / `SIGNING_CERTIFICATE_PASSWORD` を GitHub Secrets に登録
- [x] `task check` 通過（fmt/vet/test/lint）
- [x] `go build ./...` 通過
- [x] `go run ./cmd/dsx --help` 表示確認
- [x] `task release:check` 通過
- [x] CHANGELOG.md: [Unreleased] → [v0.9.0] - 2026-09-10
- [x] README.md: バージョン表記とリリース方針を v0.9.0 に更新
- [x] リリース準備 PR を作成（PR #115）
- [x] `v0.9.0` タグ発行・push → goreleaser が GitHub Release を自動作成

---

## Issue #119: pnpm 本体の self-update

- [x] `CheckSelfUpdate` で prerelease を含む SemVer 順序により現在版と registry の最新版を読み取り専用で比較
- [x] 通常実行では `pnpm self-update` を呼び出し、グローバルパッケージ更新とは別結果にする
- [x] DryRun で自己更新を実行せず、pnpm の pin 設定と Corepack の project spec を無効化する子プロセス環境を固定
- [x] レビュー指摘: Corepack Shim 経由の本体更新を回帰テストで固定
- [x] レビュー指摘: 同一 core の prerelease 更新を検出し SemVer 順序をテスト
- [x] 呼び出し元の作業ディレクトリを維持して NVM for Windows Shim の Node.js 選択を保つ
- [x] `internal/updater/pnpm_test.go` に候補比較・DryRun・成功・失敗の table-driven tests を追加
- [x] pnpm 12.8.1 で pinned packageManager の一時プロジェクトから確認コマンドを実行し、`package.json` / lockfile が不変であることを確認
- [x] `CHANGELOG.md` / `README.md` / `docs/Implementation_Plan.md` を更新
- [x] NVM Shim 実機確認を別 Issue #122 に引き継ぎ（リリース非ブロッカー）
- [x] `task check` / `go build ./...` / `go run ./cmd/dsx --help` を確認

---

## Issue #122: NVM for Windows v2 Shim と NVM 管理下 pnpm の実機更新確認

- [x] NVM for Windows v2 Shim mode は選択中 Node.js の npm global で pnpm を更新し、`nvm reshim` 後に版を確認
- [x] Shim 判定・npm global 更新・reshim・更新後バージョン確認の成功 / 失敗系を table-driven tests で検証
- [x] PR レビュー対応: NVM v2 / Shim 判定と更新後バージョン確認の失敗経路を追加し、pnpm updater のカバレッジを確認
- [x] 実機確認: NVM for Windows v2.0.0、Node.js v24.21.0、pnpm 12.4.2 → 12.8.1。`node -v` / `process.execPath` / `npm root -g` は更新前後で同一の NVM Node.js を指す
- [x] 実機確認: `package.json` / `pnpm-lock.yaml` の SHA-256 は前後一致。project pin は pnpm 12.4.2 のまま、pin を無効にした pnpm と npm global 一覧は 12.8.1
- [x] standalone pnpm 12.8.1 と global package inventory / lockfile hash に変化がないことを確認
- [x] NVM v24 の global tools を `corepack@0.36.0` / `npm@11.19.0` に復元し、NVM link mode / default Node.js v26.10.0 を復元

---

## v0.8.1 リリース準備

- [x] PR #102 が main にマージ済みであることを確認
- [x] `task check` 通過（fmt/vet/test/lint）
- [x] `go build ./...` 通過
- [x] `go run ./cmd/dsx --help` 表示確認
- [x] `task release:check` 通過
- [x] CHANGELOG.md: [Unreleased] → [v0.8.1] - 2026-07-25
- [x] README.md: バージョン表記を v0.8.1 に更新
- [x] リリース準備 PR を作成
- [ ] `v0.8.1` タグ発行・push → goreleaser が GitHub Release を自動作成

---

## Issue #111: MSIX 版での self-update と config init の対応（BREAKING: Windows の go install 廃止）

方針: Windows は MSIX 版のみサポート / Linux・macOS は go install を維持 / 並存時の警告は PR B のインストーラーで扱う

- [x] `internal/msix`: `GetCurrentPackageFullName` によるパッケージ実行判定（Windows 以外は常に false）とインストールワンライナー定数
- [x] `dsx self-update`: Windows では go install せず、MSIX 版は自動更新を案内・MSIX 版以外は移行手順付きでエラー
- [x] 実行後の更新通知の案内文を実行環境ごとに切り替え
- [x] `dsx config init`: MSIX 版では実行エイリアス（`%LOCALAPPDATA%\Microsoft\WindowsApps\dsx.exe`）を埋め込む
- [x] table-driven tests（OS / パッケージ判定を差し替え、判定失敗の伝播を含む）
- [x] `CHANGELOG.md`（BREAKING と移行手順）/ `README.md`（Windows 非サポートの注記）
- [x] 開発者モード登録での実機確認: alias 経由は自動更新案内（exit 0）、パッケージ外は移行案内エラー（exit 1）、パッケージフォルダの実体パス直接実行はパッケージ外と判定されることを確認（`config init` は対話専用のため埋め込みパスは単体テストで確認）
- [x] PR 作成・レビュー・マージ

---

## Issue #104: PowerShell ワンライナー (irm ... | iex) による証明書信頼設定と MSIX/.appinstaller 自動インストールの提供

- [x] GitHub Issue #104 を tacho-graph-studio 実績ベースのワンライナー MSIX/Appinstaller 方針に更新
方針: 署名証明書は dsx 専用に新規生成 / 信頼ストアは `LocalMachine\TrustedPeople` / 配布 URL は `releases/latest/download/install.ps1` / PR は 2 分割

### PR A: MSIX パッケージ化とリリースパイプライン（PR #112）

- [x] `packaging/msix/AppxManifest.xml`（`AppExecutionAlias`・書き込み仮想化無効化）と `.appinstaller` テンプレート
- [x] `packaging/msix/Build-Msix.ps1`（makeappx / signtool / .cer / .appinstaller 生成）
- [x] `packaging/msix/New-SigningCertificate.ps1`（dsx 専用署名証明書の生成）
- [x] `release.yml` に `msix` job 追加（goreleaser の Windows バイナリを署名済み MSIX 化して Release に追加）
- [x] `ci.yml` に未署名 MSIX パッケージ化検証 job 追加
- [x] ローカル検証（未署名/署名付きビルド、開発者モード登録で alias 起動・仮想化無効を確認）
- [x] golangci-lint の固定バージョンを v2.11.4 → v2.13.2 に更新（`lefthook.yml` / `ci.yml`。v2.11.4 は Go 1.27 の export data を読めず pre-push が失敗するため）
- [ ] **次回リリース前に必須**: `New-SigningCertificate.ps1` を実行し `SIGNING_CERTIFICATE_BASE64` / `SIGNING_CERTIFICATE_PASSWORD` を dsx リポジトリの Secrets に登録（ユーザー作業）

### PR B: インストーラースクリプト

- [x] ワンライナー対応インストーラースクリプト (`scripts/install.ps1`) の作成（証明書信頼登録 ＋ `.appinstaller` 導入、昇格は証明書インポートのみ）
- [x] Windows PowerShell 5.1 での日本語復号を検証: Release アセットは `application/octet-stream` 配信のため `irm | iex` では文字化けする（pwsh 7 は正常）。`iwr` の Byte[] を UTF-8 で明示復号すれば 5.1 / 7 とも正常
- [x] 配布ワンライナーを決定: `iex ([Text.Encoding]::UTF8.GetString((iwr -UseBasicParsing https://github.com/scottlz0310/dsx/releases/latest/download/install.ps1).Content))`
- [x] PR A マージ後に着手（AGENTS.md: レビュー対応中はサブ PR を作成しない）
- [x] Pester テストと CI job の追加
- [x] `release.yml` で `install.ps1` を Release アセットに追加
- [x] ドキュメント更新（`README.md` にワンライナー導入手順を追加）
- [x] UTF-8 BOM が `iex` で誤解釈されないよう、ワンライナーの実行例を更新

### 後続 Issue

- [x] Issue #111 作成: MSIX 版での `dsx self-update`（go install）と `config init`（バージョン付き実体パスの埋め込み）の対応

---

## Issue #101: pnpm update -g 実行時のインタラクティブプロンプトによるタイムアウト防止

- [x] `internal/updater/pnpm.go` に `--no-interactive` フラグおよび `CI=true` 環境変数を追加
- [x] `internal/updater/pnpm_test.go` に `--no-interactive` フラグと `CI=true` のテスト検証を追加
- [x] `CHANGELOG.md` 更新
- [x] `task check` 通過
- [x] コミット・push & PR #102 作成

---

## Issue #1: dsx repo branch-clean サブコマンド実装

- [x] `internal/repo/branch_scan.go` 実装（4カテゴリ検出ロジック）
- [x] `internal/repo/branch_clean.go` 実装（削除・prune ロジック）
- [x] `cmd/dsx/repo_branch_clean.go` 実装（Cobra コマンド定義・インタラクティブ/dry-run/yes モード）
- [x] `internal/repo/branch_scan_test.go` テスト追加（table-driven・境界値・エラー系）
- [x] `task check` 通過（fmt/vet/test/lint 全件パス、lint 0 issues）
- [x] `CHANGELOG.md` 更新
- [x] feature ブランチ作成・コミット・push
- [x] Draft PR 作成（Closes #1）
- [x] PR #66 マージ

---

## v0.6.0 リリース準備

- [x] `dsx repo branch-clean --help` 実機確認
- [x] `dsx repo branch-clean --dry-run --no-fetch` 実機確認
- [x] README.md に `repo branch-clean` の使用方法を追加
- [x] ルートヘルプに `repo branch-clean` を追加
- [x] CHANGELOG.md: [Unreleased] → [v0.6.0] - 2026-05-15
- [x] README.md: バージョン表記を v0.6.0 に更新
- [x] PR 作成
- [x] PR マージ
- [x] `v0.6.0` タグ発行・push → goreleaser が GitHub Release を自動作成
- [x] GitHub Release ページを見やすく編集

---

## dsx env: ロック済み BW_SESSION の再アンロック対応

- [x] `dsx env unlock ; dsx env export` 失敗時の挙動を調査
- [x] `dsx env export` / `dsx env run` がロック済み `BW_SESSION` を自動再アンロックするよう修正
- [x] `dsx config init` 生成シェル関数の `dsx-env` でロック済み `BW_SESSION` を検知して再アンロックするよう修正
- [x] table-driven tests で未設定・ロック済み・アンロック済み・`--sync` 経路を固定
- [x] `CHANGELOG.md` / `README.md` 更新
- [x] PR レビューコメント対応: `dsx env status --quiet` 追加、シェル連携の状態判定を Go 側へ集約、非対話環境のエラーを明確化
- [x] PR #68 マージ

---

## v0.6.1 リリース準備

- [x] `.gitignore` に `.claude/`（Claude Code 個人作業領域）を追加
- [x] CHANGELOG.md: [Unreleased] → [v0.6.1] - 2026-05-19
- [x] README.md: バージョン表記を v0.6.1 に更新
- [x] `task check` 実機確認（fmt/vet/test/lint）
- [x] PR 作成（PR #69）
- [x] PR マージ
- [x] `v0.6.1` タグ発行・push → goreleaser が GitHub Release を自動作成
- [x] GitHub Release ページを見やすく編集

---



## Issue #29: dsx repo update のブランチ更新状態確認スクリプト連携

### 修正1（高優先）: `refs/remotes/origin/HEAD` 未設定時のスキップを廃止

対象: `internal/repo/update.go`

- [x] `detectNonDefaultTrackingBranch()` で `getRemoteDefaultRef()` が失敗した場合、スキップではなく空文字（pull 許可）を返すよう修正
- [x] 修正に対応するユニットテストを追加（`internal/repo/update_test.go`）
- [x] `scripts/branch-chk.ps1` で `spotify-ad-analyzer` の BEHIND 状態が解消されることを確認（実機検証）

### 修正2（中優先）: スキップ時の表示を「成功」から区別

対象: `cmd/dsx/repo.go`

- [x] `printRepoUpdateResult()` で `SkippedMessages` が非空の場合、「✅ 成功」ではなく「⚪ スキップ（pull を実行しませんでした）」と表示
- [ ] TUI 側（`internal/tui/progress.go`）の表示も同様に対応（別 PR）

### 修正3（低優先）: pull 後の BEHIND チェック追加

対象: `internal/repo/update.go`

- [x] `planAndRunPull()` 完了後に `git rev-list --count HEAD..@{u}` で BEHIND 残存を確認
- [x] BEHIND が残っている場合、`SkippedMessages` に警告を追記
- [x] テストを追加（`TestGetBehindCount`）

---

## Issue #45: GoBinaryInfo構造体定義・ParseGoBinaryInfo実装（完了）

- [x] `GoBinaryInfo` 構造体定義（6フィールド）
- [x] `ParseGoBinaryInfo(binaryPath, output)` 実装（path行・mod行の分離、scanner.Err()チェック）
- [x] `UpdateTarget()` メソッド実装（ポインタレシーバ、nilガード付き）
- [x] `ParseGoVersionOutput` 削除
- [x] テスト追加（table-driven、境界値・nilガード含む）
- [x] PR #49 マージ → main

---

## Issue #46: DiscoverGoBinaries / DiscoverGoBinariesInDir 実装（完了）

- [x] `DiscoverResult` / `SkippedBinary` 構造体定義
- [x] `discoverInDir` 実装（バックアップファイル除外・context キャンセル早期 return）
- [x] `DiscoverGoBinariesInDir` 実装
- [x] `DiscoverGoBinaries` 実装（GOBIN/GOPATH 複数エントリ対応・空エントリ ~/go フォールバック）
- [x] `runGoVersionM` を `runCommandOutputWithLocaleC` ベースに変更
- [x] テスト追加（table-driven、context キャンセル・GOBIN 優先 など）
- [x] CHANGELOG.md 更新
- [x] PR #51 作成・Copilot レビュー 3 サイクル対応（計 8 スレッド全件 accept）

---

## Issue #47: dsx sys discover コマンド実装（PR #53、マージ済み）

- [x] `cmd/dsx/sys_discover.go` 実装（`dsx sys discover` コマンド）
- [x] `cmd/dsx/sys_discover_test.go` テスト追加（table-driven・境界値・エラー系）
- [x] `task check` 通過
- [x] PR #53 作成・Copilot レビュー 3 サイクル対応（計 8 スレッド全件 accept）
- [x] CHANGELOG.md 更新
- [x] PR #53 マージ

---

## Issue #48: targets 未設定メッセージ改善 + テスト追加

- [x] `internal/updater/go.go` の `Check()`/`Update()` 内メッセージを改善（`dsx sys discover` 誘導ヒント追加）
- [x] `internal/updater/go_test.go` に `TestGoUpdater_Check_EmptyTargets` 追加（改善後メッセージ文言検証）
- [x] `task check` 通過（テスト全件パス）
- [x] CHANGELOG.md 更新
- [x] PR 作成・マージ（PR #48、main にマージ済み）

---

## Issue #56: dsx sys discover --apply / --dry-run 実装

- [x] `--dry-run` 単独指定時のエラー処理（`--apply` 必須）
- [x] `--apply` フラグ追加（`cmd/dsx/sys_discover.go`）
- [x] `--apply --dry-run` フラグ追加（変更プレビュー表示）
- [x] 重複排除マージロジック実装（`mergeGoTargets`）
- [x] パッケージパス正規化（`packagePathFrom`、`strings.LastIndex` 使用）
- [x] 既存 pinned バージョンは変更しない設計（PackagePath ベースで重複判定）
- [x] `config.SaveAtomic()` 実装（バックアップ + atomic write）
- [x] dry-run 時のコメント喪失警告・新規作成予告メッセージ
- [x] テスト追加（`TestPackagePathFrom`・`TestMergeGoTargets`・`TestPrintGoApplyDryRun_*`・`TestSaveAtomic`）
- [x] `task check` 通過（fmt/vet/test/lint）
- [x] `CHANGELOG.md` 更新
- [x] PR 作成・マージ（close #56）→ PR #57 マージ済み

---

## pnpm v11 対応: parseOutdatedJSON が WARNING 行で失敗する問題の修正

- [x] `internal/updater/pnpm.go` の `parseOutdatedJSON` で `[WARN]`/`[ERR]`/`[ERROR]` 行をフィルタリングしてから JSON 抽出するよう修正
- [x] `internal/updater/pnpm_test.go` に `[WARN]` 混入ケースおよび JSON なし出力のテストケースを追加
- [x] `task check` 通過（全テストパス）
- [x] `CHANGELOG.md` 更新
- [x] PR 作成・マージ（PR #59 マージ済み）

---

## Codecov カバレッジ計測の有効化 + v0.4.1 リリース準備

- [x] `ci.yml` に `CODECOV_TOKEN` を追加（`fail_ci_if_error: true` に変更）
- [x] `codecov.yml` を新規追加（閾値・PR コメント設定）
- [x] `CHANGELOG.md` を `[v0.4.1]` としてリリース準備
- [x] PR 作成・マージ（PR #60 マージ済み）
- [x] `v0.4.1` タグ発行・push → GoReleaser が GitHub Release を自動作成
- [x] GitHub Release ページを日本語で編集

---

## Issue #63: Go updater の installed/latest 比較と dsx 本体除外

- [x] Issue #63 本文をコードベースと照合し、既実装・矛盾点・リスクを整理
- [x] `dsx self-update` の更新判定ロジックを `internal/selfupdate` に分離
- [x] Go updater で `go version -m` 由来の installed と `go list -m -json <module>@latest` の latest を比較
- [x] latest 一致時は `go install` をスキップし、判定不能・固定バージョン target は従来通り `go install` 対象にする
- [x] `github.com/scottlz0310/dsx/cmd/dsx` は Go updater で `go install` せず、更新ありの場合のみ `dsx self-update` 誘導エラーにする
- [x] table-driven tests を追加（最新版スキップ・更新あり・判定不能・固定バージョン・dsx 本体例外・latest 取得キャッシュ）
- [x] `CHANGELOG.md` / `README.md` を更新
- [x] `task check` 通過
- [x] `task test` 通過
- [x] `go build ./...` 通過
- [x] `dsx --help` 表示確認（`go run ./cmd/dsx --help` で現行コードを確認）
- [x] コミット・push

---

## v0.5.0 リリース準備

- [x] `task check` 通過確認（fmt/vet/test/lint）
- [x] バージョン: v0.5.0（マイナーバージョンアップ）
- [x] CHANGELOG.md: [Unreleased] → [v0.5.0] - 2026-05-11
- [x] README.md: Go updater 最新版判定機能・バージョン表記を更新
- [x] PR 作成・マージ
- [x] `release/v0.5.0` ブランチ削除（ローカル + リモート参照プルーン）
- [x] `v0.5.0` タグ発行・push → goreleaser が GitHub Release を自動作成
- [x] GitHub Release ページを見やすく編集
- [x] Issue #63 クローズ確認

---

## v0.6.3 リリース準備

- [x] PR #71 マージ（bw stdout 混入対応・JSON パース堅牢化）
- [x] CHANGELOG.md: v0.6.3 セクション確認（2026-05-20）
- [x] README.md: バージョン表記を v0.6.3 に更新
- [x] PR 作成・マージ
- [x] `v0.6.3` タグ発行・push → goreleaser が GitHub Release を自動作成
- [x] GitHub Release ページを見やすく編集

---

## Issue #74: cargo 更新パフォーマンス最適化

### Phase 1: cargo-update 自動インストール（完了）

- [x] `internal/updater/cargo.go`: cargo-update 未インストール時に `cargo install cargo-update` で自動インストールし `cargo install-update -a` 経路に統一
- [x] `internal/updater/cargo_test.go`: 自動インストール成功・失敗テストケース追加
- [x] `CHANGELOG.md` 更新
- [x] PR #75 作成・マージ（359bdc2）

### Phase 2: Check() 事前実行のスキップ（完了）

- [x] cargo-update 経路では `cargo install --list` を省略

### Phase 3: UpdatedCount 表示バグ修正（完了）

- [x] `cargo install-update -a` の出力をパースし、`Updated N package(s).` サマリ行から更新件数を取得

### Phase 4: 細部の整理（完了）

- [x] `exec.LookPath` への変更（Phase 1 で実施済み）
- [x] `cmd.Stdin` 接続の削除（非対話コマンドのため不要）

---

## v0.6.4 リリース準備

- [x] cargo 更新処理の最適化・修正 PR #75〜#79 が main にマージ済みであることを確認
- [x] PR #79 マージ後の実機テストを実施（cargo-update v20.0.0 / `dsx sys update` cargo 経路）
- [x] `task check` 通過（fmt/vet/test/lint）
- [x] `go build ./...` 通過
- [x] `dsx --help` 表示確認（`dist/dsx.exe --help`）
- [x] CHANGELOG.md: [Unreleased] → [v0.6.4] - 2026-05-28
- [x] README.md: バージョン表記を v0.6.4 に更新

---

## Issue #81: sys update マネージャ本体更新フェーズ

- [x] `ManagerSelfUpdater` / `CheckSelfUpdate` / `SelfUpdate` を追加し、既存 `Updater` インターフェースを壊さない拡張点を用意
- [x] `sys update` の中央実行フローに「マネージャ本体更新フェーズ → 通常更新フェーズ」を追加
- [x] `uv` updater で `uv self update --dry-run` / `uv self update` に対応
- [x] `uv self update` 非対応のインストール経路では本体更新をスキップし、通常更新フェーズを継続
- [x] `pnpm` のグローバル更新を `pnpm update -g --latest` に変更
- [x] dry-run が `SelfUpdate` ではなく `CheckSelfUpdate` のみを呼ぶことをテストで固定
- [x] self update 後に通常更新を継続するかスキップするかを `SelfUpdateContinuation` で表現し、スキップ時の挙動をテストで固定
- [x] `CHANGELOG.md` / `README.md` / `docs/Implementation_Plan.md` を更新

---

## Issue #96: Bun のグローバルツール更新とマネージャ本体更新

- [x] `BunUpdater` を updater レジストリへ追加
- [x] `bun outdated -g` / `bun update -g --latest` によるグローバルパッケージ更新に対応
- [x] Bun 管理のインストールで `bun upgrade` による本体更新に対応
- [x] Homebrew / Scoop 管理下または所有元不明の本体更新を安全にスキップ
- [x] dry-run、出力 parser、境界値、失敗系、インストール経路判定を table-driven tests で固定
- [x] `config init` / `sys update --help` / `sys list` に Bun を統合
- [x] `CHANGELOG.md` / `README.md` / `tasks.md` / `docs/Implementation_Plan.md` を更新
- [x] `task test` / `task check` / `go build ./...` / `dsx --help` の検証

---

## v0.7.0 リリース準備

- [x] PR #82 が main にマージ済みであることを確認
- [x] `task check` 通過（fmt/vet/test/lint）
- [x] `go build ./...` 通過
- [x] `go run ./cmd/dsx --help` 表示確認
- [x] `task release:check` 通過
- [ ] タグ push 後の Release workflow で `go test -race ./...` 通過を確認（ローカル Windows は gcc 未導入のため未実施）
- [x] CHANGELOG.md: [Unreleased] → [v0.7.0] - 2026-05-30
- [x] README.md: バージョン表記を v0.7.0 に更新
- [x] `v0.7.0` タグ発行・push → goreleaser が GitHub Release を自動作成

---

## v0.8.0 リリース準備

- [x] PR #97 が main にマージ済みであることを確認
- [x] `task check` 通過（fmt/vet/test/lint）
- [x] `go build ./...` 通過
- [x] `go run ./cmd/dsx --help` 表示確認
- [x] `task release:check` 通過
- [x] CHANGELOG.md: [Unreleased] → [v0.8.0] - 2026-07-16
- [x] README.md: バージョン表記とリリース方針を v0.8.0 に更新
- [x] リリース準備 PR #98 をレビューレディで作成
- [ ] `v0.8.0` タグ発行・push → goreleaser が GitHub Release を自動作成

---

## Backlog / 改善候補

### `AutoStash` オプションの修正（設定が機能していないバグ）

**問題**: `AutoStash=true` に設定しても、DIRTY チェック（`detectUnsafeRepoState`）が先に走るため
pull がスキップされ `git pull --rebase --autostash` が一切実行されない。
設定として存在するのに意味をなさない状態であり、対応するテストも存在しない。

**実装方針**:

- `AutoStash=true` かつ DIRTY の場合は、スキップせずに `git pull --rebase --autostash` を実行する
- `AutoStash=false`（デフォルト）の場合は現在と同様に DIRTY でスキップする
- 具体的には `buildUnsafeMessages()` または `planAndRunPull()` の分岐で `AutoStash` を参照する

**必要なテスト**:

- `AutoStash=true` + DIRTY リポジトリ → pull が実行されることを検証
- `AutoStash=false` + DIRTY リポジトリ → 現在通りスキップされることを検証（回帰）
- `AutoStash=true` + DIRTY + pull 成功 → SkippedMessages が空であることを検証

対象: `internal/repo/update.go` / `internal/repo/update_test.go`

---

- [x] `AutoStash` が DIRTY リポジトリで機能するよう修正（上記方針に基づく実装）
- [x] `repo list` コマンドに BEHIND カウントの表示を追加（現在は `Ahead` のみ）

### pull スキップのサマリー集約

- [x] `buildRepoUpdateJobs()` 内で pull スキップ発生時にリポジトリ名を収集
- [x] `printRepoUpdateSummary()` に「pull スキップ: N 件」行と一覧を追加

対象: `cmd/dsx/repo.go`

---

## v0.4.0 リリース準備（完了）

- [x] `task check` 通過確認（fmt/vet/test/lint）
- [x] バージョン: v0.4.0（マイナーバージョンアップ）
- [x] CHANGELOG.md: [Unreleased] → [v0.4.0] - 2026-05-09
- [x] PR 作成・マージ（[#58](https://github.com/scottlz0310/dsx/pull/58)）
- [x] `release/v0.4.0` ブランチ削除（ローカル + リモート参照プルーン）
- [x] `v0.4.0` タグ発行・push → goreleaser が GitHub Release を自動作成
- [x] GitHub Release ページを見やすく編集
- [x] Issue #56 クローズ済み（PR #57 マージ時に自動クローズ）

---

## v0.3.0 リリース準備（完了）

- [x] `task check` 通過確認（fmt/vet/test/lint）
- [x] バージョン: v0.3.0（マイナーバージョンアップ）
- [x] CHANGELOG.md: [Unreleased] → [v0.3.0] - 2026-05-09
- [x] README.md: dsx sys discover 追加・バージョン更新
- [x] root.go Long description: dsx sys discover 追記
- [x] PR 作成・マージ（[#55](https://github.com/scottlz0310/dsx/pull/55)）
