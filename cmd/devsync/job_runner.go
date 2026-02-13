package main

import (
	"context"
	"fmt"
	"os"

	"github.com/scottlz0310/devsync/internal/runner"
	progressui "github.com/scottlz0310/devsync/internal/tui"
)

func runJobsWithOptionalTUI(ctx context.Context, title string, jobs int, execJobs []runner.Job, useTUI bool) runner.Summary {
	if !useTUI {
		return runner.Execute(ctx, jobs, execJobs)
	}

	summary, err := progressui.RunJobProgress(ctx, title, jobs, execJobs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  TUI表示中にエラーが発生しました（実行結果は継続）: %v\n", err)
	}

	return summary
}

// printFailedJobDetails は runner.Summary から失敗ジョブのエラー詳細を表示します。
func printFailedJobDetails(summary runner.Summary) {
	var failures []runner.Result

	for _, r := range summary.Results {
		if r.Status == runner.StatusFailed && r.Err != nil {
			failures = append(failures, r)
		}
	}

	if len(failures) == 0 {
		return
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "❌ 失敗ジョブの詳細:")

	for i, f := range failures {
		prefix := "  ├──"
		if i == len(failures)-1 {
			prefix = "  └──"
		}

		fmt.Fprintf(os.Stderr, "%s %s: %v\n", prefix, f.Name, f.Err)
	}
}

// printFailedErrors はエラー一覧から失敗詳細を表示します（sys update 用）。
func printFailedErrors(errors []error) {
	if len(errors) == 0 {
		return
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "❌ 失敗ジョブの詳細:")

	for i, err := range errors {
		prefix := "  ├──"
		if i == len(errors)-1 {
			prefix = "  └──"
		}

		fmt.Fprintf(os.Stderr, "%s %v\n", prefix, err)
	}
}
