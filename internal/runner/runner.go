// Package runner は汎用的な並列実行基盤を提供します。
package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

const defaultMaxJobs = 1

// Job は実行対象のジョブです。
type Job struct {
	Name string
	Run  func(context.Context) error
}

// ResultStatus はジョブ実行結果の状態です。
type ResultStatus string

const (
	StatusSuccess ResultStatus = "success"
	StatusFailed  ResultStatus = "failed"
	StatusSkipped ResultStatus = "skipped"
)

// Result は単一ジョブの実行結果です。
type Result struct {
	Name     string
	Status   ResultStatus
	Err      error
	Duration time.Duration
}

// Summary は全ジョブの実行集計です。
type Summary struct {
	Total   int
	Success int
	Failed  int
	Skipped int
	Results []Result
}

// Execute はジョブを指定並列数で実行し、結果を返します。
func Execute(ctx context.Context, maxJobs int, jobs []Job) Summary {
	summary := Summary{
		Total:   len(jobs),
		Results: make([]Result, len(jobs)),
	}

	if len(jobs) == 0 {
		return summary
	}

	maxJobs = normalizeMaxJobs(maxJobs)

	sem := semaphore.NewWeighted(int64(maxJobs))
	group, groupCtx := errgroup.WithContext(ctx)

	var mu sync.Mutex

	for index, job := range jobs {
		i := index
		currentJob := job
		start := time.Now()
		name := normalizeJobName(i, currentJob.Name)

		if currentJob.Run == nil {
			recordResult(&mu, &summary, i, Result{
				Name:     name,
				Status:   StatusFailed,
				Err:      fmt.Errorf("ジョブ実体が nil です"),
				Duration: time.Since(start),
			})

			continue
		}

		if err := sem.Acquire(groupCtx, 1); err != nil {
			recordResult(&mu, &summary, i, Result{
				Name:     name,
				Status:   StatusSkipped,
				Err:      err,
				Duration: time.Since(start),
			})

			continue
		}

		group.Go(func() error {
			// Acquire 済みのため、完了時に必ず Release する。
			defer sem.Release(1)

			if err := groupCtx.Err(); err != nil {
				recordResult(&mu, &summary, i, Result{
					Name:     name,
					Status:   StatusSkipped,
					Err:      err,
					Duration: time.Since(start),
				})

				return nil
			}

			err := currentJob.Run(groupCtx)
			status := resolveStatus(err)

			recordResult(&mu, &summary, i, Result{
				Name:     name,
				Status:   status,
				Err:      err,
				Duration: time.Since(start),
			})

			// errgroup の fail-fast を無効化するため、エラーを返さない。
			return nil
		})
	}

	if waitErr := group.Wait(); waitErr != nil {
		// group.Go は nil を返す設計だが、将来の実装変更に備えて集計に残す。
		summary.Results = append(summary.Results, Result{
			Name:   "runner",
			Status: StatusFailed,
			Err:    waitErr,
		})
		summary.Total++
	}

	recount(&summary)

	return summary
}

func normalizeMaxJobs(maxJobs int) int {
	if maxJobs <= 0 {
		return defaultMaxJobs
	}

	return maxJobs
}

func normalizeJobName(index int, name string) string {
	if name != "" {
		return name
	}

	return fmt.Sprintf("job-%d", index+1)
}

func resolveStatus(err error) ResultStatus {
	if err == nil {
		return StatusSuccess
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return StatusSkipped
	}

	return StatusFailed
}

func recordResult(mu *sync.Mutex, summary *Summary, index int, result Result) {
	mu.Lock()
	defer mu.Unlock()

	summary.Results[index] = result
}

func recount(summary *Summary) {
	for _, result := range summary.Results {
		switch result.Status {
		case StatusSuccess:
			summary.Success++
		case StatusFailed:
			summary.Failed++
		case StatusSkipped:
			summary.Skipped++
		}
	}
}
