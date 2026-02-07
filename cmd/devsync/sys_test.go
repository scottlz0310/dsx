package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/scottlz0310/devsync/internal/config"
	"github.com/scottlz0310/devsync/internal/updater"
)

type stubUpdater struct {
	name      string
	updateErr error
}

func (s stubUpdater) Name() string {
	return s.name
}

func (s stubUpdater) DisplayName() string {
	return s.name
}

func (s stubUpdater) IsAvailable() bool {
	return true
}

func (s stubUpdater) Check(context.Context) (*updater.CheckResult, error) {
	return &updater.CheckResult{}, nil
}

func (s stubUpdater) Update(context.Context, updater.UpdateOptions) (*updater.UpdateResult, error) {
	if s.updateErr != nil {
		return nil, s.updateErr
	}

	return &updater.UpdateResult{}, nil
}

func (s stubUpdater) Configure(config.ManagerConfig) error {
	return nil
}

func TestResolveSysJobs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		configJobs int
		flagJobs   int
		want       int
	}{
		{
			name:       "フラグ優先",
			configJobs: 8,
			flagJobs:   3,
			want:       3,
		},
		{
			name:       "フラグ未指定なら設定値",
			configJobs: 6,
			flagJobs:   0,
			want:       6,
		},
		{
			name:       "設定が不正なら1",
			configJobs: 0,
			flagJobs:   0,
			want:       1,
		},
		{
			name:       "負数フラグは設定値にフォールバック",
			configJobs: 5,
			flagJobs:   -1,
			want:       5,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := resolveSysJobs(tc.configJobs, tc.flagJobs)
			if got != tc.want {
				t.Fatalf("resolveSysJobs(%d, %d) = %d, want %d", tc.configJobs, tc.flagJobs, got, tc.want)
			}
		})
	}
}

func TestSplitUpdatersForExecution(t *testing.T) {
	t.Parallel()

	input := []updater.Updater{
		stubUpdater{name: "brew"},
		stubUpdater{name: "apt"},
		stubUpdater{name: "go"},
	}

	exclusive, parallel := splitUpdatersForExecution(input)

	if len(exclusive) != 1 {
		t.Fatalf("exclusive length = %d, want 1", len(exclusive))
	}

	if exclusive[0].Name() != "apt" {
		t.Fatalf("exclusive[0] = %s, want apt", exclusive[0].Name())
	}

	if len(parallel) != 2 {
		t.Fatalf("parallel length = %d, want 2", len(parallel))
	}

	if parallel[0].Name() != "brew" || parallel[1].Name() != "go" {
		t.Fatalf("parallel order = [%s, %s], want [brew, go]", parallel[0].Name(), parallel[1].Name())
	}
}

func TestMustRunExclusively(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		in   updater.Updater
		want bool
	}{
		{
			name: "aptは単独実行",
			in:   stubUpdater{name: "apt"},
			want: true,
		},
		{
			name: "brewは並列可",
			in:   stubUpdater{name: "brew"},
			want: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := mustRunExclusively(tc.in)
			if got != tc.want {
				t.Fatalf("mustRunExclusively(%s) = %v, want %v", tc.in.Name(), got, tc.want)
			}
		})
	}
}

func TestExecuteUpdatesParallel_ContextCanceledIsNotFailed(t *testing.T) {
	t.Parallel()

	updaters := []updater.Updater{
		stubUpdater{
			name:      "brew",
			updateErr: context.Canceled,
		},
	}

	stats := executeUpdatesParallel(context.Background(), updaters, updater.UpdateOptions{}, 2)

	if stats.Failed != 0 {
		t.Fatalf("Failed = %d, want 0", stats.Failed)
	}

	if len(stats.Errors) != 1 {
		t.Fatalf("Errors length = %d, want 1", len(stats.Errors))
	}

	if !strings.Contains(stats.Errors[0].Error(), "スキップ") {
		t.Fatalf("Errors[0] = %q, want skipped message", stats.Errors[0].Error())
	}
}

func TestExecuteUpdatesParallel_NonContextErrorIsFailed(t *testing.T) {
	t.Parallel()

	updaters := []updater.Updater{
		stubUpdater{
			name:      "brew",
			updateErr: errors.New("update failure"),
		},
	}

	stats := executeUpdatesParallel(context.Background(), updaters, updater.UpdateOptions{}, 2)

	if stats.Failed != 1 {
		t.Fatalf("Failed = %d, want 1", stats.Failed)
	}

	if len(stats.Errors) != 1 {
		t.Fatalf("Errors length = %d, want 1", len(stats.Errors))
	}

	if !strings.Contains(stats.Errors[0].Error(), "update failure") {
		t.Fatalf("Errors[0] = %q, want update failure", stats.Errors[0].Error())
	}
}
