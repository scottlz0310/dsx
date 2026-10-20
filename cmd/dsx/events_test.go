package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scottlz0310/dsx/internal/event"
	repomgr "github.com/scottlz0310/dsx/internal/repo"
	"github.com/scottlz0310/dsx/internal/runner"
	"github.com/scottlz0310/dsx/internal/updater"
)

// 注意: activeEvents はパッケージのグローバル変数です。これを書き換えるテストは t.Parallel を使いません
// （並列テストは、直列のテストがすべて終わってから動くため、競合しません）。

type recordedEvent map[string]any

func parseEventLines(t *testing.T, data []byte) []recordedEvent {
	t.Helper()

	var events []recordedEvent

	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if line == "" {
			continue
		}

		var ev recordedEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("イベントの JSON 解析に失敗: %v\n行: %s", err, line)
		}

		events = append(events, ev)
	}

	return events
}

// useBufferedEvents は activeEvents をバッファに向け、後始末で nil に戻します。
func useBufferedEvents(t *testing.T, phase string) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer

	activeEvents = event.New(&buf, "test", "dev", nil)
	activeEvents.SetPhase(phase)

	t.Cleanup(func() { activeEvents = nil })

	return &buf
}

// evString はイベントの文字列フィールドを返します。文字列でなければ空文字を返します。
func evString(ev any, key string) string {
	fields, ok := ev.(map[string]any)
	if !ok {
		if recorded, isRecorded := ev.(recordedEvent); isRecorded {
			fields = recorded
		}
	}

	if s, isString := fields[key].(string); isString {
		return s
	}

	return ""
}

func finishedJobs(events []recordedEvent) map[string]recordedEvent {
	jobs := map[string]recordedEvent{}

	for _, ev := range events {
		if ev["type"] == "job_finished" {
			jobs[evString(ev, "name")] = ev
		}
	}

	return jobs
}

func TestUpdaterJobResult(t *testing.T) {
	tests := []struct {
		name        string
		result      *updater.UpdateResult
		err         error
		wantStatus  event.Status
		wantCode    event.Code
		wantUpdated int
		wantReason  string
	}{
		{name: "result が nil で エラーなしは成功", wantStatus: event.StatusSuccess},
		{name: "更新件数が記録される", result: &updater.UpdateResult{UpdatedCount: 3}, wantStatus: event.StatusSuccess, wantUpdated: 3},
		{
			name:       "部分的なエラーは失敗（終了コード 1 の原因）",
			result:     &updater.UpdateResult{UpdatedCount: 1, Errors: []error{errors.New("a の更新に失敗"), errors.New("b の更新に失敗")}},
			wantStatus: event.StatusFailed, wantCode: event.CodeError, wantUpdated: 1, wantReason: "a の更新に失敗; b の更新に失敗",
		},
		{
			name:       "エラー文なしで FailedCount だけでも失敗",
			result:     &updater.UpdateResult{FailedCount: 2},
			wantStatus: event.StatusFailed, wantCode: event.CodeError, wantReason: "2 件の更新に失敗しました",
		},
		{name: "通常のエラーは失敗", err: errors.New("exit status 100"), wantStatus: event.StatusFailed, wantCode: event.CodeError, wantReason: "exit status 100"},
		{name: "キャンセルはスキップ", err: context.Canceled, wantStatus: event.StatusSkipped, wantCode: event.CodeCancelled},
		{name: "タイムアウトはスキップ", err: context.DeadlineExceeded, wantStatus: event.StatusSkipped, wantCode: event.CodeCancelled},
		{name: "ラップされたキャンセルもスキップ", err: errors.Join(errors.New("apt"), context.Canceled), wantStatus: event.StatusSkipped, wantCode: event.CodeCancelled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := updaterJobResult(tt.result, tt.err)

			if got.Status != tt.wantStatus || got.Code != tt.wantCode || got.Updated != tt.wantUpdated {
				t.Errorf("updaterJobResult() = %+v, want status=%s code=%s updated=%d", got, tt.wantStatus, tt.wantCode, tt.wantUpdated)
			}

			if tt.wantReason != "" && got.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", got.Reason, tt.wantReason)
			}

			if tt.wantStatus != event.StatusSuccess && got.Reason == "" {
				t.Error("failed / skipped の Reason が空です")
			}
		})
	}
}

