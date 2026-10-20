package event

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeClock は呼び出しごとではなく、advance で明示的に進む時計です。
type fakeClock struct{ t time.Time }

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 10, 21, 1, 2, 3, 0, time.UTC)}
}

func (c *fakeClock) now() time.Time { return c.t }

func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func splitLines(t *testing.T, buf *bytes.Buffer) []string {
	t.Helper()

	trimmed := strings.TrimRight(buf.String(), "\n")
	if trimmed == "" {
		return nil
	}

	return strings.Split(trimmed, "\n")
}

// TestStream_Golden はスキーマ（フィールド名・並び・値の形）を固定します。
// Arbor との契約なので、変更するときは SchemaVersion を上げ、Arbor 側と突き合わせてください。
func TestStream_Golden(t *testing.T) {
	clock := newFakeClock()

	var buf bytes.Buffer

	s := New(&buf, "run", "v0.13.0", clock.now)

	s.SetPhase("sys")
	s.Warn(CategoryManagerUnknown, "foo", "未知のマネージャが指定されています: foo")
	s.JobFinished(KindManager, "gem", JobResult{
		Status: StatusSkipped, Code: CodeUnavailable, Reason: "未インストールまたは利用不可",
	})
	s.JobStarted(KindManager, "go")
	clock.advance(1500 * time.Millisecond)
	s.JobFinished(KindManager, "go", JobResult{Status: StatusSuccess, Updated: 3})

	s.SetPhase("repo")
	s.JobStarted(KindRepo, "dsx")
	clock.advance(250 * time.Millisecond)
	s.JobFinished(KindRepo, "dsx", JobResult{Status: StatusFailed, Code: CodeError, Reason: "fetch に失敗"})

	if err := s.Finish(errors.New("1 件のリポジトリ更新に失敗しました")); err != nil {
		t.Fatalf("Finish() = %v, want nil", err)
	}

	want := []string{
		`{"schema":1,"type":"start","time":"2026-10-21T01:02:03Z","command":"run","version":"v0.13.0"}`,
		`{"schema":1,"type":"warning","time":"2026-10-21T01:02:03Z","phase":"sys","category":"manager_unknown","subject":"foo","message":"未知のマネージャが指定されています: foo"}`,
		`{"schema":1,"type":"job_finished","time":"2026-10-21T01:02:03Z","phase":"sys","kind":"manager","name":"gem","status":"skipped","code":"unavailable","reason":"未インストールまたは利用不可","updated":0,"duration_ms":0}`,
		`{"schema":1,"type":"job_started","time":"2026-10-21T01:02:03Z","phase":"sys","kind":"manager","name":"go"}`,
		`{"schema":1,"type":"job_finished","time":"2026-10-21T01:02:04.5Z","phase":"sys","kind":"manager","name":"go","status":"success","updated":3,"duration_ms":1500}`,
		`{"schema":1,"type":"job_started","time":"2026-10-21T01:02:04.5Z","phase":"repo","kind":"repo","name":"dsx"}`,
		`{"schema":1,"type":"job_finished","time":"2026-10-21T01:02:04.75Z","phase":"repo","kind":"repo","name":"dsx","status":"failed","code":"error","reason":"fetch に失敗","updated":0,"duration_ms":250}`,
		`{"schema":1,"type":"summary","time":"2026-10-21T01:02:04.75Z","command":"run","ok":false,"error":"1 件のリポジトリ更新に失敗しました","jobs":{"success":1,"failed":1,"skipped":1},"updated":3,"warnings":1,` +
			`"issues":[{"phase":"sys","kind":"manager","name":"gem","status":"skipped","code":"unavailable","reason":"未インストールまたは利用不可"},` +
			`{"phase":"repo","kind":"repo","name":"dsx","status":"failed","code":"error","reason":"fetch に失敗"}],` +
			`"warning_items":[{"phase":"sys","category":"manager_unknown","subject":"foo","message":"未知のマネージャが指定されています: foo"}]}`,
	}

	got := splitLines(t, &buf)
	if len(got) != len(want) {
		t.Fatalf("イベント数 = %d, want %d\n%s", len(got), len(want), buf.String())
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("行 %d が一致しません\n got: %s\nwant: %s", i+1, got[i], want[i])
		}
	}
}

