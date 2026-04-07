# Current Tasks

現在進行中のタスクリストです。開発の進捗に合わせて随時更新してください。

> 過去の完了タスク履歴は [docs/archive/tasks_v0.2.3.md](docs/archive/tasks_v0.2.3.md) を参照してください。

---

## Issue #29: 一括pullが成功表示にも関わらず同期されない問題

### 修正1（高優先）: `refs/remotes/origin/HEAD` 未設定時のスキップを廃止

対象: `internal/repo/update.go`

- [x] `detectNonDefaultTrackingBranch()` で `getRemoteDefaultRef()` が失敗した場合、スキップではなく空文字（pull 許可）を返すよう修正
- [x] 修正に対応するユニットテストを追加（`internal/repo/update_test.go`）
- [ ] `scripts/branch-chk.ps1` で `spotify-ad-analyzer` の BEHIND 状態が解消されることを確認（実機検証）

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

- [ ] `AutoStash` が DIRTY リポジトリで機能するよう修正（上記方針に基づく実装）
- [ ] `repo list` コマンドに BEHIND カウントの表示を追加（現在は `Ahead` のみ）

### pull スキップのサマリー集約

**問題**: pull がスキップされても `Update()` は `error = nil` を返すため、
`runner.Summary` では `StatusSuccess` としてカウントされる。
最終サマリーの「スキップ: N 件」はタイムアウト/キャンセルのみで、
pull スキップは「成功」に混入している。

**実装方針**:

- `runner.Summary` に `PullSkipped int` フィールドを追加する
- または `Update()` の戻り値（`UpdateResult`）に `Skipped bool` フラグを持たせ、
  `buildRepoUpdateJobs()` 内でカウントして最後にサマリーへ集計する
- `printRepoUpdateSummary()` に「pull スキップ: N 件」行を追加する
- スキップしたリポジトリ名を末尾に一覧表示する

対象: `internal/runner/runner.go` / `cmd/dsx/repo.go`