func TestRepoJobResult(t *testing.T) {
	tests := []struct {
		name       string
		result     *repomgr.UpdateResult
		err        error
		wantStatus event.Status
		wantCode   event.Code
	}{
		{name: "成功", result: &repomgr.UpdateResult{}, wantStatus: event.StatusSuccess},
		{name: "result が nil でも成功", wantStatus: event.StatusSuccess},
		{name: "警告だけなら成功（警告は別イベント）", result: &repomgr.UpdateResult{Warnings: []string{"BEHIND が残っています"}}, wantStatus: event.StatusSuccess},
		{name: "pull を実行しなかった", result: &repomgr.UpdateResult{SkippedMessages: []string{"upstream なし"}}, wantStatus: event.StatusSkipped, wantCode: event.CodePullSkipped},
		{name: "失敗", err: errors.New("fetch に失敗"), wantStatus: event.StatusFailed, wantCode: event.CodeError},
		{name: "キャンセル", err: context.Canceled, wantStatus: event.StatusSkipped, wantCode: event.CodeCancelled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repoJobResult(tt.result, tt.err)
			if got.Status != tt.wantStatus || got.Code != tt.wantCode {
				t.Errorf("repoJobResult() = %+v, want status=%s code=%s", got, tt.wantStatus, tt.wantCode)
			}
		})
	}
}

func TestRecordEnableWarning(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantSkipped  []string
		wantWarnings map[string]string // subject → category
	}{
		{
			name:        "利用不可のマネージャは skipped（unavailable）",
			err:         &updater.EnableWarning{Unavailable: []string{"gem", "snap"}},
			wantSkipped: []string{"gem", "snap"},
		},
		{
			name:         "未知のマネージャは警告",
			err:          &updater.EnableWarning{NotFound: []string{"foo"}},
			wantWarnings: map[string]string{"foo": event.CategoryManagerUnknown},
		},
		{
			name:         "両方が混在",
			err:          &updater.EnableWarning{Unavailable: []string{"gem"}, NotFound: []string{"foo"}},
			wantSkipped:  []string{"gem"},
			wantWarnings: map[string]string{"foo": event.CategoryManagerUnknown},
		},
		{
			name:         "設定の適用失敗は manager_config の警告",
			err:          errors.New("apt の設定適用に失敗: x"),
			wantWarnings: map[string]string{"": event.CategoryManagerConfig},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := useBufferedEvents(t, "sys")

			recordEnableWarning(tt.err)

			events := parseEventLines(t, buf.Bytes())

			skipped := map[string]bool{}
			warnings := map[string]string{}

			for _, ev := range events {
				switch ev["type"] {
				case "job_finished":
					if ev["status"] != "skipped" || ev["code"] != "unavailable" {
						t.Errorf("job_finished = %v, want skipped / unavailable", ev)
					}

					skipped[evString(ev, "name")] = true
				case "warning":
					warnings[evString(ev, "subject")] = evString(ev, "category")
				}
			}

			if len(skipped) != len(tt.wantSkipped) {
				t.Errorf("skipped = %v, want %v", skipped, tt.wantSkipped)
			}

			for _, name := range tt.wantSkipped {
				if !skipped[name] {
					t.Errorf("%s が skipped として記録されていません", name)
				}
			}

			if len(warnings) != len(tt.wantWarnings) {
				t.Errorf("warnings = %v, want %v", warnings, tt.wantWarnings)
			}

			for subject, category := range tt.wantWarnings {
				if warnings[subject] != category {
					t.Errorf("警告 %q の category = %q, want %q", subject, warnings[subject], category)
				}
			}
		})
	}
}