func TestStream_SummaryCounts(t *testing.T) {
	tests := []struct {
		name        string
		run         func(s *Stream)
		runErr      error
		wantOK      bool
		wantJobs    JobCounts
		wantUpdated int
		wantIssues  int
		wantWarns   int
	}{
		{
			name:     "何も実行しない場合は空の要約になる",
			run:      func(*Stream) {},
			wantOK:   true,
			wantJobs: JobCounts{},
		},
		{
			name: "成功だけなら issues は空で警告もない",
			run: func(s *Stream) {
				s.JobFinished(KindManager, "go", JobResult{Status: StatusSuccess, Updated: 2})
				s.JobFinished(KindManager, "npm", JobResult{Status: StatusSuccess, Updated: 1})
			},
			wantOK:      true,
			wantJobs:    JobCounts{Success: 2},
			wantUpdated: 3,
		},
		{
			name: "警告とスキップだけでも ok（終了コード 0 の対象）",
			run: func(s *Stream) {
				s.Warn(CategoryRepoWarning, "a", "BEHIND が残っています")
				s.JobFinished(KindRepo, "b", JobResult{Status: StatusSkipped, Code: CodePullSkipped, Reason: "upstream なし"})
			},
			wantOK:     true,
			wantJobs:   JobCounts{Skipped: 1},
			wantIssues: 1,
			wantWarns:  1,
		},
		{
			name: "失敗があれば runErr が ok を false にする",
			run: func(s *Stream) {
				s.JobFinished(KindManager, "apt", JobResult{Status: StatusFailed, Code: CodeError, Reason: "exit 100"})
			},
			runErr:     errors.New("1 件のエラーが発生しました"),
			wantOK:     false,
			wantJobs:   JobCounts{Failed: 1},
			wantIssues: 1,
		},
		{
			name:   "失敗ジョブが無くても runErr（例: root 不正）は ok を false にする",
			run:    func(*Stream) {},
			runErr: errors.New("root が存在しません"),
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			s := New(&buf, "sys update", "dev", newFakeClock().now)
			tt.run(s)

			if err := s.Finish(tt.runErr); err != nil {
				t.Fatalf("Finish() = %v", err)
			}

			lines := splitLines(t, &buf)

			var got struct {
				Type     string        `json:"type"`
				OK       bool          `json:"ok"`
				Jobs     JobCounts     `json:"jobs"`
				Updated  int           `json:"updated"`
				Warnings int           `json:"warnings"`
				Issues   []Issue       `json:"issues"`
				Items    []WarningItem `json:"warning_items"`
			}

			if err := json.Unmarshal([]byte(lines[len(lines)-1]), &got); err != nil {
				t.Fatalf("summary の JSON 解析に失敗: %v", err)
			}

			if got.Type != "summary" {
				t.Fatalf("最終行の type = %q, want summary", got.Type)
			}

			if got.OK != tt.wantOK {
				t.Errorf("ok = %v, want %v", got.OK, tt.wantOK)
			}

			if got.Jobs != tt.wantJobs {
				t.Errorf("jobs = %+v, want %+v", got.Jobs, tt.wantJobs)
			}

			if got.Updated != tt.wantUpdated {
				t.Errorf("updated = %d, want %d", got.Updated, tt.wantUpdated)
			}

			if len(got.Issues) != tt.wantIssues {
				t.Errorf("issues = %d 件, want %d 件", len(got.Issues), tt.wantIssues)
			}

			if got.Warnings != tt.wantWarns || len(got.Items) != tt.wantWarns {
				t.Errorf("warnings = %d / warning_items = %d 件, want %d 件", got.Warnings, len(got.Items), tt.wantWarns)
			}
		})
	}
}

// TestStream_EmptyCollectionsAreArrays は issues / warning_items が null でなく [] になることを固定します
// （受け取る側が null と空配列を分けて扱わなくて済むように）。
func TestStream_EmptyCollectionsAreArrays(t *testing.T) {
	var buf bytes.Buffer

	s := New(&buf, "sys update", "dev", newFakeClock().now)
	if err := s.Finish(nil); err != nil {
		t.Fatalf("Finish() = %v", err)
	}

	last := splitLines(t, &buf)
	if !strings.Contains(last[len(last)-1], `"issues":[],"warning_items":[]`) {
		t.Errorf("空のコレクションが [] ではありません: %s", last[len(last)-1])
	}
}

