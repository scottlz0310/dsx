package updater

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/scottlz0310/devsync/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestFlatpakUpdater_Name(t *testing.T) {
	f := &FlatpakUpdater{}
	assert.Equal(t, "flatpak", f.Name())
}

func TestFlatpakUpdater_DisplayName(t *testing.T) {
	f := &FlatpakUpdater{}
	assert.Equal(t, "Flatpak", f.DisplayName())
}

func TestFlatpakUpdater_Configure(t *testing.T) {
	testCases := []struct {
		name     string
		cfg      config.ManagerConfig
		wantUser bool
		describe string
	}{
		{
			name:     "nilの設定",
			cfg:      nil,
			wantUser: false,
			describe: "nil設定の場合はデフォルト値を維持",
		},
		{
			name:     "新キーuse_user=true",
			cfg:      config.ManagerConfig{"use_user": true},
			wantUser: true,
			describe: "新キーuse_userが有効化される",
		},
		{
			name:     "旧キーuser=true",
			cfg:      config.ManagerConfig{"user": true},
			wantUser: true,
			describe: "旧キーuserも後方互換で受け付ける",
		},
		{
			name:     "新旧キーが競合する場合はuse_userを優先",
			cfg:      config.ManagerConfig{"use_user": false, "user": true},
			wantUser: false,
			describe: "use_userが優先される",
		},
		{
			name:     "不正な型の値は無視",
			cfg:      config.ManagerConfig{"use_user": "true"},
			wantUser: false,
			describe: "bool以外は適用しない",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := &FlatpakUpdater{useUser: false}
			err := f.Configure(tc.cfg)

			assert.NoError(t, err)
			assert.Equal(t, tc.wantUser, f.useUser, tc.describe)
		})
	}
}

func TestFlatpakUpdater_buildCommandArgs(t *testing.T) {
	testCases := []struct {
		name    string
		useUser bool
		args    []string
		want    []string
	}{
		{
			name:    "ユーザー更新無効",
			useUser: false,
			args:    []string{"update", "-y"},
			want:    []string{"update", "-y"},
		},
		{
			name:    "ユーザー更新有効",
			useUser: true,
			args:    []string{"remote-ls", "--updates"},
			want:    []string{"--user", "remote-ls", "--updates"},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := &FlatpakUpdater{useUser: tc.useUser}
			got := f.buildCommandArgs(tc.args...)

			assert.Equal(t, tc.want, got)
		})
	}
}

func TestFlatpakUpdater_parseRemoteLSOutput(t *testing.T) {
	testCases := []struct {
		name   string
		output string
		want   []PackageInfo
	}{
		{
			name:   "空出力",
			output: "",
			want:   []PackageInfo{},
		},
		{
			name: "ヘッダーのみ",
			output: `Application ID  Version
`,
			want: []PackageInfo{},
		},
		{
			name: "1件のみ",
			output: `org.gnome.Calculator 44.0
`,
			want: []PackageInfo{
				{Name: "org.gnome.Calculator", NewVersion: "44.0"},
			},
		},
		{
			name: "複数件と空行を含む",
			output: `Application ID  Version
org.mozilla.firefox 122.0

org.gnome.TextEditor 45.1
`,
			want: []PackageInfo{
				{Name: "org.mozilla.firefox", NewVersion: "122.0"},
				{Name: "org.gnome.TextEditor", NewVersion: "45.1"},
			},
		},
		{
			name: "バージョン欠落時は名前のみ",
			output: `org.example.Tool
`,
			want: []PackageInfo{
				{Name: "org.example.Tool"},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := &FlatpakUpdater{}
			got := f.parseRemoteLSOutput(tc.output)

			assert.Equal(t, tc.want, got)
		})
	}
}

func TestFlatpakUpdater_Check(t *testing.T) {
	testCases := []struct {
		name        string
		mode        string
		wantUpdates int
		wantErr     bool
		errContains string
	}{
		{
			name:        "stderrに警告があってもstdoutのみをパース",
			mode:        "success_with_stderr",
			wantUpdates: 1,
			wantErr:     false,
		},
		{
			name:        "失敗時はstderrを含むエラーを返す",
			mode:        "failure_with_stderr",
			wantUpdates: 0,
			wantErr:     true,
			errContains: "fatal issue",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeFlatpakCommand(t, fakeDir)

			pathValue := fakeDir + string(os.PathListSeparator) + os.Getenv("PATH")
			t.Setenv("PATH", pathValue)
			t.Setenv("DEVSYNC_TEST_FLATPAK_MODE", tc.mode)

			f := &FlatpakUpdater{}
			got, err := f.Check(context.Background())

			if tc.wantErr {
				assert.Error(t, err)

				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.wantUpdates, got.AvailableUpdates)
		})
	}
}

