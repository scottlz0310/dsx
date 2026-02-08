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

	stats := executeUpdatesParallel(context.Background(), updaters, updater.UpdateOptions{}, 2, false)

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

	stats := executeUpdatesParallel(context.Background(), updaters, updater.UpdateOptions{}, 2, false)

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

func TestEnabledMark(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		enabled bool
		want    string
	}{
		{
			name:    "有効",
			enabled: true,
			want:    "✅",
		},
		{
			name:    "無効",
			enabled: false,
			want:    "❌",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := enabledMark(tc.enabled)
			if got != tc.want {
				t.Fatalf("enabledMark(%v) = %q, want %q", tc.enabled, got, tc.want)
			}
		})
	}
}

func TestUpdaterRequiresSudo(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		updater  string
		managers map[string]config.ManagerConfig
		want     bool
	}{
		{
			name:    "aptはデフォルトでsudo必要",
			updater: "apt",
			managers: map[string]config.ManagerConfig{
				"apt": {},
			},
			want: true,
		},
		{
			name:    "aptはuse_sudo=falseでsudo不要",
			updater: "apt",
			managers: map[string]config.ManagerConfig{
				"apt": {"use_sudo": false},
			},
			want: false,
		},
		{
			name:    "snapはsudo=falseでsudo不要（旧キー互換）",
			updater: "snap",
			managers: map[string]config.ManagerConfig{
				"snap": {"sudo": false},
			},
			want: false,
		},
		{
			name:    "brewはsudo不要",
			updater: "brew",
			want:    false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := updaterRequiresSudo(tc.updater, tc.managers)
			if got != tc.want {
				t.Fatalf("updaterRequiresSudo(%q) = %v, want %v", tc.updater, got, tc.want)
			}
		})
	}
}

func TestPhaseRequiresSudo(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		updaters []updater.Updater
		managers map[string]config.ManagerConfig
		want     bool
	}{
		{
			name: "aptを含む場合はsudo必要",
			updaters: []updater.Updater{
				stubUpdater{name: "apt"},
				stubUpdater{name: "go"},
			},
			want: true,
		},
		{
			name: "snapがsudo無効なら不要",
			updaters: []updater.Updater{
				stubUpdater{name: "snap"},
			},
			managers: map[string]config.ManagerConfig{
				"snap": {"use_sudo": false},
			},
			want: false,
		},
		{
			name: "sudo対象がなければ不要",
			updaters: []updater.Updater{
				stubUpdater{name: "brew"},
				stubUpdater{name: "go"},
			},
			want: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := phaseRequiresSudo(tc.updaters, tc.managers)
			if got != tc.want {
				t.Fatalf("phaseRequiresSudo() = %v, want %v", got, tc.want)
			}
		})
	}
}