func TestStream_SkipUnstarted(t *testing.T) {
	tests := []struct {
		name        string
		started     []string
		finished    []string
		all         []string
		wantSkipped []string
	}{
		{
			name:        "開始していないジョブだけが skipped になる",
			started:     []string{"a"},
			finished:    []string{"a"},
			all:         []string{"a", "b", "c"},
			wantSkipped: []string{"b", "c"},
		},
		{
			name:        "開始済みで未完了のジョブは skipped にしない（完了を待つ）",
			started:     []string{"a"},
			all:         []string{"a", "b"},
			wantSkipped: []string{"b"},
		},
		{
			name:        "すべて完了していれば何も追加しない",
			started:     []string{"a", "b"},
			finished:    []string{"a", "b"},
			all:         []string{"a", "b"},
			wantSkipped: nil,
		},
		{
			name:        "名前が空の一覧は何もしない",
			all:         nil,
			wantSkipped: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			s := New(&buf, "repo update", "dev", newFakeClock().now)
			s.SetPhase("repo")

			for _, name := range tt.started {
				s.JobStarted(KindRepo, name)
			}

			for _, name := range tt.finished {
				s.JobFinished(KindRepo, name, JobResult{Status: StatusSuccess})
			}

			s.SkipUnstarted(KindRepo, tt.all, CodeCancelled, "キャンセルまたはタイムアウト")

			var gotSkipped []string

			for _, line := range splitLines(t, &buf) {
				var ev struct {
					Type   string `json:"type"`
					Name   string `json:"name"`
					Status Status `json:"status"`
					Code   Code   `json:"code"`
				}
				if err := json.Unmarshal([]byte(line), &ev); err != nil {
					t.Fatalf("JSON 解析に失敗: %v", err)
				}

				if ev.Type == "job_finished" && ev.Status == StatusSkipped {
					if ev.Code != CodeCancelled {
						t.Errorf("%s の code = %q, want %q", ev.Name, ev.Code, CodeCancelled)
					}

					gotSkipped = append(gotSkipped, ev.Name)
				}
			}

			if strings.Join(gotSkipped, ",") != strings.Join(tt.wantSkipped, ",") {
				t.Errorf("skipped = %v, want %v", gotSkipped, tt.wantSkipped)
			}

			// 二重に呼んでも増えない。
			before := buf.Len()

			s.SkipUnstarted(KindRepo, tt.all, CodeCancelled, "キャンセルまたはタイムアウト")

			if buf.Len() != before {
				t.Errorf("SkipUnstarted を 2 回呼ぶとイベントが重複します")
			}
		})
	}
}

func TestNilStream_IsNoOp(t *testing.T) {
	var s *Stream

	s.SetPhase("sys")
	s.JobStarted(KindManager, "go")
	s.JobFinished(KindManager, "go", JobResult{Status: StatusSuccess})
	s.SkipUnstarted(KindManager, []string{"go"}, CodeCancelled, "")
	s.Warn(CategoryRepoWarning, "a", "b")

	if err := s.Finish(errors.New("x")); err != nil {
		t.Errorf("nil Stream の Finish() = %v, want nil", err)
	}
}

type failingWriter struct{ calls int }

func (w *failingWriter) Write([]byte) (int, error) {
	w.calls++

	return 0, errors.New("ディスクが一杯です")
}

// TestStream_WriteErrorIsReportedByFinish は、書き込み失敗が握りつぶされず Finish から返ることを確認します。
func TestStream_WriteErrorIsReportedByFinish(t *testing.T) {
	w := &failingWriter{}
	s := New(w, "sys update", "dev", newFakeClock().now)

	s.JobFinished(KindManager, "go", JobResult{Status: StatusSuccess})

	err := s.Finish(nil)
	if err == nil {
		t.Fatal("Finish() = nil, want 書き込みエラー")
	}

	if !strings.Contains(err.Error(), "ディスクが一杯です") {
		t.Errorf("元のエラーが含まれていません: %v", err)
	}

	if w.calls != 1 {
		t.Errorf("失敗後も書き込みを続けています（Write 呼び出し %d 回, want 1）", w.calls)
	}
}
