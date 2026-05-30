# Current Tasks

現在進行中のタスクリストです。開発の進捗に合わせて随時更新してください。

> 過去の完了タスク履歴は [docs/archive/tasks_v0.2.3.md](docs/archive/tasks_v0.2.3.md) を参照してください。

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

## v0.7.0 リリース準備

- [x] PR #82 が main にマージ済みであることを確認
- [x] `task check` 通過（fmt/vet/test/lint）
- [x] `go build ./...` 通過
- [x] `go run ./cmd/dsx --help` 表示確認
- [x] `task release:check` 通過
- [ ] タグ push 後の Release workflow で `go test -race ./...` 通過を確認（ローカル Windows は gcc 未導入のため未実施）
- [x] CHANGELOG.md: [Unreleased] → [v0.7.0] - 2026-05-30
- [x] README.md: バージョン表記を v0.7.0 に更新
- [ ] `v0.7.0` タグ発行・push → goreleaser が GitHub Release を自動作成

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
