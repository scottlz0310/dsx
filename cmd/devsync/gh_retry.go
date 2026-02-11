package main

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// GitHub CLI(gh) の呼び出しは、repo cleanup の squashed 判定などで大量に発生し得る。
// GitHub の secondary rate limit などにより 429/403 が返ると一時的に失敗するため、
// ここで最小限のリトライとスロットリングを行う。

const (
	ghRetryMaxAttempts            = 6
	ghRetryBaseDelay              = 2 * time.Second
	ghRetryMaxDelay               = 60 * time.Second
	ghRetryMaxDelayFromRetryAfter = 5 * time.Minute

	// gh への同時アクセスを抑えて secondary rate limit を避ける（repo cleanup の並列数とは独立）。
	ghConcurrencyLimit = 1
)

var (
	ghLimiter = make(chan struct{}, ghConcurrencyLimit)

	ghSleepStep = sleepWithContext

	reRetryAfterSeconds = regexp.MustCompile(`(?i)retry[- ]after[: ]+(\d+)`)
)

func acquireGHLimiter(ctx context.Context) (func(), error) {
	select {
	case ghLimiter <- struct{}{}:
		return func() { <-ghLimiter }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func runGhOutputWithRetry(ctx context.Context, dir string, args ...string) (output []byte, stderr string, err error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var lastErr error

	for attempt := 1; attempt <= ghRetryMaxAttempts; attempt++ {
		output, stderr, err = runGhOutputOnce(ctx, dir, args...)
		lastErr = err

		if err == nil {
			return output, stderr, nil
		}

		// ctx が死んでいる場合は即終了（リトライしない）
		if ctx.Err() != nil {
			return nil, stderr, ctx.Err()
		}

		if !isRetryableGhError(stderr) {
			return nil, stderr, err
		}

		if attempt == ghRetryMaxAttempts {
			break
		}

		delay := calcGhRetryDelay(attempt, stderr)
		if sleepErr := ghSleepStep(ctx, delay); sleepErr != nil {
			return nil, stderr, sleepErr
		}
	}

	return nil, stderr, fmt.Errorf("gh のリトライ回数が上限に達しました: %w", lastErr)
}

func runGhOutputOnce(ctx context.Context, dir string, args ...string) (output []byte, stderr string, err error) {
	release, err := acquireGHLimiter(ctx)
	if err != nil {
		return nil, "", err
	}
	defer release()

	cmd := repoExecCommandStep(ctx, "gh", args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}

	var stderrBuf bytes.Buffer

	cmd.Stderr = &stderrBuf

	output, err = cmd.Output()

	return output, strings.TrimSpace(stderrBuf.String()), err
}

func isRetryableGhError(stderr string) bool {
	msg := strings.ToLower(strings.TrimSpace(stderr))
	if msg == "" {
		return false
	}

	// GitHub の rate limit / secondary rate limit
	if strings.Contains(msg, "too many requests") || strings.Contains(msg, "429") {
		return true
	}

	if strings.Contains(msg, "rate limit") || strings.Contains(msg, "secondary rate limit") {
		return true
	}

	// 一時的な障害（gh 内部で HTTP ステータスが見えることがある）
	if strings.Contains(msg, "502") || strings.Contains(msg, "503") || strings.Contains(msg, "504") {
		return true
	}

	if strings.Contains(msg, "bad gateway") || strings.Contains(msg, "service unavailable") || strings.Contains(msg, "gateway timeout") {
		return true
	}

	return false
}

func calcGhRetryDelay(attempt int, stderr string) time.Duration {
	if d, ok := parseRetryAfter(stderr); ok {
		// Retry-After は「最低待機時間」なので少し余裕を持たせる。
		// ただし無制限に待たないよう、最大値は別で制限する。
		return clampDuration(d+time.Second, ghRetryBaseDelay, ghRetryMaxDelayFromRetryAfter)
	}

	delay := ghRetryBaseDelay * time.Duration(1<<(attempt-1))

	return clampDuration(delay, ghRetryBaseDelay, ghRetryMaxDelay)
}

func parseRetryAfter(stderr string) (time.Duration, bool) {
	m := reRetryAfterSeconds.FindStringSubmatch(stderr)
	if len(m) != 2 {
		return 0, false
	}

	secs := strings.TrimSpace(m[1])
	if secs == "" {
		return 0, false
	}

	parsed, err := time.ParseDuration(secs + "s")
	if err != nil {
		return 0, false
	}

	if parsed <= 0 {
		return 0, false
	}

	return parsed, true
}

func clampDuration(d, minDur, maxDur time.Duration) time.Duration {
	if d < minDur {
		return minDur
	}

	if d > maxDur {
		return maxDur
	}

	return d
}

func isGitHubRateLimitError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	if msg == "" {
		return false
	}

	return strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "429") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "secondary rate limit")
}
