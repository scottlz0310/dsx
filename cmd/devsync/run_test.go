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
			name:          "sys更新失敗でもrepo続行",
			loadEnvStats:  &secret.LoadStats{Loaded: 1},
			sysUpdateErr:  errors.New("sys failed"),
			wantErr:       true,
			wantErrSubstr: "1 件のフェーズでエラーが発生しました",
			wantCalls:     []string{"unlock", "load_env", "sys_update", "repo_update"},
		},
		{
			name:          "repo同期失敗",
			loadEnvStats:  &secret.LoadStats{Loaded: 1},
			repoUpdateErr: errors.New("repo failed"),
			wantErr:       true,
			wantErrSubstr: "1 件のフェーズでエラーが発生しました",
			wantCalls:     []string{"unlock", "load_env", "sys_update", "repo_update"},
		},
		{
			name:          "sys・repo両方失敗",
			loadEnvStats:  &secret.LoadStats{Loaded: 1},
			sysUpdateErr:  errors.New("sys failed"),
			repoUpdateErr: errors.New("repo failed"),
			wantErr:       true,
			wantErrSubstr: "2 件のフェーズでエラーが発生しました",
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

func TestPropagateRunFlags(t *testing.T) {
	// 各グローバルフラグの退避・復元
	origSysDryRun := sysDryRun
	origSysJobs := sysJobs
	origSysTUI := sysTUI
	origSysNoTUI := sysNoTUI
	origSysLogFile := sysLogFile
	origRepoDryRun := repoUpdateDryRun
	origRepoJobs := repoUpdateJobs
	origRepoTUI := repoUpdateTUI
	origRepoNoTUI := repoUpdateNoTUI
	origRepoLogFile := repoUpdateLogFile

	t.Cleanup(func() {
		sysDryRun = origSysDryRun
		sysJobs = origSysJobs
		sysTUI = origSysTUI
		sysNoTUI = origSysNoTUI
		sysLogFile = origSysLogFile
		repoUpdateDryRun = origRepoDryRun
		repoUpdateJobs = origRepoJobs
		repoUpdateTUI = origRepoTUI
		repoUpdateNoTUI = origRepoNoTUI
		repoUpdateLogFile = origRepoLogFile
	})

	tests := []struct {
		name            string
		setFlags        func(cmd *cobra.Command)
		wantSysDryRun   bool
		wantRepoDryRun  bool
		wantSysJobs     int
		wantRepoJobs    int
		wantSysTUI      bool
		wantRepoTUI     bool
		wantSysNoTUI    bool
		wantRepoNoTUI   bool
		wantSysLogFile  string
		wantRepoLogFile string
	}{
		{
			name:     "フラグ未指定時は伝播しない",
			setFlags: func(cmd *cobra.Command) {},
		},
		{
			name: "dry-runフラグが伝播される",
			setFlags: func(cmd *cobra.Command) {
				if err := cmd.Flags().Set("dry-run", "true"); err != nil {
					panic(err)
				}
			},
			wantSysDryRun:  true,
			wantRepoDryRun: true,
		},
		{
			name: "jobsフラグが伝播される",
			setFlags: func(cmd *cobra.Command) {
				if err := cmd.Flags().Set("jobs", "4"); err != nil {
					panic(err)
				}
			},
			wantSysJobs:  4,
			wantRepoJobs: 4,
		},
		{
			name: "tuiフラグが伝播される",
			setFlags: func(cmd *cobra.Command) {
				if err := cmd.Flags().Set("tui", "true"); err != nil {
					panic(err)
				}
			},
			wantSysTUI:  true,
			wantRepoTUI: true,
		},
		{
			name: "no-tuiフラグが伝播される",
			setFlags: func(cmd *cobra.Command) {
				if err := cmd.Flags().Set("no-tui", "true"); err != nil {
					panic(err)
				}
			},
			wantSysNoTUI:  true,
			wantRepoNoTUI: true,
		},
		{
			name: "log-fileフラグが伝播される",
			setFlags: func(cmd *cobra.Command) {
				if err := cmd.Flags().Set("log-file", "/tmp/test.log"); err != nil {
					panic(err)
				}
			},
			wantSysLogFile:  "/tmp/test.log",
			wantRepoLogFile: "/tmp/test.log",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// リセット
			sysDryRun = false
			sysJobs = 0
			sysTUI = false
			sysNoTUI = false
			sysLogFile = ""
			repoUpdateDryRun = false
			repoUpdateJobs = 0
			repoUpdateTUI = false
			repoUpdateNoTUI = false
			repoUpdateLogFile = ""

			cmd := &cobra.Command{Use: "run"}
			cmd.Flags().BoolVarP(&runDryRun, "dry-run", "n", false, "")
			cmd.Flags().IntVarP(&runJobs, "jobs", "j", 0, "")
			cmd.Flags().BoolVar(&runTUI, "tui", false, "")
			cmd.Flags().BoolVar(&runNoTUI, "no-tui", false, "")
			cmd.Flags().StringVar(&runLogFile, "log-file", "", "")
			tt.setFlags(cmd)

			propagateRunFlags(cmd)

			assertPropagatedFlags(t, tt.wantSysDryRun, tt.wantRepoDryRun, tt.wantSysJobs, tt.wantRepoJobs,
				tt.wantSysTUI, tt.wantRepoTUI, tt.wantSysNoTUI, tt.wantRepoNoTUI,
				tt.wantSysLogFile, tt.wantRepoLogFile)
		})
	}
}

