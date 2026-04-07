# Current Tasks

現在進行中のタスクリストです。開発の進捗に合わせて随時更新してください。

> 過去の完了タスク履歴は [docs/archive/tasks_v0.2.3.md](docs/archive/tasks_v0.2.3.md) を参照してください。

---

## Issue #29: 一括pullが成功表示にも関わらず同期されない問題

### 修正1（高優先）: `refs/remotes/origin/HEAD` 未設定時のスキップを廃止

対象: `internal/repo/update.go`

- [ ] `detectNonDefaultTrackingBranch()` で `getRemoteDefaultRef()` が失敗した場合、スキップではなく空文字（pull 許可）を返すよう修正
- [ ] 修正に対応するユニットテストを追加（`internal/repo/update_test.go`）
- [ ] `scripts/branch-chk.ps1` で `spotify-ad-analyzer` の BEHIND 状態が解消されることを確認

### 修正2（中優先）: スキップ時の表示を「成功」から区別

対象: `cmd/dsx/repo.go`

- [ ] `printRepoUpdateResult()` で `SkippedMessages` が非空の場合、「✅ 成功」ではなく「⚪ スキップ（pull を実行しませんでした）」と表示
- [ ] TUI 側（`internal/tui/progress.go`）の表示も同様に対応

### 修正3（低優先）: pull 後の BEHIND チェック追加

対象: `internal/repo/update.go`

- [ ] `planAndRunPull()` 完了後に `git rev-list --count HEAD..@{u}` で BEHIND 残存を確認
- [ ] BEHIND が残っている場合、`SkippedMessages` に警告を追記
- [ ] テストを追加

---

## Backlog / 改善候補

- [ ] DIRTY + BEHIND リポジトリへの `--autostash` 対応（現在は DIRTY なら AutoStash 設定に関わらずスキップ）
- [ ] `repo list` コマンドに BEHIND カウントの表示を追加（現在は `Ahead` のみ）