func writeFakeFlatpakCommand(t *testing.T, dir string) {
	t.Helper()

	var (
		fileName string
		content  string
	)

	if runtime.GOOS == "windows" {
		fileName = "flatpak.cmd"
		content = `@echo off
setlocal enabledelayedexpansion
set "subcmd="
for %%a in (%*) do (
  if "%%a"=="remote-ls" set "subcmd=remote-ls"
  if "%%a"=="update" set "subcmd=update"
)
if "%DEVSYNC_TEST_FLATPAK_MODE%"=="success_with_stderr" (
  if "!subcmd!"=="remote-ls" (
    >&2 echo warning from stderr
    echo Application ID  Version
    echo org.example.App 1.2.3
    exit /b 0
  )
)
if "%DEVSYNC_TEST_FLATPAK_MODE%"=="failure_with_stderr" (
  if "!subcmd!"=="remote-ls" (
    >&2 echo fatal issue
    exit /b 1
  )
)
if "%DEVSYNC_TEST_FLATPAK_MODE%"=="updates" (
  if "!subcmd!"=="remote-ls" (
    echo org.mozilla.firefox 122.0
    echo org.gnome.Calculator 44.0
    exit /b 0
  )
  if "!subcmd!"=="update" (
    exit /b 0
  )
)
if "%DEVSYNC_TEST_FLATPAK_MODE%"=="update_error" (
  if "!subcmd!"=="remote-ls" (
    echo org.mozilla.firefox 122.0
    exit /b 0
  )
  if "!subcmd!"=="update" (
    >&2 echo update failed
    exit /b 1
  )
)
if "%DEVSYNC_TEST_FLATPAK_MODE%"=="none" (
  if "!subcmd!"=="remote-ls" (
    exit /b 0
  )
)
echo Application ID  Version
exit /b 0
`
	} else {
		fileName = "flatpak"
		content = `#!/bin/sh
mode="${DEVSYNC_TEST_FLATPAK_MODE}"
subcmd=""
for arg in "$@"; do
  case "$arg" in
    remote-ls) subcmd="remote-ls" ;;
    update) subcmd="update" ;;
  esac
done
if [ "${mode}" = "success_with_stderr" ] && [ "${subcmd}" = "remote-ls" ]; then
  echo "warning from stderr" 1>&2
  echo "Application ID  Version"
  echo "org.example.App 1.2.3"
  exit 0
fi
if [ "${mode}" = "failure_with_stderr" ] && [ "${subcmd}" = "remote-ls" ]; then
  echo "fatal issue" 1>&2
  exit 1
fi
if [ "${mode}" = "updates" ]; then
  if [ "${subcmd}" = "remote-ls" ]; then
    echo "org.mozilla.firefox 122.0"
    echo "org.gnome.Calculator 44.0"
    exit 0
  fi
  if [ "${subcmd}" = "update" ]; then
    exit 0
  fi
fi
if [ "${mode}" = "update_error" ]; then
  if [ "${subcmd}" = "remote-ls" ]; then
    echo "org.mozilla.firefox 122.0"
    exit 0
  fi
  if [ "${subcmd}" = "update" ]; then
    echo "update failed" 1>&2
    exit 1
  fi
fi
if [ "${mode}" = "none" ] && [ "${subcmd}" = "remote-ls" ]; then
  exit 0
fi
echo "Application ID  Version"
exit 0
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

	if _, statErr := os.Stat(fullPath); statErr != nil {
		t.Fatalf("fake command stat failed (%s): %v", fullPath, statErr)
	}
}

func TestFlatpakUpdater_Update(t *testing.T) {
	testCases := []struct {
		name         string
		mode         string
		opts         UpdateOptions
		wantUpdated  int
		wantMsg      string
		wantPkgCount int
		wantErr      bool
		errContains  string
	}{
		{
			name:         "DryRun",
			mode:         "updates",
			opts:         UpdateOptions{DryRun: true},
			wantUpdated:  0,
			wantMsg:      "2 件の Flatpak パッケージが更新可能です（DryRunモード）",
			wantPkgCount: 2,
			wantErr:      false,
		},
		{
			name:         "対象なし",
			mode:         "none",
			opts:         UpdateOptions{},
			wantUpdated:  0,
			wantMsg:      "すべての Flatpak パッケージは最新です",
			wantPkgCount: 0,
			wantErr:      false,
		},
		{
			name:         "更新成功",
			mode:         "updates",
			opts:         UpdateOptions{},
			wantUpdated:  2,
			wantMsg:      "2 件の Flatpak パッケージを更新しました",
			wantPkgCount: 2,
			wantErr:      false,
		},
		{
			name:        "更新失敗",
			mode:        "update_error",
			opts:        UpdateOptions{},
			wantErr:     true,
			errContains: "flatpak update に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeFlatpakCommand(t, fakeDir)

			pathValue := fakeDir + string(os.PathListSeparator) + os.Getenv("PATH")
			t.Setenv("PATH", pathValue)
			t.Setenv("DEVSYNC_TEST_FLATPAK_MODE", tc.mode)

			f := &FlatpakUpdater{}
			got, err := f.Update(context.Background(), tc.opts)

			if tc.wantErr {
				assert.Error(t, err)

				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.wantUpdated, got.UpdatedCount)
			assert.Equal(t, tc.wantMsg, got.Message)
			assert.Len(t, got.Packages, tc.wantPkgCount)
		})
	}
}
