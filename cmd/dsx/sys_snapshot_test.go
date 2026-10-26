package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scottlz0310/dsx/internal/snapshot"
	"github.com/scottlz0310/dsx/internal/updater"
	"github.com/stretchr/testify/require"
)

type snapshotStubUpdater struct {
	stubUpdater
	err   error
	calls int
}

func (s *snapshotStubUpdater) Snapshot(context.Context) ([]snapshot.Tool, error) {
	s.calls++
	return []snapshot.Tool{{Package: "ruff", Version: "0.12.0"}}, s.err
}

func TestSysSnapshotCommand(t *testing.T) {
	for _, tc := range []struct {
		name                                         string
		existing, force, failure, cancelled, success bool
		extra                                        []string
	}{
		{name: "新規保存", success: true},
		{name: "既存保護", existing: true},
		{name: "明示上書き", existing: true, force: true, success: true},
		{name: "失敗時に保存しない", failure: true},
		{name: "失敗時に既存保護", existing: true, force: true, failure: true},
		{name: "キャンセル", cancelled: true},
		{name: "引数を拒否", extra: []string{"unexpected"}},
		{name: "ゼロのタイムアウト", extra: []string{"--timeout", "0s"}},
		{name: "負のタイムアウト", extra: []string{"--timeout", "-1s"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "tools.yaml")
			manager := &snapshotStubUpdater{stubUpdater: stubUpdater{name: "uv"}}
			args := []string{"--output", path, "--timeout", "1m"}

			if tc.existing {
				require.NoError(t, os.WriteFile(path, []byte("既存"), 0o600))
			}

			if tc.force {
				args = append(args, "--force")
			}

			if tc.failure {
				manager.err = errors.New("取得不能")
			}

			args = append(args, tc.extra...)

			cmd := newSysSnapshotCmd(func() []updater.Updater { return []updater.Updater{manager} })
			cmd.SetArgs(args)

			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)

			ctx := context.Background()
			if tc.cancelled {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()

				ctx = cancelled
			}

			err := cmd.ExecuteContext(ctx)
			if tc.success {
				require.NoError(t, err)

				data, readErr := os.ReadFile(path)
				require.NoError(t, readErr)
				require.Contains(t, string(data), "package: ruff")
				require.Contains(t, output.String(), "保存しました")

				return
			}

			require.Error(t, err)
			require.NotContains(t, output.String(), "保存しました")

			if tc.existing {
				data, readErr := os.ReadFile(path)
				require.NoError(t, readErr)
				require.Equal(t, "既存", string(data))
			} else {
				_, statErr := os.Stat(path)
				require.ErrorIs(t, statErr, os.ErrNotExist)
			}

			if tc.existing && !tc.force {
				require.Zero(t, manager.calls)
				require.ErrorContains(t, err, "--force")
			}
		})
	}
}

func TestSnapshotTimeoutConfig(t *testing.T) {
	for _, tc := range []struct {
		name, configTimeout string
		explicit            bool
		wantError           bool
	}{
		{"設定を使用", "2s", false, false},
		{"不正な設定", "invalid", false, true},
		{"明示フラグ優先", "invalid", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DSX_CONTROL_TIMEOUT", tc.configTimeout)

			cmd := newSysSnapshotCmd(updater.All)
			if tc.explicit {
				require.NoError(t, cmd.ParseFlags([]string{"--timeout", "3s"}))
			}

			duration, err := snapshotTimeout(cmd, 3*time.Second)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)

				want := int64(2)
				if tc.explicit {
					want = 3
				}

				require.Equal(t, want, int64(duration.Seconds()))
			}
		})
	}
}
