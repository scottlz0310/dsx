package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	repomgr "github.com/scottlz0310/devsync/internal/repo"
	"github.com/scottlz0310/devsync/internal/runner"
)

func TestWantsCleanupTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		targets  []string
		want     string
		expected bool
	}{
		{
			name:     "空のtargets",
			targets:  nil,
			want:     "merged",
			expected: false,
		},
		{
			name:     "一致",
			targets:  []string{"merged"},
			want:     "merged",
			expected: true,
		},
		{
			name:     "大文字小文字と空白を無視",
			targets:  []string{"  SQUASHED  "},
			want:     "squashed",
			expected: true,
		},
		{
			name:     "含まれない",
			targets:  []string{"merged"},
			want:     "squashed",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := wantsCleanupTarget(tt.targets, tt.want)
			if got != tt.expected {
				t.Fatalf("wantsCleanupTarget(%#v, %q) = %v, want %v", tt.targets, tt.want, got, tt.expected)
			}
		})
	}
}

func TestListMergedPRHeads_GHMissing(t *testing.T) {
	originalLookPathStep := repoLookPathStep
	originalCommandStep := repoExecCommandStep
	t.Cleanup(func() {
		repoLookPathStep = originalLookPathStep
		repoExecCommandStep = originalCommandStep
	})

	repoPath := t.TempDir()

	repoLookPathStep = func(string) (string, error) {
		return "", errors.New("not found")
	}

	repoExecCommandStep = func(context.Context, string, ...string) *exec.Cmd {
		t.Fatalf("repoExecCommandStep should not be called when gh is missing")

		return nil
	}

	_, err := listMergedPRHeads(context.Background(), repoPath, "main")
	if err == nil {
		t.Fatalf("listMergedPRHeads() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "gh コマンドが見つかりません") {
		t.Fatalf("error should contain missing gh message: %v", err)
	}
}

func TestListMergedPRHeads_CommandFailureIncludesStderr(t *testing.T) {
	originalLookPathStep := repoLookPathStep
	originalCommandStep := repoExecCommandStep
	t.Cleanup(func() {
		repoLookPathStep = originalLookPathStep
		repoExecCommandStep = originalCommandStep
	})

	repoPath := t.TempDir()
	baseBranch := "main"

	repoLookPathStep = func(file string) (string, error) {
		if file != "gh" {
			t.Fatalf("repoLookPathStep file = %q, want gh", file)
		}

		return "/usr/bin/gh", nil
	}

	repoExecCommandStep = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		assertGHPullRequestListArgs(t, name, arg, baseBranch)

		return helperProcessCommand(ctx, "", "auth failed\n", 1)
	}

	_, err := listMergedPRHeads(context.Background(), repoPath, baseBranch)
	if err == nil {
		t.Fatalf("listMergedPRHeads() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "gh pr list の実行に失敗しました") {
		t.Fatalf("error should contain command failure message: %v", err)
	}

	if !strings.Contains(err.Error(), "auth failed") {
		t.Fatalf("error should contain stderr details: %v", err)
	}
}

func TestListMergedPRHeads_JSONParseFailure(t *testing.T) {
	originalLookPathStep := repoLookPathStep
	originalCommandStep := repoExecCommandStep
	t.Cleanup(func() {
		repoLookPathStep = originalLookPathStep
		repoExecCommandStep = originalCommandStep
	})

	repoPath := t.TempDir()
	baseBranch := "main"

	repoLookPathStep = func(string) (string, error) {
		return "/usr/bin/gh", nil
	}

	repoExecCommandStep = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		assertGHPullRequestListArgs(t, name, arg, baseBranch)

		return helperProcessCommand(ctx, "not json", "", 0)
	}

	_, err := listMergedPRHeads(context.Background(), repoPath, baseBranch)
	if err == nil {
		t.Fatalf("listMergedPRHeads() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "PR 一覧の解析に失敗") {
		t.Fatalf("error should contain json unmarshal message: %v", err)
	}
}

func TestListMergedPRHeads_SuccessReturnsLatest(t *testing.T) {
	originalLookPathStep := repoLookPathStep
	originalCommandStep := repoExecCommandStep
	t.Cleanup(func() {
		repoLookPathStep = originalLookPathStep
		repoExecCommandStep = originalCommandStep
	})

	repoPath := t.TempDir()
	baseBranch := "develop"

	repoLookPathStep = func(string) (string, error) {
		return "/usr/bin/gh", nil
	}

	repoExecCommandStep = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		assertGHPullRequestListArgs(t, name, arg, baseBranch)

		stdout := `[{"headRefName":" feature/a ","headRefOid":"111","mergedAt":"2026-02-09T00:00:00Z"},` +
			`{"headRefName":"feature/a","headRefOid":" 222 ","mergedAt":"2026-02-10T00:00:00Z"},` +
			`{"headRefName":"feature/b","headRefOid":"333","mergedAt":"2026-02-08T00:00:00Z"},` +
			`{"headRefName":"","headRefOid":"444","mergedAt":"2026-02-10T00:00:00Z"},` +
			`{"headRefName":"feature/c","headRefOid":"","mergedAt":"2026-02-10T00:00:00Z"}]`

		return helperProcessCommand(ctx, stdout+"\n", "", 0)
	}

	got, err := listMergedPRHeads(context.Background(), repoPath, baseBranch)
	if err != nil {
		t.Fatalf("listMergedPRHeads() unexpected error: %v", err)
	}

	want := mergedPRHeadsResult{
		Heads: map[string]string{
			"feature/a": "222",
			"feature/b": "333",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("listMergedPRHeads() = %#v, want %#v", got, want)
	}
}

func TestListMergedPRHeads_LimitWarning(t *testing.T) {
	originalLookPathStep := repoLookPathStep
	originalCommandStep := repoExecCommandStep
	t.Cleanup(func() {
		repoLookPathStep = originalLookPathStep
		repoExecCommandStep = originalCommandStep
	})

	repoPath := t.TempDir()
	baseBranch := "main"

	repoLookPathStep = func(string) (string, error) {
		return "/usr/bin/gh", nil
	}

	repoExecCommandStep = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		assertGHPullRequestListArgs(t, name, arg, baseBranch)

		var b strings.Builder
		b.WriteString("[")

		for i := range githubPullRequestListLimit {
			if i > 0 {
				b.WriteString(",")
			}

			b.WriteString(`{"headRefName":"feature/`)
			b.WriteString(strconv.Itoa(i))
			b.WriteString(`","headRefOid":"oid-`)
			b.WriteString(strconv.Itoa(i))
			b.WriteString(`","mergedAt":"2026-02-10T00:00:00Z"}`)
		}

		b.WriteString("]")

		return helperProcessCommand(ctx, b.String()+"\n", "", 0)
	}

	got, err := listMergedPRHeads(context.Background(), repoPath, baseBranch)
	if err != nil {
		t.Fatalf("listMergedPRHeads() unexpected error: %v", err)
	}

	if got.Warning == "" {
		t.Fatalf("Warning should not be empty")
	}

	if len(got.Heads) != githubPullRequestListLimit {
		t.Fatalf("Heads length = %d, want %d", len(got.Heads), githubPullRequestListLimit)
	}
}

