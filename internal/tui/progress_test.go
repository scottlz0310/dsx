package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/scottlz0310/devsync/internal/runner"
)

func TestModelApplyEvent(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name              string
		events            []runner.Event
		wantState         jobState
		wantLogContains   string
		wantErrorContains string
	}{
		{
			name: "成功終了",
			events: []runner.Event{
				{Type: runner.EventStarted, JobIndex: 0, JobName: "job-1", Timestamp: time.Now()},
				{Type: runner.EventFinished, JobIndex: 0, JobName: "job-1", Status: runner.StatusSuccess, Duration: 200 * time.Millisecond, Timestamp: time.Now()},
			},
			wantState:       jobSuccess,
			wantLogContains: "完了: job-1",
		},
		{
			name: "失敗終了",
			events: []runner.Event{
				{Type: runner.EventStarted, JobIndex: 0, JobName: "job-1", Timestamp: time.Now()},
				{Type: runner.EventFinished, JobIndex: 0, JobName: "job-1", Status: runner.StatusFailed, Err: errors.New("boom"), Duration: 100 * time.Millisecond, Timestamp: time.Now()},
			},
			wantState:         jobFailed,
			wantLogContains:   "失敗: job-1",
			wantErrorContains: "boom",
		},
		{
			name: "スキップ終了",
			events: []runner.Event{
				{Type: runner.EventFinished, JobIndex: 0, JobName: "job-1", Status: runner.StatusSkipped, Err: context.Canceled, Duration: 50 * time.Millisecond, Timestamp: time.Now()},
			},
			wantState:         jobSkipped,
			wantLogContains:   "スキップ: job-1",
			wantErrorContains: "context canceled",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := newModel("test", []runner.Job{{Name: "job-1"}})
			for _, event := range tc.events {
				event := event
				m.applyEvent(&event)
			}

			if len(m.jobs) != 1 {
				t.Fatalf("job count = %d, want 1", len(m.jobs))
			}

			if m.jobs[0].State != tc.wantState {
				t.Fatalf("state = %s, want %s", m.jobs[0].State, tc.wantState)
			}

			if tc.wantErrorContains != "" && !strings.Contains(m.jobs[0].Err, tc.wantErrorContains) {
				t.Fatalf("err = %q, want contains %q", m.jobs[0].Err, tc.wantErrorContains)
			}

			if tc.wantLogContains != "" && !containsLog(m.logs, tc.wantLogContains) {
				t.Fatalf("logs = %+v, want contains %q", m.logs, tc.wantLogContains)
			}
		})
	}
}

func TestRenderBar(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		percent float64
		want    string
	}{
		{
			name:    "0未満は0扱い",
			percent: -1,
			want:    "[----------]",
		},
		{
			name:    "0.5は半分",
			percent: 0.5,
			want:    "[=====-----]",
		},
		{
			name:    "1超えは最大",
			percent: 2,
			want:    "[==========]",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := renderBar(tc.percent, 10)
			if got != tc.want {
				t.Fatalf("renderBar(%v, 10) = %q, want %q", tc.percent, got, tc.want)
			}
		})
	}
}

func TestProgressPercent(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		state jobState
		frame int
		min   float64
		max   float64
	}{
		{
			name:  "待機は0",
			state: jobPending,
			frame: 0,
			min:   0,
			max:   0,
		},
		{
			name:  "実行中は0.2-0.7",
			state: jobRunning,
			frame: 3,
			min:   0.2,
			max:   0.7,
		},
		{
			name:  "完了は1",
			state: jobSuccess,
			frame: 0,
			min:   1,
			max:   1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := progressPercent(tc.state, tc.frame)
			if got < tc.min || got > tc.max {
				t.Fatalf("progressPercent(%s, %d) = %v, want between %v and %v", tc.state, tc.frame, got, tc.min, tc.max)
			}
		})
	}
}

