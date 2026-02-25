package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scottlz0310/dsx/internal/testutil"
)

// setupEmptyConfig はテスト用の空設定ファイルを作成する。
// マネージャが0件なので sys update は即座に終了する。
func setupEmptyConfig(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "dsx")

	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("config dir creation failed: %v", err)
	}

	configBody := `version: 1
control:
  concurrency: 1
  timeout: "1m"
sys:
  enable: []
  managers: {}
repo:
  root: ""
`

	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configBody), 0o644); err != nil {
		t.Fatalf("config file write failed: %v", err)
	}

	testutil.SetTestHome(t, home)

	return home
}

// resetGlobalFlags はテスト間でグローバルフラグ変数を初期値にリセットする。
func resetGlobalFlags(t *testing.T) {
	t.Helper()

	// sys update のグローバル変数
	sysDryRun = false
	sysVerbose = false
	sysJobs = 0
	sysTimeout = "10m"
	sysTUI = false
	sysNoTUI = false

	// repo update のグローバル変数
	repoRootOverride = ""
	repoUpdateJobs = 0
	repoUpdateDryRun = false
	repoUpdateSubmodules = false
	repoUpdateNoSubmodule = false
	repoUpdateTUI = false
	repoUpdateNoTUI = false
}

// executeRootCommand は rootCmd にコマンドライン引数を設定して実行する。
// stdout / stderr をキャプチャし、エラーを返す。
func executeRootCommand(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	resetGlobalFlags(t)

	var cmdErr error

	stderr = captureStderr(t, func() {
		stdout = captureStdout(t, func() {
			rootCmd.SetArgs(args)
			cmdErr = rootCmd.Execute()
		})
	})

	return stdout, stderr, cmdErr
}

// --- sys update E2E テスト ---

func TestSysUpdate_NoManagers_ExitZero(t *testing.T) {
	setupEmptyConfig(t)

	stdout, _, err := executeRootCommand(t, "sys", "update")
	if err != nil {
		t.Fatalf("sys update with no managers should exit 0, got error: %v", err)
	}

	if !strings.Contains(stdout, "有効化されたマネージャがありません") {
		t.Fatalf("stdout should contain no-manager help, got: %q", stdout)
	}
}

func TestSysUpdate_TUIFlagOnNonTTY_FallbackWithWarning(t *testing.T) {
	setupEmptyConfig(t)

	// パイプ経由実行のため、stdout/stderr は非TTY
	_, stderr, err := executeRootCommand(t, "sys", "update", "--tui")
	if err != nil {
		t.Fatalf("sys update --tui on non-TTY should exit 0, got error: %v", err)
	}

	if !strings.Contains(stderr, "--tui は対話端末でのみ有効です") {
		t.Fatalf("stderr should contain TUI unavailable warning, got: %q", stderr)
	}
}

func TestSysUpdate_NoTUIFlag(t *testing.T) {
	setupEmptyConfig(t)

	stdout, stderr, err := executeRootCommand(t, "sys", "update", "--no-tui")
	if err != nil {
		t.Fatalf("sys update --no-tui should exit 0, got error: %v", err)
	}

	// TUI 無効で通常出力（マネージャ0件なのでヘルプが出る）
	if !strings.Contains(stdout, "有効化されたマネージャがありません") {
		t.Fatalf("stdout should contain no-manager help, got: %q", stdout)
	}

	// 非TTY警告が出ないこと（--no-tui 指定時は TUI を要求しない）
	if strings.Contains(stderr, "対話端末") {
		t.Fatalf("stderr should not contain TTY warning when --no-tui is used, got: %q", stderr)
	}
}

func TestSysUpdate_ConflictingFlags_Error(t *testing.T) {
	setupEmptyConfig(t)

	_, stderr, err := executeRootCommand(t, "sys", "update", "--tui", "--no-tui")
	if err == nil {
		t.Fatal("sys update --tui --no-tui should return error, got nil")
	}

	if !strings.Contains(err.Error(), "--tui と --no-tui は同時指定できません") {
		t.Fatalf("error should contain conflicting flag message, got: %q", err.Error())
	}

	_ = stderr
}

func TestSysUpdate_DryRun(t *testing.T) {
	setupEmptyConfig(t)

	stdout, _, err := executeRootCommand(t, "sys", "update", "--dry-run")
	if err != nil {
		t.Fatalf("sys update --dry-run should exit 0, got error: %v", err)
	}

	// マネージャ0件なので DryRun 表示はされないが、正常終了すること
	if !strings.Contains(stdout, "有効化されたマネージャがありません") {
		t.Fatalf("stdout should contain no-manager help, got: %q", stdout)
	}
}

// --- repo update E2E テスト ---

func TestRepoUpdate_NoRepos_ExitZero(t *testing.T) {
	home := setupEmptyConfig(t)

	// 空のリポジトリルートを作成
	emptyRoot := filepath.Join(home, "repos")
	if err := os.MkdirAll(emptyRoot, 0o755); err != nil {
		t.Fatalf("failed to create empty root: %v", err)
	}

	stdout, _, err := executeRootCommand(t, "repo", "update", "--root", emptyRoot)
	if err != nil {
		t.Fatalf("repo update with no repos should exit 0, got error: %v", err)
	}

	_ = stdout
}

func TestRepoUpdate_TUIFlagOnNonTTY_FallbackWithWarning(t *testing.T) {
	home := setupEmptyConfig(t)

	emptyRoot := filepath.Join(home, "repos")
	if err := os.MkdirAll(emptyRoot, 0o755); err != nil {
		t.Fatalf("failed to create empty root: %v", err)
	}

	_, stderr, err := executeRootCommand(t, "repo", "update", "--root", emptyRoot, "--tui")
	if err != nil {
		t.Fatalf("repo update --tui on non-TTY should exit 0, got error: %v", err)
	}

	if !strings.Contains(stderr, "--tui は対話端末でのみ有効です") {
		t.Fatalf("stderr should contain TUI unavailable warning, got: %q", stderr)
	}
}

func TestRepoUpdate_ConflictingFlags_Error(t *testing.T) {
	home := setupEmptyConfig(t)

	emptyRoot := filepath.Join(home, "repos")
	if err := os.MkdirAll(emptyRoot, 0o755); err != nil {
		t.Fatalf("failed to create empty root: %v", err)
	}

	_, _, err := executeRootCommand(t, "repo", "update", "--root", emptyRoot, "--tui", "--no-tui")
	if err == nil {
		t.Fatal("repo update --tui --no-tui should return error, got nil")
	}

	if !strings.Contains(err.Error(), "--tui と --no-tui は同時指定できません") {
		t.Fatalf("error should contain conflicting flag message, got: %q", err.Error())
	}
}