// TestExecuteUpdates_RecordsEvents は、並列・逐次の両方で、成功・失敗・キャンセルが漏れなくイベントに載ることを確認します。
func TestExecuteUpdates_RecordsEvents(t *testing.T) {
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name         string
		run          func(updaters []updater.Updater)
		ctx          context.Context
		updaters     []updater.Updater
		wantStatuses map[string]string
	}{
		{
			name:     "逐次: 成功と失敗",
			updaters: []updater.Updater{stubUpdater{name: "go"}, stubUpdater{name: "npm", updateErr: errors.New("boom")}},
			run: func(u []updater.Updater) {
				executeUpdates(context.Background(), u, updater.UpdateOptions{})
			},
			wantStatuses: map[string]string{"go": "success", "npm": "failed"},
		},
		{
			name:     "並列: 成功と失敗",
			updaters: []updater.Updater{stubUpdater{name: "go"}, stubUpdater{name: "npm", updateErr: errors.New("boom")}},
			run: func(u []updater.Updater) {
				executeUpdatesParallel(context.Background(), u, updater.UpdateOptions{}, 2, false)
			},
			wantStatuses: map[string]string{"go": "success", "npm": "failed"},
		},
		{
			name:     "逐次: キャンセル済みなら全件が skipped",
			updaters: []updater.Updater{stubUpdater{name: "go"}, stubUpdater{name: "npm"}},
			run: func(u []updater.Updater) {
				executeUpdates(canceledCtx, u, updater.UpdateOptions{})
			},
			wantStatuses: map[string]string{"go": "skipped", "npm": "skipped"},
		},
		{
			name:     "並列: キャンセル済みなら全件が skipped",
			updaters: []updater.Updater{stubUpdater{name: "go"}, stubUpdater{name: "npm"}},
			run: func(u []updater.Updater) {
				executeUpdatesParallel(canceledCtx, u, updater.UpdateOptions{}, 2, false)
			},
			wantStatuses: map[string]string{"go": "skipped", "npm": "skipped"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := useBufferedEvents(t, "sys")

			captureStderr(t, func() {
				captureStdout(t, func() { tt.run(tt.updaters) })
			})

			jobs := finishedJobs(parseEventLines(t, buf.Bytes()))
			if len(jobs) != len(tt.wantStatuses) {
				t.Fatalf("job_finished = %d 件, want %d 件: %v", len(jobs), len(tt.wantStatuses), jobs)
			}

			for name, status := range tt.wantStatuses {
				got, ok := jobs[name]
				if !ok {
					t.Errorf("%s の job_finished がありません", name)

					continue
				}

				if got["status"] != status {
					t.Errorf("%s の status = %v, want %s", name, got["status"], status)
				}

				if status == "skipped" && got["code"] != "cancelled" {
					t.Errorf("%s の code = %v, want cancelled", name, got["code"])
				}
			}
		})
	}
}

func TestRecordRepoUpdate(t *testing.T) {
	buf := useBufferedEvents(t, "repo")

	recordRepoUpdate("ok", &repomgr.UpdateResult{}, nil)
	recordRepoUpdate("warn", &repomgr.UpdateResult{Warnings: []string{"BEHIND が残っています"}}, nil)
	recordRepoUpdate("skip", &repomgr.UpdateResult{SkippedMessages: []string{"upstream なし"}}, nil)
	recordRepoUpdate("fail", nil, errors.New("fetch に失敗"))
	skipUnstartedRepos([]runner.Job{{Name: "ok"}, {Name: "never"}})

	events := parseEventLines(t, buf.Bytes())
	jobs := finishedJobs(events)

	want := map[string]string{"ok": "success", "warn": "success", "skip": "skipped", "fail": "failed", "never": "skipped"}
	for name, status := range want {
		if jobs[name]["status"] != status {
			t.Errorf("%s の status = %v, want %s", name, jobs[name]["status"], status)
		}
	}

	if jobs["never"]["code"] != "cancelled" {
		t.Errorf("開始しなかったリポジトリの code = %v, want cancelled", jobs["never"]["code"])
	}

	var warnings int

	for _, ev := range events {
		if ev["type"] == "warning" {
			warnings++

			if ev["subject"] != "warn" || ev["category"] != event.CategoryRepoWarning {
				t.Errorf("警告 = %v, want subject=warn category=repo_warning", ev)
			}
		}
	}

	if warnings != 1 {
		t.Errorf("警告 = %d 件, want 1 件", warnings)
	}
}