func TestResolveJobIndex(t *testing.T) {
	t.Parallel()

	m := newModel("test", []runner.Job{
		{Name: "dup"},
		{Name: "dup"},
		{Name: "uniq"},
	})

	testCases := []struct {
		name     string
		fallback int
		jobName  string
		want     int
	}{
		{
			name:     "有効なJobIndexを優先",
			fallback: 1,
			jobName:  "dup",
			want:     1,
		},
		{
			name:     "無効なJobIndexなら名前で解決",
			fallback: -1,
			jobName:  "uniq",
			want:     2,
		},
		{
			name:     "重複名は先頭indexへフォールバック",
			fallback: -1,
			jobName:  "dup",
			want:     0,
		},
		{
			name:     "解決不能ならfallbackを返す",
			fallback: 99,
			jobName:  "missing",
			want:     99,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := m.resolveJobIndex(tc.fallback, tc.jobName)
			if got != tc.want {
				t.Fatalf("resolveJobIndex(%d, %q) = %d, want %d", tc.fallback, tc.jobName, got, tc.want)
			}
		})
	}
}

func TestAppendLog_Capped(t *testing.T) {
	t.Parallel()

	m := newModel("test", []runner.Job{{Name: "job-1"}})

	total := maxBufferedLogs + 25
	for i := 0; i < total; i++ {
		m.appendLog(logInfo, "line")
	}

	if len(m.logs) != maxBufferedLogs {
		t.Fatalf("log length = %d, want %d", len(m.logs), maxBufferedLogs)
	}
}

func TestApplyEvent_WithDuplicateNamesUsesJobIndex(t *testing.T) {
	t.Parallel()

	m := newModel("test", []runner.Job{
		{Name: "dup"},
		{Name: "dup"},
	})

	event := runner.Event{
		Type:      runner.EventFinished,
		JobIndex:  1,
		JobName:   "dup",
		Status:    runner.StatusSuccess,
		Duration:  100 * time.Millisecond,
		Timestamp: time.Now(),
	}

	m.applyEvent(&event)

	if m.jobs[0].State == jobSuccess {
		t.Fatalf("jobs[0] should not be updated by duplicated name event")
	}

	if m.jobs[1].State != jobSuccess {
		t.Fatalf("jobs[1].State = %s, want %s", m.jobs[1].State, jobSuccess)
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		maxChars int
		want     string
	}{
		{"空文字列", "", 10, ""},
		{"maxChars=0", "hello", 0, ""},
		{"maxChars=負", "hello", -1, ""},
		{"maxChars=1", "hello", 1, "…"},
		{"maxChars=2", "hello", 2, "h…"},
		{"丁度の長さ", "hello", 5, "hello"},
		{"切り詰め不要", "hi", 5, "hi"},
		{"切り詰め発生", "hello world", 5, "hell…"},
		{"日本語文字列", "こんにちは世界", 4, "こんに…"},
		{"絵文字", "🍣🍺🎉🎊🎋", 3, "🍣🍺…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := truncate(tt.input, tt.maxChars)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxChars, got, tt.want)
			}
		})
	}
}

func TestSummarizeStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                                     string
		jobs                                     []jobProgress
		wantSuccess, wantFail, wantSkip, wantRun int
	}{
		{"空スライス", nil, 0, 0, 0, 0},
		{"全成功", []jobProgress{{State: jobSuccess}, {State: jobSuccess}}, 2, 0, 0, 0},
		{"全失敗", []jobProgress{{State: jobFailed}, {State: jobFailed}}, 0, 2, 0, 0},
		{"混在", []jobProgress{
			{State: jobSuccess},
			{State: jobFailed},
			{State: jobSkipped},
			{State: jobRunning},
			{State: jobPending},
		}, 1, 1, 1, 1},
		{"pendingはカウントされない", []jobProgress{{State: jobPending}}, 0, 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, f, sk, r := summarizeStates(tt.jobs)
			if s != tt.wantSuccess || f != tt.wantFail || sk != tt.wantSkip || r != tt.wantRun {
				t.Errorf("summarizeStates() = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
					s, f, sk, r, tt.wantSuccess, tt.wantFail, tt.wantSkip, tt.wantRun)
			}
		})
	}
}