//nolint:cyclop // テスト用アサーションヘルパーのため複雑度は許容する
func assertPropagatedFlags(t *testing.T,
	wantSysDryRun, wantRepoDryRun bool,
	wantSysJobs, wantRepoJobs int,
	wantSysTUI, wantRepoTUI, wantSysNoTUI, wantRepoNoTUI bool,
	wantSysLogFile, wantRepoLogFile string,
) {
	t.Helper()

	if sysDryRun != wantSysDryRun {
		t.Errorf("sysDryRun = %v, want %v", sysDryRun, wantSysDryRun)
	}

	if repoUpdateDryRun != wantRepoDryRun {
		t.Errorf("repoUpdateDryRun = %v, want %v", repoUpdateDryRun, wantRepoDryRun)
	}

	if sysJobs != wantSysJobs {
		t.Errorf("sysJobs = %v, want %v", sysJobs, wantSysJobs)
	}

	if repoUpdateJobs != wantRepoJobs {
		t.Errorf("repoUpdateJobs = %v, want %v", repoUpdateJobs, wantRepoJobs)
	}

	if sysTUI != wantSysTUI {
		t.Errorf("sysTUI = %v, want %v", sysTUI, wantSysTUI)
	}

	if repoUpdateTUI != wantRepoTUI {
		t.Errorf("repoUpdateTUI = %v, want %v", repoUpdateTUI, wantRepoTUI)
	}

	if sysNoTUI != wantSysNoTUI {
		t.Errorf("sysNoTUI = %v, want %v", sysNoTUI, wantSysNoTUI)
	}

	if repoUpdateNoTUI != wantRepoNoTUI {
		t.Errorf("repoUpdateNoTUI = %v, want %v", repoUpdateNoTUI, wantRepoNoTUI)
	}

	if sysLogFile != wantSysLogFile {
		t.Errorf("sysLogFile = %v, want %v", sysLogFile, wantSysLogFile)
	}

	if repoUpdateLogFile != wantRepoLogFile {
		t.Errorf("repoUpdateLogFile = %v, want %v", repoUpdateLogFile, wantRepoLogFile)
	}
}

func TestPrintPhaseErrors(t *testing.T) {
	tests := []struct {
		name   string
		errors []phaseError
		want   string
	}{
		{
			name:   "空のエラー一覧",
			errors: nil,
			want:   "",
		},
		{
			name: "1件のエラー",
			errors: []phaseError{
				{Name: "システム更新", Err: errors.New("apt failed")},
			},
			want: "└──",
		},
		{
			name: "複数エラー",
			errors: []phaseError{
				{Name: "システム更新", Err: errors.New("apt failed")},
				{Name: "リポジトリ同期", Err: errors.New("git timeout")},
			},
			want: "├──",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStderr(t, func() {
				printPhaseErrors(tt.errors)
			})

			if tt.want == "" {
				if output != "" {
					t.Errorf("printPhaseErrors() output = %q, want empty", output)
				}
			} else {
				if !strings.Contains(output, tt.want) {
					t.Errorf("printPhaseErrors() output = %q, want substring %q", output, tt.want)
				}
			}
		})
	}
}

func TestRunDaily_ConflictingTUIFlags(t *testing.T) {
	originalSysUpdate := runSysUpdateStep
	originalRepoUpdate := runRepoUpdateStep

	t.Cleanup(func() {
		runSysUpdateStep = originalSysUpdate
		runRepoUpdateStep = originalRepoUpdate
	})

	testutil.SetTestHome(t, t.TempDir())

	runSysUpdateStep = func(*cobra.Command, []string) error {
		t.Fatal("sys_update should not be called with conflicting flags")
		return nil
	}

	runRepoUpdateStep = func(*cobra.Command, []string) error {
		t.Fatal("repo_update should not be called with conflicting flags")
		return nil
	}

	cmd := &cobra.Command{Use: "run"}
	cmd.Flags().BoolVarP(&runDryRun, "dry-run", "n", false, "")
	cmd.Flags().IntVarP(&runJobs, "jobs", "j", 0, "")
	cmd.Flags().BoolVar(&runTUI, "tui", false, "")
	cmd.Flags().BoolVar(&runNoTUI, "no-tui", false, "")

	if err := cmd.Flags().Set("tui", "true"); err != nil {
		t.Fatal(err)
	}

	if err := cmd.Flags().Set("no-tui", "true"); err != nil {
		t.Fatal(err)
	}

	err := runDaily(cmd, nil)
	if err == nil {
		t.Fatal("runDaily() should return error for conflicting --tui and --no-tui")
	}

	if !strings.Contains(err.Error(), "--tui と --no-tui は同時指定できません") {
		t.Errorf("runDaily() error = %q, want substring about conflicting flags", err.Error())
	}
}
