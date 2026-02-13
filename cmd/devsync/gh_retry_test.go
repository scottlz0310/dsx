package main

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
)

func TestRunGhOutputWithRetry_RateLimitThenSuccess(t *testing.T) {
	originalCommandStep := repoExecCommandStep
	originalSleepStep := ghSleepStep
	t.Cleanup(func() {
		repoExecCommandStep = originalCommandStep
		ghSleepStep = originalSleepStep
	})

	var calls int

	repoExecCommandStep = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		calls++

		if calls == 1 {
			return helperProcessCommand(ctx, "", "exceeded retry limit, last status: 429 Too Many Requests, request id: 50e58657-3180-4fd7-99f4-e0d005d07a9d\n", 1)
		}

		return helperProcessCommand(ctx, "[]\n", "", 0)
	}

	var slept []time.Duration

	ghSleepStep = func(ctx context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}

	got, stderr, err := runGhOutputWithRetry(context.Background(), "", "repo", "list", "owner", "--limit", "1", "--json", "name")
	if err != nil {
		t.Fatalf("runGhOutputWithRetry() error = %v", err)
	}

	if string(got) != "[]\n" {
		t.Fatalf("stdout = %q, want %q", string(got), "[]\n")
	}

	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}

	if len(slept) != 1 {
		t.Fatalf("slept len = %d, want 1. slept=%v", len(slept), slept)
	}
}

func TestRunGhOutputWithRetry_NonRetryableErrorDoesNotRetry(t *testing.T) {
	originalCommandStep := repoExecCommandStep
	originalSleepStep := ghSleepStep
	t.Cleanup(func() {
		repoExecCommandStep = originalCommandStep
		ghSleepStep = originalSleepStep
	})

	var calls int

	repoExecCommandStep = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		calls++
		return helperProcessCommand(ctx, "", "auth failed\n", 1)
	}

	var slept []time.Duration

	ghSleepStep = func(ctx context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}

	_, _, err := runGhOutputWithRetry(context.Background(), "", "repo", "list", "owner", "--limit", "1", "--json", "name")
	if err == nil {
		t.Fatalf("runGhOutputWithRetry() error = nil, want error")
	}

	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}

	if len(slept) != 0 {
		t.Fatalf("sleep should not be called. slept=%v", slept)
	}
}

func TestRunGhOutputWithRetry_ExhaustsAttempts(t *testing.T) {
	originalCommandStep := repoExecCommandStep
	originalSleepStep := ghSleepStep
	t.Cleanup(func() {
		repoExecCommandStep = originalCommandStep
		ghSleepStep = originalSleepStep
	})

	var calls int

	repoExecCommandStep = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		calls++
		return helperProcessCommand(ctx, "", "exceeded retry limit, last status: 429 Too Many Requests\n", 1)
	}

	var slept []time.Duration

	ghSleepStep = func(ctx context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}

	_, stderr, err := runGhOutputWithRetry(context.Background(), "", "repo", "list", "owner", "--limit", "1", "--json", "name")
	if err == nil {
		t.Fatalf("runGhOutputWithRetry() error = nil, want error")
	}

	if stderr == "" {
		t.Fatalf("stderr should not be empty")
	}

	if calls != ghRetryMaxAttempts {
		t.Fatalf("calls = %d, want %d", calls, ghRetryMaxAttempts)
	}

	if len(slept) != ghRetryMaxAttempts-1 {
		t.Fatalf("slept len = %d, want %d. slept=%v", len(slept), ghRetryMaxAttempts-1, slept)
	}
}

func TestCalcGhRetryDelay_ParsesRetryAfter(t *testing.T) {
	t.Parallel()

	got := calcGhRetryDelay(1, "Retry-After: 10")
	// parseRetryAfter() で 10s を見つけた場合は +1s して返す
	if got != 11*time.Second {
		t.Fatalf("calcGhRetryDelay() = %v, want %v", got, 11*time.Second)
	}
}

func TestIsRetryableGhError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		stderr string
		want   bool
	}{
		{
			name:   "429 Too Many Requests",
			stderr: "exceeded retry limit, last status: 429 Too Many Requests",
			want:   true,
		},
		{
			name:   "rate limit",
			stderr: "secondary rate limit",
			want:   true,
		},
		{
			name:   "一時的な障害",
			stderr: "502 Bad Gateway",
			want:   true,
		},
		{
			name:   "非リトライ対象",
			stderr: "auth failed",
			want:   false,
		},
		{
			name:   "空文字列",
			stderr: "",
			want:   false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isRetryableGhError(tt.stderr)
			if got != tt.want {
				t.Fatalf("isRetryableGhError(%q) = %v, want %v", tt.stderr, got, tt.want)
			}
		})
	}
}

func TestClampDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		d      time.Duration
		minDur time.Duration
		maxDur time.Duration
		want   time.Duration
	}{
		{
			name:   "最小値に丸める",
			d:      time.Second,
			minDur: 2 * time.Second,
			maxDur: 10 * time.Second,
			want:   2 * time.Second,
		},
		{
			name:   "最大値に丸める",
			d:      20 * time.Second,
			minDur: 2 * time.Second,
			maxDur: 10 * time.Second,
			want:   10 * time.Second,
		},
		{
			name:   "範囲内はそのまま返す",
			d:      5 * time.Second,
			minDur: 2 * time.Second,
			maxDur: 10 * time.Second,
			want:   5 * time.Second,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := clampDuration(tt.d, tt.minDur, tt.maxDur)
			if got != tt.want {
				t.Fatalf("clampDuration(%v, %v, %v) = %v, want %v", tt.d, tt.minDur, tt.maxDur, got, tt.want)
			}
		})
	}
}

func TestSleepWithContext(t *testing.T) {
	t.Parallel()

	t.Run("0以下は即時復帰", func(t *testing.T) {
		t.Parallel()

		if err := sleepWithContext(context.Background(), 0); err != nil {
			t.Fatalf("sleepWithContext() error = %v", err)
		}
	})

	t.Run("待機してnilを返す", func(t *testing.T) {
		t.Parallel()

		if err := sleepWithContext(context.Background(), time.Millisecond); err != nil {
			t.Fatalf("sleepWithContext() error = %v", err)
		}
	})

	t.Run("キャンセルはctx.Errを返す", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := sleepWithContext(ctx, 10*time.Second)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("sleepWithContext() error = %v, want %v", err, context.Canceled)
		}
	})
}

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		stderr  string
		wantDur time.Duration
		wantOK  bool
	}{
		{"秒数を正しくパース", "Retry-After: 30", 30 * time.Second, true},
		{"ハイフン区切り", "Retry-After: 5", 5 * time.Second, true},
		{"スペース区切り", "Retry After: 10", 10 * time.Second, true},
		{"ヘッダーなし", "some other error", 0, false},
		{"空文字列", "", 0, false},
		{"0秒", "Retry-After: 0", 0, false},
		{"メッセージ中に埋め込み", "error: rate limit exceeded. Retry-After: 60. please wait.", 60 * time.Second, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dur, ok := parseRetryAfter(tt.stderr)
			if ok != tt.wantOK {
				t.Fatalf("parseRetryAfter(%q) ok = %v, want %v", tt.stderr, ok, tt.wantOK)
			}

			if dur != tt.wantDur {
				t.Fatalf("parseRetryAfter(%q) dur = %v, want %v", tt.stderr, dur, tt.wantDur)
			}
		})
	}
}

func TestCalcGhRetryDelay_ExponentialBackoff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		attempt int
		stderr  string
		want    time.Duration
	}{
		{"attempt=1、RetryAfterなし", 1, "429 error", 2 * time.Second},
		{"attempt=2、RetryAfterなし", 2, "429 error", 4 * time.Second},
		{"attempt=3、RetryAfterなし", 3, "429 error", 8 * time.Second},
		{"attempt=4、RetryAfterなし", 4, "429 error", 16 * time.Second},
		{"attempt=5、RetryAfterなし", 5, "429 error", 32 * time.Second},
		{"attempt=6、最大値にクランプ", 6, "429 error", 60 * time.Second},
		{"RetryAfter=120は121秒", 1, "Retry-After: 120", 121 * time.Second},
		{"RetryAfter=1は最小値にクランプ", 1, "Retry-After: 1", 2 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := calcGhRetryDelay(tt.attempt, tt.stderr)
			if got != tt.want {
				t.Errorf("calcGhRetryDelay(%d, %q) = %v, want %v", tt.attempt, tt.stderr, got, tt.want)
			}
		})
	}
}

func TestIsGitHubRateLimitError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nilエラー", nil, false},
		{"too many requests", errors.New("too many requests"), true},
		{"429含む", errors.New("HTTP 429"), true},
		{"rate limit", errors.New("API rate limit exceeded"), true},
		{"secondary rate limit", errors.New("secondary rate limit"), true},
		{"無関係なエラー", errors.New("permission denied"), false},
		{"空メッセージ", errors.New(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isGitHubRateLimitError(tt.err)
			if got != tt.want {
				t.Errorf("isGitHubRateLimitError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsRetryableGhError_Extended(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		stderr string
		want   bool
	}{
		{"503 Service Unavailable", "503 Service Unavailable", true},
		{"504 Gateway Timeout", "504 Gateway Timeout", true},
		{"service unavailable テキスト", "service unavailable", true},
		{"gateway timeout テキスト", "gateway timeout", true},
		{"bad gateway テキスト", "bad gateway", true},
		{"大文字混在", "TOO MANY REQUESTS", true},
		{"空白のみ", "   ", false},
		{"permission denied", "permission denied", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isRetryableGhError(tt.stderr)
			if got != tt.want {
				t.Errorf("isRetryableGhError(%q) = %v, want %v", tt.stderr, got, tt.want)
			}
		})
	}
}
