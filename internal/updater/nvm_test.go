package updater

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/scottlz0310/devsync/internal/config"
	"github.com/scottlz0310/devsync/internal/testutil"
	"github.com/stretchr/testify/assert"
)

func TestNvmUpdater_Name(t *testing.T) {
	t.Parallel()

	n := &NvmUpdater{}
	assert.Equal(t, "nvm", n.Name())
}

func TestNvmUpdater_DisplayName(t *testing.T) {
	t.Parallel()

	n := &NvmUpdater{}
	assert.Equal(t, "nvm (Node.js バージョン管理)", n.DisplayName())
}

func TestNvmUpdater_Configure(t *testing.T) {
	t.Parallel()

	n := &NvmUpdater{}
	err := n.Configure(config.ManagerConfig{"dummy": true})
	assert.NoError(t, err)
}

func TestParseNvmCurrentVersion(t *testing.T) {
	testCases := []struct {
		name        string
		output      string
		want        string
		expectErr   bool
		errContains string
	}{
		{
			name:   "通常のv付きバージョン",
			output: "v20.11.1",
			want:   "20.11.1",
		},
		{
			name:   "追加テキストを含む出力",
			output: "v18.19.0 (Currently using 64-bit executable)",
			want:   "18.19.0",
		},
		{
			name:   "noneは未選択として扱う",
			output: "none",
			want:   "",
		},
		{
			name:   "systemは未選択として扱う",
			output: "system",
			want:   "",
		},
		{
			name:        "不正な形式はエラー",
			output:      "not-a-version",
			expectErr:   true,
			errContains: "バージョン形式を解釈できません",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseNvmCurrentVersion(tc.output)
			if tc.expectErr {
				if assert.Error(t, err) && tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseLatestNodeVersion(t *testing.T) {
	testCases := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "空出力",
			output: "",
			want:   "",
		},
		{
			name: "nvm ls-remote 形式",
			output: `
      v18.20.4   (LTS: Hydrogen)
      v20.17.0   (LTS: Iron)
      v22.11.0   (Latest LTS: Jod)
`,
			want: "22.11.0",
		},
		{
			name: "nvm list available 形式（Windows系）",
			output: `
|   CURRENT    |     LTS      |  OLD STABLE  | OLD UNSTABLE |
|    22.11.0   |    20.17.0   |   0.12.18    |   0.11.16    |
`,
			want: "22.11.0",
		},
		{
			name: "iojs行は除外",
			output: `
      iojs-v3.3.1
      v20.10.0
`,
			want: "20.10.0",
		},
		{
			name: "不正な行のみ",
			output: `
      stable
      latest
`,
			want: "",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := parseLatestNodeVersion(tc.output)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestIsSemverLess(t *testing.T) {
	testCases := []struct {
		name        string
		left        string
		right       string
		want        bool
		expectErr   bool
		errContains string
	}{
		{
			name:  "左が古い",
			left:  "20.10.0",
			right: "20.11.0",
			want:  true,
		},
		{
			name:  "同一バージョン",
			left:  "20.11.0",
			right: "20.11.0",
			want:  false,
		},
		{
			name:  "左が新しい",
			left:  "22.0.0",
			right: "20.11.0",
			want:  false,
		},
		{
			name:        "不正な形式はエラー",
			left:        "20.11",
			right:       "20.12.0",
			expectErr:   true,
			errContains: "不正な semver 形式",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := isSemverLess(tc.left, tc.right)
			if tc.expectErr {
				assert.Error(t, err)

				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestNvmUpdater_Check(t *testing.T) {
	testCases := []struct {
		name        string
		mode        string
		wantCount   int
		expectErr   bool
		errContains string
	}{
		{
			name:      "更新候補あり",
			mode:      "check_update",
			wantCount: 1,
		},
		{
			name:      "最新状態",
			mode:      "check_none",
			wantCount: 0,
		},
		{
			name:      "current=none は導入提案",
			mode:      "current_none",
			wantCount: 1,
		},
		{
			name:      "list available 失敗時は ls-remote にフォールバック",
			mode:      "list_available_fail_remote_fallback",
			wantCount: 1,
		},
		{
			name:        "current が不正形式ならエラー",
			mode:        "invalid_current",
			expectErr:   true,
			errContains: "出力解析に失敗",
		},
		{
			name:        "最新バージョンの取得失敗",
			mode:        "latest_fail",
			expectErr:   true,
			errContains: "最新 Node.js バージョンの取得に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeNvmCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))

			if runtime.GOOS != "windows" {
				t.Setenv("NVM_DIR", fakeDir)
			}

			t.Setenv("DEVSYNC_TEST_NVM_MODE", tc.mode)

			n := &NvmUpdater{}
			got, err := n.Check(context.Background())

			if tc.expectErr {
				if assert.Error(t, err) && tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.wantCount, got.AvailableUpdates)
		})
	}
}

func TestNvmUpdater_Update(t *testing.T) {
	testCases := []struct {
		name        string
		mode        string
		opts        UpdateOptions
		expectErr   bool
		errContains string
		wantUpdated int
		wantDryRun  bool
	}{
		{
			name:       "DryRunは計画のみ返す",
			mode:       "check_update",
			opts:       UpdateOptions{DryRun: true},
			wantDryRun: true,
		},
		{
			name:        "通常更新成功",
			mode:        "check_update",
			opts:        UpdateOptions{},
			wantUpdated: 1,
		},
		{
			name:        "事前チェック失敗",
			mode:        "latest_fail",
			opts:        UpdateOptions{},
			expectErr:   true,
			errContains: "最新 Node.js バージョンの取得に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeNvmCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))

			if runtime.GOOS != "windows" {
				t.Setenv("NVM_DIR", fakeDir)
			}

			t.Setenv("DEVSYNC_TEST_NVM_MODE", tc.mode)

			n := &NvmUpdater{}
			got, err := n.Update(context.Background(), tc.opts)

			if tc.expectErr {
				if assert.Error(t, err) && tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.wantUpdated, got.UpdatedCount)

			if tc.wantDryRun {
				assert.Contains(t, got.Message, "DryRun")
				return
			}

			assert.Contains(t, got.Message, "インストールしました")
		})
	}
}

func TestBuildUnixNvmCommand(t *testing.T) {
	t.Parallel()

	got := buildUnixNvmCommand("/home/user/.nvm/nvm.sh", "install", "v20.11.1")
	want := ". '/home/user/.nvm/nvm.sh' >/dev/null 2>&1 && nvm 'install' 'v20.11.1'"
	assert.Equal(t, want, got)
}

func TestQuotePosixShellArg(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "空文字",
			value: "",
			want:  "''",
		},
		{
			name:  "通常文字列",
			value: "nvm.sh",
			want:  "'nvm.sh'",
		},
		{
			name:  "シングルクォートを含む",
			value: "O'Brien",
			want:  `'O'\''Brien'`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := quotePosixShellArg(tc.value)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResolveNvmScriptPath(t *testing.T) {
	testCases := []struct {
		name      string
		setup     func(t *testing.T) string
		expectErr bool
	}{
		{
			name: "NVM_DIR が優先される",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				scriptPath := filepath.Join(dir, "nvm.sh")

				testutil.SetTestHome(t, t.TempDir())
				t.Setenv("NVM_DIR", dir)

				writeNvmScriptForTest(t, scriptPath)

				return scriptPath
			},
		},
		{
			name: "HOME/.nvm にフォールバック",
			setup: func(t *testing.T) string {
				home := t.TempDir()
				testutil.SetTestHome(t, home)
				t.Setenv("NVM_DIR", "")

				scriptPath := filepath.Join(home, ".nvm", "nvm.sh")
				writeNvmScriptForTest(t, scriptPath)

				return scriptPath
			},
		},
		{
			name: "見つからない場合はエラー",
			setup: func(t *testing.T) string {
				testutil.SetTestHome(t, t.TempDir())
				t.Setenv("NVM_DIR", "")

				return ""
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			want := tc.setup(t)

			got, err := resolveNvmScriptPath()
			if tc.expectErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func writeNvmScriptForTest(t *testing.T, scriptPath string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("failed to create nvm script dir: %v", err)
	}

	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to write nvm script: %v", err)
	}
}

func writeFakeNvmCommand(t *testing.T, dir string) {
	t.Helper()

	var (
		fileName string
		content  string
	)

	if runtime.GOOS == "windows" {
		fileName = "nvm.cmd"
		content = `@echo off
set cmd1=%1
set cmd2=%2
set mode=%DEVSYNC_TEST_NVM_MODE%

if "%cmd1%"=="current" (
  if "%mode%"=="invalid_current" (
    echo not-a-version
    exit /b 0
  )
  if "%mode%"=="current_none" (
    echo none
    exit /b 0
  )
  if "%mode%"=="check_none" (
    echo v22.11.0
    exit /b 0
  )
  echo v20.10.0
  exit /b 0
)

if "%cmd1%"=="list" (
  if "%cmd2%"=="available" (
    if "%mode%"=="latest_fail" (
      >&2 echo list failed
      exit /b 1
    )
    if "%mode%"=="list_available_fail_remote_fallback" (
      >&2 echo list failed
      exit /b 1
    )
    if "%mode%"=="check_none" (
      echo ^|   CURRENT    ^|     LTS      ^|
      echo ^|    22.11.0   ^|    20.17.0   ^|
      exit /b 0
    )
    echo ^|   CURRENT    ^|     LTS      ^|
    echo ^|    22.11.0   ^|    20.17.0   ^|
    exit /b 0
  )
)

if "%cmd1%"=="ls-remote" (
  if "%mode%"=="latest_fail" (
    >&2 echo remote failed
    exit /b 1
  )
  echo v22.11.0
  exit /b 0
)

if "%cmd1%"=="install" (
  if "%mode%"=="install_fail" (
    >&2 echo install failed
    exit /b 1
  )
  echo installed %2
  exit /b 0
)

exit /b 0
`
	} else {
		fileName = "nvm.sh"
		content = `#!/bin/sh
nvm() {
  cmd1="$1"
  cmd2="$2"
  mode="${DEVSYNC_TEST_NVM_MODE}"

  if [ "${cmd1}" = "current" ]; then
    if [ "${mode}" = "invalid_current" ]; then
      echo "not-a-version"
      return 0
    fi
    if [ "${mode}" = "current_none" ]; then
      echo "none"
      return 0
    fi
    if [ "${mode}" = "check_none" ]; then
      echo "v22.11.0"
      return 0
    fi
    echo "v20.10.0"
    return 0
  fi

  if [ "${cmd1}" = "list" ] && [ "${cmd2}" = "available" ]; then
    if [ "${mode}" = "latest_fail" ] || [ "${mode}" = "list_available_fail_remote_fallback" ]; then
      echo "list failed" 1>&2
      return 1
    fi
    echo "|   CURRENT    |     LTS      |"
    echo "|    22.11.0   |    20.17.0   |"
    return 0
  fi

  if [ "${cmd1}" = "ls-remote" ]; then
    if [ "${mode}" = "latest_fail" ]; then
      echo "remote failed" 1>&2
      return 1
    fi
    echo "v22.11.0"
    return 0
  fi

  if [ "${cmd1}" = "install" ]; then
    if [ "${mode}" = "install_fail" ]; then
      echo "install failed" 1>&2
      return 1
    fi
    echo "installed $2"
    return 0
  fi

  return 0
}
`
	}

	fullPath := filepath.Join(dir, fileName)
	if writeErr := os.WriteFile(fullPath, []byte(content), 0o755); writeErr != nil {
		t.Fatalf("fake command write failed: %v", writeErr)
	}

	if runtime.GOOS != "windows" {
		if chmodErr := os.Chmod(fullPath, 0o755); chmodErr != nil {
			t.Fatalf("fake command chmod failed: %v", chmodErr)
		}
	}
}