// resetEventsFlags は --events-file のフラグ変数を、テスト終了時に初期値へ戻します。
// runDaily を直接呼ぶ他のテストは resetGlobalFlags を通らないため、残すと次のテストが失敗します。
func resetEventsFlags(t *testing.T) {
	t.Helper()

	t.Cleanup(func() {
		sysEventsFile = ""
		repoUpdateEventsFile = ""
		runEventsFile = ""
	})
}

func readSummary(t *testing.T, path string) recordedEvent {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("イベントファイルを読めません: %v", err)
	}

	events := parseEventLines(t, data)
	if len(events) < 2 {
		t.Fatalf("イベントが少なすぎます: %d 件", len(events))
	}

	if events[0]["type"] != "start" || events[0]["schema"] != float64(event.SchemaVersion) {
		t.Errorf("先頭のイベント = %v, want type=start schema=%d", events[0], event.SchemaVersion)
	}

	last := events[len(events)-1]
	if last["type"] != "summary" {
		t.Fatalf("最後のイベントの type = %v, want summary", last["type"])
	}

	return last
}

// TestSysUpdate_EventsFile_WarningOnlyExitsZero は、警告だけの実行が終了コード 0 で、要約に警告が載ることを確認します。
func TestSysUpdate_EventsFile_WarningOnlyExitsZero(t *testing.T) {
	resetEventsFlags(t)
	home := setupEmptyConfig(t)

	configPath := filepath.Join(home, ".config", "dsx", "config.yaml")
	if err := os.WriteFile(configPath, []byte("version: 1\nsys:\n  enable: [no-such-manager]\n"), 0o644); err != nil {
		t.Fatalf("設定の書き込みに失敗: %v", err)
	}

	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")

	_, _, err := executeRootCommand(t, "sys", "update", "--events-file", eventsPath)
	if err != nil {
		t.Fatalf("警告だけの sys update は終了コード 0 のはずが、エラー: %v", err)
	}

	if activeEvents != nil {
		t.Error("コマンド終了後も activeEvents が残っています")
	}

	summary := readSummary(t, eventsPath)

	if summary["ok"] != true {
		t.Errorf("ok = %v, want true", summary["ok"])
	}

	if summary["warnings"] != float64(1) {
		t.Errorf("warnings = %v, want 1", summary["warnings"])
	}

	items, ok := summary["warning_items"].([]any)
	if !ok || len(items) != 1 || evString(items[0], "category") != event.CategoryManagerUnknown {
		t.Errorf("warning_items = %v, want manager_unknown 1 件", summary["warning_items"])
	}
}

func TestRepoUpdate_EventsFile_EmptyRoot(t *testing.T) {
	resetEventsFlags(t)
	setupEmptyConfig(t)

	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")

	_, _, err := executeRootCommand(t, "repo", "update", "--root", t.TempDir(), "--events-file", eventsPath)
	if err != nil {
		t.Fatalf("対象なしの repo update はエラーにならないはずが: %v", err)
	}

	summary := readSummary(t, eventsPath)
	if summary["ok"] != true || summary["command"] != "repo update" {
		t.Errorf("summary = %v, want ok=true command=repo update", summary)
	}
}