func TestPrepareRepoCleanupOptions(t *testing.T) {
	t.Run("squashedを要求しない場合はそのまま返す", func(t *testing.T) {
		repoPath := t.TempDir()

		opts := repomgr.CleanupOptions{
			Prune:   true,
			DryRun:  true,
			Targets: []string{"merged"},
		}

		got, warnings := prepareRepoCleanupOptions(context.Background(), repoPath, opts)
		if !reflect.DeepEqual(got, opts) {
			t.Fatalf("prepareRepoCleanupOptions() = %#v, want %#v", got, opts)
		}

		if len(warnings) != 0 {
			t.Fatalf("warnings should be empty: %v", warnings)
		}
	})

	t.Run("デフォルトブランチ判定に失敗した場合は警告", func(t *testing.T) {
		repoPath := t.TempDir()

		opts := repomgr.CleanupOptions{
			Prune:   true,
			DryRun:  true,
			Targets: []string{"squashed"},
		}

		got, warnings := prepareRepoCleanupOptions(context.Background(), repoPath, opts)
		if !reflect.DeepEqual(got, opts) {
			t.Fatalf("prepareRepoCleanupOptions() = %#v, want %#v", got, opts)
		}

		if len(warnings) == 0 {
			t.Fatalf("warnings should not be empty")
		}

		if !strings.Contains(warnings[0], "squashed 判定の準備に失敗") {
			t.Fatalf("warning should contain squashed setup message: %v", warnings)
		}
	})
}

