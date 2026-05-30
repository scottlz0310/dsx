package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/scottlz0310/dsx/internal/config"
	"github.com/scottlz0310/dsx/internal/updater"
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

type selfUpdateStubUpdater struct {
	stubUpdater
	checkResult *updater.CheckResult
	checkErr    error
	selfResult  *updater.SelfUpdateResult
	selfErr     error
	checkCalls  int
	selfCalls   int
}

func (s *selfUpdateStubUpdater) CheckSelfUpdate(context.Context) (*updater.CheckResult, error) {
	s.checkCalls++

	if s.checkErr != nil {
		return nil, s.checkErr
	}

	if s.checkResult != nil {
		return s.checkResult, nil
	}

	return &updater.CheckResult{}, nil
}

func (s *selfUpdateStubUpdater) SelfUpdate(context.Context, updater.UpdateOptions) (*updater.SelfUpdateResult, error) {
	s.selfCalls++

	if s.selfErr != nil {
		return s.selfResult, s.selfErr
	}

	if s.selfResult != nil {
		return s.selfResult, nil
	}

	return &updater.SelfUpdateResult{Continuation: updater.ContinueNormalUpdate}, nil
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

func TestExecuteManagerSelfUpdate_DryRunUsesCheckOnly(t *testing.T) {
	u := &selfUpdateStubUpdater{
		stubUpdater: stubUpdater{name: "uv"},
		checkResult: &updater.CheckResult{
			AvailableUpdates: 1,
			Packages: []updater.PackageInfo{
				{Name: "uv", CurrentVersion: "0.11.16", NewVersion: "0.11.17"},
			},
			Message: "uv 本体の更新が可能です",
		},
	}

	got, err := executeManagerSelfUpdate(context.Background(), u, updater.UpdateOptions{DryRun: true})
	if err != nil {
		t.Fatalf("executeManagerSelfUpdate returned error: %v", err)
	}

	if u.checkCalls != 1 {
		t.Fatalf("CheckSelfUpdate calls = %d, want 1", u.checkCalls)
	}

	if u.selfCalls != 0 {
		t.Fatalf("SelfUpdate calls = %d, want 0", u.selfCalls)
	}

	if got.UpdatedCount != 0 {
		t.Fatalf("UpdatedCount = %d, want 0", got.UpdatedCount)
	}

	if !strings.Contains(got.Message, "DryRun") {
		t.Fatalf("Message = %q, want DryRun", got.Message)
	}
}

func TestRunManagerSelfUpdatePhase_SkipNormalUpdate(t *testing.T) {
	selfUpdater := &selfUpdateStubUpdater{
		stubUpdater: stubUpdater{name: "uv"},
		selfResult: &updater.SelfUpdateResult{
			UpdateResult: updater.UpdateResult{
				UpdatedCount: 1,
				Message:      "uv 本体を更新しました",
			},
			Continuation: updater.SkipNormalUpdate,
		},
	}
	normalUpdater := stubUpdater{name: "npm"}

	remaining, stats := runManagerSelfUpdatePhase(
		context.Background(),
		updater.UpdateOptions{},
		[]updater.Updater{selfUpdater, normalUpdater},
		false,
	)

	if stats.Updated != 1 {
		t.Fatalf("Updated = %d, want 1", stats.Updated)
	}

	if len(remaining) != 1 || remaining[0].Name() != "npm" {
		names := make([]string, 0, len(remaining))
		for _, u := range remaining {
			names = append(names, u.Name())
		}

		t.Fatalf("remaining = %v, want [npm]", names)
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
