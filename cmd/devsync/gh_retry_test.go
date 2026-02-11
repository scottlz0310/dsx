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
