package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/scottlz0310/devsync/internal/secret"
	"github.com/scottlz0310/devsync/internal/testutil"
	"github.com/spf13/cobra"
)

// setupSecretsEnabledConfig はテスト用の一時ディレクトリに secrets.enabled=true の設定を作成し、
// HOME / USERPROFILE を差し替えることで config.Load() が実際の設定を読まないようにします。
func setupSecretsEnabledConfig(t *testing.T) {
	t.Helper()

	tmpHome := t.TempDir()
	configDir := filepath.Join(tmpHome, ".config", "devsync")

	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	configContent := "version: 1\nsecrets:\n  enabled: true\n  provider: bitwarden\n"
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	testutil.SetTestHome(t, tmpHome)
}

func TestRunDaily(t *testing.T) {
	originalUnlock := runUnlockStep
	originalLoadEnv := runLoadEnvStep
	originalSysUpdate := runSysUpdateStep
	originalRepoUpdate := runRepoUpdateStep

	t.Cleanup(func() {
		runUnlockStep = originalUnlock
		runLoadEnvStep = originalLoadEnv
		runSysUpdateStep = originalSysUpdate
		runRepoUpdateStep = originalRepoUpdate
	})

	testCases := []struct {
		name          string
		unlockErr     error
		loadEnvStats  *secret.LoadStats
		loadEnvErr    error
		sysUpdateErr  error
		repoUpdateErr error
		wantErr       bool
		wantErrSubstr string
		wantCalls     []string
	}{
		{
			name:         "全工程成功",
			loadEnvStats: &secret.LoadStats{Loaded: 1},
			wantCalls:    []string{"unlock", "load_env", "sys_update", "repo_update"},
		},
		{
			name:         "環境変数読み込み失敗は継続",
			loadEnvErr:   errors.New("load env failed"),
			loadEnvStats: &secret.LoadStats{},
			wantCalls:    []string{"unlock", "load_env", "sys_update", "repo_update"},
		},
		{
			name:      "アンロック失敗でスキップして続行",
			unlockErr: errors.New("unlock failed"),
			// アンロック失敗時は secrets フェーズをスキップし、sys/repo 更新を続行する
			wantCalls: []string{"unlock", "sys_update", "repo_update"},
		},
		{
			name:          "sys更新失敗で中断",
			loadEnvStats:  &secret.LoadStats{Loaded: 1},
			sysUpdateErr:  errors.New("sys failed"),
			wantErr:       true,
			wantErrSubstr: "システム更新に失敗しました",
			wantCalls:     []string{"unlock", "load_env", "sys_update"},
		},
		{
			name:          "repo同期失敗で中断",
			loadEnvStats:  &secret.LoadStats{Loaded: 1},
			repoUpdateErr: errors.New("repo failed"),
			wantErr:       true,
			wantErrSubstr: "リポジトリ同期に失敗しました",
			wantCalls:     []string{"unlock", "load_env", "sys_update", "repo_update"},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			setupSecretsEnabledConfig(t)
			t.Setenv("GPAT", "dummy-token")
			t.Setenv("DEVSYNC_ENV_LOADED", "")

			calls := make([]string, 0, 4)

			runUnlockStep = func() error {
				calls = append(calls, "unlock")
				return tc.unlockErr
			}

			runLoadEnvStep = func() (*secret.LoadStats, error) {
				calls = append(calls, "load_env")
				return tc.loadEnvStats, tc.loadEnvErr
			}

			runSysUpdateStep = func(*cobra.Command, []string) error {
				calls = append(calls, "sys_update")
				return tc.sysUpdateErr
			}

			runRepoUpdateStep = func(*cobra.Command, []string) error {
				calls = append(calls, "repo_update")
				return tc.repoUpdateErr
			}

			err := runDaily(&cobra.Command{Use: "run"}, nil)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("runDaily() error = nil, want error")
				}

				if tc.wantErrSubstr != "" && !strings.Contains(err.Error(), tc.wantErrSubstr) {
					t.Fatalf("runDaily() error = %q, want substring %q", err.Error(), tc.wantErrSubstr)
				}
			} else if err != nil {
				t.Fatalf("runDaily() unexpected error: %v", err)
			}

			if !reflect.DeepEqual(calls, tc.wantCalls) {
				t.Fatalf("runDaily() calls = %#v, want %#v", calls, tc.wantCalls)
			}
		})
	}
}

func TestRunDaily_SecretsDisabled(t *testing.T) {
	originalSysUpdate := runSysUpdateStep
	originalRepoUpdate := runRepoUpdateStep

	t.Cleanup(func() {
		runSysUpdateStep = originalSysUpdate
		runRepoUpdateStep = originalRepoUpdate
	})

	// secrets.enabled=false のデフォルト設定を使う（設定ファイルなしの一時HOME）
	testutil.SetTestHome(t, t.TempDir())

	calls := make([]string, 0, 2)

	runSysUpdateStep = func(*cobra.Command, []string) error {
		calls = append(calls, "sys_update")
		return nil
	}

	runRepoUpdateStep = func(*cobra.Command, []string) error {
		calls = append(calls, "repo_update")
		return nil
	}

	err := runDaily(&cobra.Command{Use: "run"}, nil)
	if err != nil {
		t.Fatalf("runDaily() unexpected error: %v", err)
	}

	// secrets が無効なので unlock/load_env は呼ばれない
	want := []string{"sys_update", "repo_update"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("runDaily() calls = %#v, want %#v", calls, want)
	}
}

func TestRunDaily_EnvAlreadyLoaded(t *testing.T) {
	originalUnlock := runUnlockStep
	originalLoadEnv := runLoadEnvStep
	originalSysUpdate := runSysUpdateStep
	originalRepoUpdate := runRepoUpdateStep

	t.Cleanup(func() {
		runUnlockStep = originalUnlock
		runLoadEnvStep = originalLoadEnv
		runSysUpdateStep = originalSysUpdate
		runRepoUpdateStep = originalRepoUpdate
	})

	setupSecretsEnabledConfig(t)
	t.Setenv("DEVSYNC_ENV_LOADED", "1")

	calls := make([]string, 0, 4)

	runUnlockStep = func() error {
		calls = append(calls, "unlock")
		return nil
	}

	runLoadEnvStep = func() (*secret.LoadStats, error) {
		calls = append(calls, "load_env")
		return &secret.LoadStats{Loaded: 1}, nil
	}

	runSysUpdateStep = func(*cobra.Command, []string) error {
		calls = append(calls, "sys_update")
		return nil
	}

	runRepoUpdateStep = func(*cobra.Command, []string) error {
		calls = append(calls, "repo_update")
		return nil
	}

	err := runDaily(&cobra.Command{Use: "run"}, nil)
	if err != nil {
		t.Fatalf("runDaily() unexpected error: %v", err)
	}

	// DEVSYNC_ENV_LOADED=1 の場合、load_env はスキップされる
	want := []string{"unlock", "sys_update", "repo_update"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("runDaily() calls = %#v, want %#v", calls, want)
	}
}
