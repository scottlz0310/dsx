# Current Tasks

現在進行中のタスクリストです。開発の進捗に合わせて随時更新してください。

> 過去の完了タスク履歴は [docs/archive/tasks_v0.2.3.md](docs/archive/tasks_v0.2.3.md) を参照してください。

---

## Issue #29: 一括pullが成功表示にも関わらず同期されない問題

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
