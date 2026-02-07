package runner

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestExecute(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		maxJobs       int
		buildContext  func() context.Context
		jobs          []Job
		wantSuccess   int
		wantFailed    int
		wantSkipped   int
		assertResults func(t *testing.T, summary Summary)
	}{
		{
			name:         "全ジョブ成功",
			maxJobs:      2,
			buildContext: context.Background,
			jobs: []Job{
				{
					Name: "job1",
					Run: func(context.Context) error {
						time.Sleep(10 * time.Millisecond)
						return nil
					},
				},
				{
					Name: "job2",
					Run: func(context.Context) error {
						time.Sleep(10 * time.Millisecond)
						return nil
					},
				},
			},
			wantSuccess: 2,
			wantFailed:  0,
			wantSkipped: 0,
		},
		{
			name:         "ジョブ失敗を集計",
			maxJobs:      2,
			buildContext: context.Background,
			jobs: []Job{
				{
					Name: "ok",
					Run: func(context.Context) error {
						return nil
					},
				},
				{
					Name: "ng",
					Run: func(context.Context) error {
						return errors.New("expected failure")
					},
				},
			},
			wantSuccess: 1,
			wantFailed:  1,
			wantSkipped: 0,
			assertResults: func(t *testing.T, summary Summary) {
				t.Helper()

				if summary.Results[1].Err == nil {
					t.Fatalf("失敗ジョブにエラーが入っていません")
				}
			},
		},
		{
			name:         "maxJobs が 0 以下なら 1 扱い",
			maxJobs:      0,
			buildContext: context.Background,
			jobs: []Job{
				{
					Name: "job1",
					Run: func(context.Context) error {
						time.Sleep(20 * time.Millisecond)
						return nil
					},
				},
				{
					Name: "job2",
					Run: func(context.Context) error {
						time.Sleep(20 * time.Millisecond)
						return nil
					},
				},
			},
			wantSuccess: 2,
			wantFailed:  0,
			wantSkipped: 0,
		},
		{
			name:    "事前キャンセル時はスキップ",
			maxJobs: 2,
			buildContext: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			jobs: []Job{
				{
					Name: "job1",
					Run: func(context.Context) error {
						return nil
					},
				},
				{
					Name: "job2",
					Run: func(context.Context) error {
						return nil
					},
				},
			},
			wantSuccess: 0,
			wantFailed:  0,
			wantSkipped: 2,
		},
		{
			name:         "nil ジョブは失敗扱い",
			maxJobs:      1,
			buildContext: context.Background,
			jobs: []Job{
				{
					Name: "nil-job",
					Run:  nil,
				},
			},
			wantSuccess: 0,
			wantFailed:  1,
			wantSkipped: 0,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var runCount int32

			jobs := cloneJobs(tc.jobs, &runCount)
			ctx := tc.buildContext()

			summary := Execute(ctx, tc.maxJobs, jobs)

			if summary.Total != len(tc.jobs) {
				t.Fatalf("Total = %d, want %d", summary.Total, len(tc.jobs))
			}

			if summary.Success != tc.wantSuccess {
				t.Fatalf("Success = %d, want %d", summary.Success, tc.wantSuccess)
			}

			if summary.Failed != tc.wantFailed {
				t.Fatalf("Failed = %d, want %d", summary.Failed, tc.wantFailed)
			}

			if summary.Skipped != tc.wantSkipped {
				t.Fatalf("Skipped = %d, want %d", summary.Skipped, tc.wantSkipped)
			}

			if tc.wantSkipped == len(tc.jobs) {
				if got := atomic.LoadInt32(&runCount); got != 0 {
					t.Fatalf("キャンセル時にジョブが実行されています: %d", got)
				}
			}

			if tc.assertResults != nil {
				tc.assertResults(t, summary)
			}
		})
	}
}

func TestNormalizeMaxJobs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		in   int
		want int
	}{
		{
			name: "負数は1",
			in:   -1,
			want: 1,
		},
		{
			name: "0は1",
			in:   0,
			want: 1,
		},
		{
			name: "正数はそのまま",
			in:   4,
			want: 4,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := normalizeMaxJobs(tc.in)
			if got != tc.want {
				t.Fatalf("normalizeMaxJobs(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func cloneJobs(jobs []Job, runCount *int32) []Job {
	result := make([]Job, 0, len(jobs))

	for _, job := range jobs {
		current := job
		if current.Run != nil {
			originalRun := current.Run
			current.Run = func(ctx context.Context) error {
				atomic.AddInt32(runCount, 1)
				return originalRun(ctx)
			}
		}

		result = append(result, current)
	}

	return result
}