// TestRun_EventsFile_SharesOneStream は、run が sys と repo で 1 つのファイル・1 つの要約を共有することを確認します。
func TestRun_EventsFile_SharesOneStream(t *testing.T) {
	resetEventsFlags(t)
	setupEmptyConfig(t)

	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")

	_, _, err := executeRootCommand(t, "run", "--root", t.TempDir(), "--events-file", eventsPath)
	if err != nil {
		t.Fatalf("run はエラーにならないはずが: %v", err)
	}

	data, readErr := os.ReadFile(eventsPath)
	if readErr != nil {
		t.Fatalf("イベントファイルを読めません: %v", readErr)
	}

	var starts, summaries int

	for _, ev := range parseEventLines(t, data) {
		switch ev["type"] {
		case "start":
			starts++

			if ev["command"] != "run" {
				t.Errorf("start.command = %v, want run", ev["command"])
			}
		case "summary":
			summaries++
		}
	}

	if starts != 1 || summaries != 1 {
		t.Errorf("start = %d 件 / summary = %d 件, want 各 1 件", starts, summaries)
	}
}

func TestBeginEvents(t *testing.T) {
	t.Run("パスが空なら何もしない", func(t *testing.T) {
		finish, err := beginEvents("sys update", "")
		if err != nil {
			t.Fatalf("beginEvents() = %v", err)
		}

		if activeEvents != nil {
			t.Error("パスが空なのに activeEvents が設定されました")
		}

		finish(nil)
	})

	t.Run("作成できないパスはエラー（実行前に失敗させる）", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "no-such-dir", "events.jsonl")

		if _, err := beginEvents("sys update", missing); err == nil {
			t.Fatal("beginEvents() = nil, want エラー")
		} else if !strings.Contains(err.Error(), "イベントファイルを作成できません") {
			t.Errorf("エラーメッセージ = %v", err)
		}

		if activeEvents != nil {
			t.Error("失敗したのに activeEvents が設定されました")
		}
	})

	t.Run("上位が開始済みなら入れ子の開始は何もしない", func(t *testing.T) {
		buf := useBufferedEvents(t, "")
		outer := activeEvents

		finish, err := beginEvents("sys update", filepath.Join(t.TempDir(), "nested.jsonl"))
		if err != nil {
			t.Fatalf("beginEvents() = %v", err)
		}

		finish(errors.New("x"))

		if activeEvents != outer {
			t.Error("入れ子の finish が上位の activeEvents を壊しました")
		}

		if strings.Contains(buf.String(), `"type":"summary"`) {
			t.Error("入れ子の finish が要約を書きました")
		}
	})
}

// TestRun_EventsFile_PhaseFailureKeepsCause は、run のフェーズ開始前の失敗（root が存在しない等）の
// 具体的な原因が、件数だけのエラーに丸められず、summary の issues に残ることを確認します。
func TestRun_EventsFile_PhaseFailureKeepsCause(t *testing.T) {
	resetEventsFlags(t)
	setupEmptyConfig(t)

	missingRoot := filepath.Join(t.TempDir(), "no-such-root")
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")

	_, _, err := executeRootCommand(t, "run", "--root", missingRoot, "--events-file", eventsPath)
	if err == nil {
		t.Fatal("存在しない root の run はエラーになるはずが nil")
	}

	summary := readSummary(t, eventsPath)
	if summary["ok"] != false {
		t.Errorf("ok = %v, want false", summary["ok"])
	}

	issues, _ := summary["issues"].([]any)
	if len(issues) != 1 {
		t.Fatalf("issues = %v, want 1 件", summary["issues"])
	}

	if evString(issues[0], "kind") != "phase" || evString(issues[0], "name") != "repo" || evString(issues[0], "status") != "failed" || evString(issues[0], "phase") != "repo" {
		t.Errorf("issue = %v, want phase=repo kind=phase name=repo status=failed", issues[0])
	}

	reason := evString(issues[0], "reason")
	if !strings.Contains(reason, missingRoot) || strings.Contains(reason, "件のフェーズ") {
		t.Errorf("reason = %q, 具体的な原因（root のパス）を含み、件数だけのメッセージではないはず", reason)
	}
}