func TestTailLogs(t *testing.T) {
	t.Parallel()

	entry := func(msg string) logEntry {
		return logEntry{Level: logInfo, Message: msg}
	}

	tests := []struct {
		name      string
		logs      []logEntry
		maxLines  int
		wantLen   int
		wantFirst string
	}{
		{"空ログ", nil, 5, 0, ""},
		{"上限以下", []logEntry{entry("a"), entry("b")}, 5, 2, "a"},
		{"丁度", []logEntry{entry("a"), entry("b"), entry("c")}, 3, 3, "a"},
		{"上限超過", []logEntry{entry("a"), entry("b"), entry("c"), entry("d")}, 2, 2, "c"},
		{"maxLines=0", []logEntry{entry("a")}, 0, 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tailLogs(tt.logs, tt.maxLines)
			if len(got) != tt.wantLen {
				t.Errorf("tailLogs() len = %d, want %d", len(got), tt.wantLen)
			}

			if tt.wantFirst != "" && len(got) > 0 && got[0].Message != tt.wantFirst {
				t.Errorf("tailLogs()[0].Message = %q, want %q", got[0].Message, tt.wantFirst)
			}
		})
	}
}

func TestRenderDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		duration time.Duration
		wantSub  string
	}{
		{"ゼロは'-'", 0, "-"},
		{"負数は'-'", -1 * time.Second, "-"},
		{"正の値は文字列化", 1500 * time.Millisecond, "1.5s"},
		{"ミリ秒精度", 123456 * time.Microsecond, "123ms"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := renderDuration(tt.duration)
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("renderDuration(%v) = %q, want contains %q", tt.duration, got, tt.wantSub)
			}
		})
	}
}

func TestRenderLog(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 15, 14, 30, 45, 0, time.UTC)

	tests := []struct {
		name    string
		entry   logEntry
		wantSub string
	}{
		{"infoレベル", logEntry{At: now, Level: logInfo, Message: "テスト"}, "テスト"},
		{"warnレベル", logEntry{At: now, Level: logWarn, Message: "警告"}, "警告"},
		{"errorレベル", logEntry{At: now, Level: logError, Message: "エラー"}, "エラー"},
		{"不明レベル", logEntry{At: now, Level: "unknown", Message: "msg"}, "msg"},
		{"タイムスタンプ含む", logEntry{At: now, Level: logInfo, Message: "msg"}, "14:30:45"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := renderLog(tt.entry)
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("renderLog() = %q, want contains %q", got, tt.wantSub)
			}
		})
	}
}

func TestRenderStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		job     *jobProgress
		wantSub string
	}{
		{"待機中", &jobProgress{State: jobPending}, "待機中"},
		{"実行中", &jobProgress{State: jobRunning}, "実行中"},
		{"成功", &jobProgress{State: jobSuccess}, "成功"},
		{"スキップ", &jobProgress{State: jobSkipped}, "スキップ"},
		{"失敗（エラーなし）", &jobProgress{State: jobFailed}, "失敗"},
		{"失敗（エラーあり）", &jobProgress{State: jobFailed, Err: "something broke"}, "something broke"},
		{"失敗（長いエラー）", &jobProgress{State: jobFailed, Err: strings.Repeat("x", 50)}, "…"},
		{"不明状態", &jobProgress{State: "unknown"}, "不明"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := renderStatus(tt.job)
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("renderStatus() = %q, want contains %q", got, tt.wantSub)
			}
		})
	}
}

func TestProgressPercent_AllStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state jobState
		frame int
		want  float64
	}{
		{"失敗は1", jobFailed, 0, 1},
		{"スキップは1", jobSkipped, 0, 1},
		{"不明stateは0", "unknown", 0, 0},
		{"実行中frame=0", jobRunning, 0, 0.2},
		{"実行中frame=5", jobRunning, 5, 0.7},
		{"実行中frame=6はラップして0.2", jobRunning, 6, 0.2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := progressPercent(tt.state, tt.frame)
			if got != tt.want {
				t.Errorf("progressPercent(%s, %d) = %v, want %v", tt.state, tt.frame, got, tt.want)
			}
		})
	}
}

func containsLog(logs []logEntry, needle string) bool {
	for _, log := range logs {
		if strings.Contains(log.Message, needle) {
			return true
		}
	}

	return false
}
