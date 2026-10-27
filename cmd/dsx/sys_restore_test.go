package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/scottlz0310/dsx/internal/restore"
	"github.com/scottlz0310/dsx/internal/snapshot"
	"github.com/stretchr/testify/require"
)

func TestSysRestoreCommand(t *testing.T) {
	for _, tc := range []struct {
		name                                                       string
		input                                                      string
		args                                                       []string
		missing, existing, unavailable, resolutionError, cancelled bool
		wantError                                                  bool
		wantText                                                   string
	}{
		{name: "追加候補", wantText: "追加候補"},
		{name: "導入済み", existing: true, wantText: "導入済み"},
		{name: "dry-run 必須", args: []string{"--dry-run=false"}, wantError: true, wantText: "--dry-run"},
		{name: "manifest 不在", missing: true, wantError: true, wantText: "読み込みに失敗"},
		{name: "不正な manifest", input: "version: 2\ntools: {}", wantError: true, wantText: "スキーマバージョン"},
		{name: "マネージャ未導入", unavailable: true, wantError: true, wantText: "解決不能"},
		{name: "提供版の照会失敗", resolutionError: true, wantError: true, wantText: "解決不能"},
		{name: "キャンセル", cancelled: true, wantError: true, wantText: "作成に失敗"},
		{name: "タイムアウト不正", args: []string{"--timeout", "0s"}, wantError: true, wantText: "正の時間"},
		{name: "引数を拒否", args: []string{"unexpected"}, wantError: true, wantText: "unknown command"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "tools.yaml")

			input := tc.input
			if input == "" {
				input = "version: 1\ntools:\n  uv:\n    - package: ruff\n      version: 1.0"
			}

			if !tc.missing {
				require.NoError(t, os.WriteFile(path, []byte(input), 0o600))
			}

			inventoryCalls, resolveCalls := 0, 0
			manager := restore.Manager{
				Available:  func() bool { return !tc.unavailable },
				Compatible: func(string) bool { return true },
				Installed: func(context.Context) ([]snapshot.Tool, error) {
					inventoryCalls++

					if tc.existing {
						return []snapshot.Tool{{Package: "ruff", Version: "1.0"}}, nil
					}

					return nil, nil
				},
				Resolve: func(context.Context, snapshot.Tool) (restore.Command, error) {
					resolveCalls++

					if tc.resolutionError {
						return restore.Command{}, errors.New("提供版なし")
					}

					return restore.Command{Args: []string{"uv", "tool", "install", "ruff==1.0"}, Version: "1.0"}, nil
				},
			}
			cmd := newSysRestoreCmd(func() map[string]restore.Manager { return map[string]restore.Manager{"uv": manager} })
			cmd.SetArgs(append([]string{"--file", path, "--dry-run", "--timeout", "1m"}, tc.args...))

			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tc.cancelled {
				cancel()
			}

			err := cmd.ExecuteContext(ctx)

			text := output.String()
			if tc.wantError {
				require.Error(t, err)
				text += err.Error()
			} else {
				require.NoError(t, err)
			}

			require.Contains(t, text, tc.wantText)

			if tc.existing {
				require.Zero(t, resolveCalls)
			}

			if inventoryCalls == 0 {
				require.Zero(t, resolveCalls)
			}

			if !tc.missing {
				data, readErr := os.ReadFile(path)
				require.NoError(t, readErr)
				require.Equal(t, input, string(data))
			}
		})
	}
}
