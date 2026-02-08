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
