package secret

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunWithEnv(t *testing.T) {
	t.Run("コマンド未指定はエラー", func(t *testing.T) {
		err := RunWithEnv(nil, map[string]string{"X": "1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "コマンドが指定されていません")
	})

	t.Run("コマンドが見つからない場合はエラー", func(t *testing.T) {
		err := RunWithEnv([]string{"definitely-not-a-real-command-devsync"}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "が見つかりません")
	})

	t.Run("環境変数の上書きが不要な場合はそのまま返す", func(t *testing.T) {
		base := []string{"A=1", "B=2"}
		got := mergeEnv(base, nil)
		assert.Equal(t, base, got)
	})

	t.Run("注入した環境変数が子プロセスで参照できる", func(t *testing.T) {
		exe, err := os.Executable()
		require.NoError(t, err)

		// 事前に異なる値を設定しておき、注入値が優先されることも合わせて確認します。
		t.Setenv("DEVSYNC_TEST", "0")

		args := []string{
			exe,
			"-test.run=TestRunWithEnvHelperProcess",
		}

		envVars := map[string]string{
			"DEVSYNC_TEST_HELPER_PROCESS": "1",
			"DEVSYNC_TEST":                "1",
		}

		require.NoError(t, RunWithEnv(args, envVars))
	})

	t.Run("注入値が期待と異なる場合は子プロセスが失敗する", func(t *testing.T) {
		exe, err := os.Executable()
		require.NoError(t, err)

		args := []string{
			exe,
			"-test.run=TestRunWithEnvHelperProcess",
		}

		envVars := map[string]string{
			"DEVSYNC_TEST_HELPER_PROCESS": "1",
			"DEVSYNC_TEST":                "0",
		}

		require.Error(t, RunWithEnv(args, envVars))
	})
}

func TestRunWithEnvHelperProcess(t *testing.T) {
	if os.Getenv("DEVSYNC_TEST_HELPER_PROCESS") != "1" {
		return
	}

	if os.Getenv("DEVSYNC_TEST") != "1" {
		os.Exit(1)
	}

	os.Exit(0)
}

func TestMergeEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		base      []string
		overrides map[string]string
		wantLen   int
		wantKV    map[string]string
		notWantKV map[string]string
	}{
		{
			name:      "空のオーバーライド",
			base:      []string{"A=1", "B=2"},
			overrides: nil,
			wantLen:   2,
			wantKV:    map[string]string{"A": "1", "B": "2"},
		},
		{
			name:      "空のベース",
			base:      nil,
			overrides: map[string]string{"X": "10"},
			wantLen:   1,
			wantKV:    map[string]string{"X": "10"},
		},
		{
			name:      "既存キーの上書き",
			base:      []string{"A=1", "B=2", "C=3"},
			overrides: map[string]string{"B": "20"},
			wantLen:   3,
			wantKV:    map[string]string{"A": "1", "B": "20", "C": "3"},
			notWantKV: map[string]string{"B": "2"},
		},
		{
			name:      "新規キーの追加",
			base:      []string{"A=1"},
			overrides: map[string]string{"NEW": "val"},
			wantLen:   2,
			wantKV:    map[string]string{"A": "1", "NEW": "val"},
		},
		{
			name:      "複数のオーバーライド",
			base:      []string{"A=1", "B=2", "C=3"},
			overrides: map[string]string{"A": "10", "C": "30", "D": "40"},
			wantLen:   4,
			wantKV:    map[string]string{"A": "10", "B": "2", "C": "30", "D": "40"},
		},
		{
			name:      "値にイコール含む",
			base:      []string{"PATH=/usr/bin:/bin"},
			overrides: map[string]string{"PATH": "/new:/usr/bin"},
			wantLen:   1,
			wantKV:    map[string]string{"PATH": "/new:/usr/bin"},
		},
		{
			name:      "空値のオーバーライド",
			base:      []string{"A=1"},
			overrides: map[string]string{"A": ""},
			wantLen:   1,
			wantKV:    map[string]string{"A": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := mergeEnv(tt.base, tt.overrides)

			if len(got) != tt.wantLen {
				t.Errorf("mergeEnv() len = %d, want %d; got = %v", len(got), tt.wantLen, got)
			}

			envMap := make(map[string]string)
			for _, kv := range got {
				parts := strings.SplitN(kv, "=", 2)
				if len(parts) == 2 {
					envMap[parts[0]] = parts[1]
				}
			}

			for k, v := range tt.wantKV {
				if envMap[k] != v {
					t.Errorf("env[%q] = %q, want %q", k, envMap[k], v)
				}
			}

			for k, v := range tt.notWantKV {
				if envMap[k] == v {
					t.Errorf("env[%q] should not be %q", k, v)
				}
			}
		})
	}
}