func TestRunRepoCleanupJob_AppendsWarnings(t *testing.T) {
	originalStep := repoCleanupStep
	t.Cleanup(func() {
		repoCleanupStep = originalStep
	})

	repoCleanupStep = func(context.Context, string, repomgr.CleanupOptions) (*repomgr.CleanupResult, error) {
		return &repomgr.CleanupResult{}, nil
	}

	repoPath := t.TempDir()

	got, err := runRepoCleanupJob(context.Background(), repoPath, repomgr.CleanupOptions{
		Targets: []string{"squashed"},
	})
	if err != nil {
		t.Fatalf("runRepoCleanupJob() error = %v", err)
	}

	if got == nil {
		t.Fatalf("runRepoCleanupJob() result is nil")
	}

	found := false
	for _, msg := range got.SkippedMessages {
		if strings.Contains(msg, "squashed 判定") {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("SkippedMessages should contain squashed warning: %v", got.SkippedMessages)
	}
}

func TestBuildRepoCleanupJobs_NameCollisionAndPrint(t *testing.T) {
	originalStep := repoCleanupStep
	t.Cleanup(func() {
		repoCleanupStep = originalStep
	})

	repoCleanupStep = func(context.Context, string, repomgr.CleanupOptions) (*repomgr.CleanupResult, error) {
		return &repomgr.CleanupResult{
			Commands: []string{
				"git -C dummy fetch --all",
			},
			PlannedDeletes: []repomgr.CleanupPlan{
				{Branch: "feature-x", Target: "merged"},
			},
			DeletedBranches: []repomgr.CleanupPlan{
				{Branch: "feature-y", Target: "merged"},
			},
			SkippedMessages: []string{
				"スキップ理由",
			},
			Errors: []error{
				errors.New("partial error"),
			},
		}, nil
	}

	root := t.TempDir()
	repoA := filepath.Join(t.TempDir(), "repo")
	repoB := filepath.Join(t.TempDir(), "repo")

	execJobs := buildRepoCleanupJobs(root, []string{repoA, repoB}, repomgr.CleanupOptions{
		Targets: []string{"merged"},
	}, false)

	if len(execJobs) != 2 {
		t.Fatalf("buildRepoCleanupJobs() len = %d, want 2", len(execJobs))
	}

	if execJobs[0].Name != filepath.Clean(repoA) {
		t.Fatalf("job[0].Name = %q, want %q", execJobs[0].Name, filepath.Clean(repoA))
	}

	if execJobs[1].Name != filepath.Clean(repoB) {
		t.Fatalf("job[1].Name = %q, want %q", execJobs[1].Name, filepath.Clean(repoB))
	}

	out := captureStdout(t, func() {
		for i, job := range execJobs {
			if err := job.Run(context.Background()); err != nil {
				t.Fatalf("job[%d].Run() error = %v", i, err)
			}
		}
	})

	if !strings.Contains(out, filepath.Clean(repoA)) {
		t.Fatalf("output should contain repoA name: %q", out)
	}

	if !strings.Contains(out, filepath.Clean(repoB)) {
		t.Fatalf("output should contain repoB name: %q", out)
	}

	if !strings.Contains(out, "📝 削除予定: feature-x") {
		t.Fatalf("output should contain planned delete: %q", out)
	}

	if !strings.Contains(out, "🗑️  削除: feature-y") {
		t.Fatalf("output should contain deleted branch: %q", out)
	}

	if !strings.Contains(out, "✅ 成功") {
		t.Fatalf("output should contain success: %q", out)
	}
}

func TestPrintRepoCleanupResult(t *testing.T) {
	t.Run("キャンセルはスキップ表示", func(t *testing.T) {
		out := captureStdout(t, func() {
			printRepoCleanupResult("repo", nil, context.Canceled)
		})

		if !strings.Contains(out, "⚪ スキップ") {
			t.Fatalf("output should contain skip: %q", out)
		}
	})

	t.Run("失敗は失敗表示", func(t *testing.T) {
		out := captureStdout(t, func() {
			printRepoCleanupResult("repo", nil, errors.New("boom"))
		})

		if !strings.Contains(out, "❌ 失敗") {
			t.Fatalf("output should contain failure: %q", out)
		}
	})
}

func TestPrintRepoCleanupSummary(t *testing.T) {
	out := captureStdout(t, func() {
		printRepoCleanupSummary(runner.Summary{
			Total:   3,
			Success: 2,
			Failed:  1,
			Skipped: 0,
		})
	})

	if !strings.Contains(out, "repo cleanup サマリー") {
		t.Fatalf("output should contain summary title: %q", out)
	}

	if !strings.Contains(out, "成功: 2 件") {
		t.Fatalf("output should contain success count: %q", out)
	}
}

func helperProcessCommand(ctx context.Context, stdout, stderr string, exitCode int) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess", "--")

	cmd.Env = append(os.Environ(),
		"GO_WANT_HELPER_PROCESS=1",
		"DEVSYNC_HELPER_STDOUT="+stdout,
		"DEVSYNC_HELPER_STDERR="+stderr,
		"DEVSYNC_HELPER_EXIT_CODE="+strconv.Itoa(exitCode),
	)

	return cmd
}

func assertGHPullRequestListArgs(t *testing.T, name string, gotArgs []string, baseBranch string) {
	t.Helper()

	if name != "gh" {
		t.Fatalf("repoExecCommandStep name = %q, want gh", name)
	}

	wantArgs := []string{
		"pr",
		"list",
		"--state",
		"merged",
		"--base",
		baseBranch,
		"--limit",
		strconv.Itoa(githubPullRequestListLimit),
		"--json",
		"headRefName,headRefOid,mergedAt",
	}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("repoExecCommandStep args = %#v, want %#v", gotArgs, wantArgs)
	}
}
