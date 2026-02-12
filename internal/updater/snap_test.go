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

func TestSnapUpdater_Name(t *testing.T) {
	snap := &SnapUpdater{}
	assert.Equal(t, "snap", snap.Name())
}

func TestSnapUpdater_DisplayName(t *testing.T) {
	snap := &SnapUpdater{}
	assert.Equal(t, "snap (Ubuntu Snap パッケージ)", snap.DisplayName())
}

func TestSnapUpdater_Configure(t *testing.T) {
	testCases := []struct {
		name        string
		cfg         config.ManagerConfig
		expectSudo  bool
		description string
	}{
		{
			name:        "nilの設定",
			cfg:         nil,
			expectSudo:  false,
			description: "nil設定の場合はデフォルトのまま",
		},
		{
			name:        "空の設定",
			cfg:         config.ManagerConfig{},
			expectSudo:  false,
			description: "空の設定の場合はデフォルトのまま",
		},
		{
			name:        "use_sudo=true",
			cfg:         config.ManagerConfig{"use_sudo": true},
			expectSudo:  true,
			description: "use_sudoがtrueの場合はsudoを使用",
		},
		{
			name:        "use_sudo=false",
			cfg:         config.ManagerConfig{"use_sudo": false},
			expectSudo:  false,
			description: "use_sudoがfalseの場合はsudoを使用しない",
		},
		{
			name:        "旧キーsudo=true",
			cfg:         config.ManagerConfig{"sudo": true},
			expectSudo:  true,
			description: "旧キーsudoも後方互換で受け付ける",
		},
		{
			name:        "use_sudoを優先",
			cfg:         config.ManagerConfig{"use_sudo": false, "sudo": true},
			expectSudo:  false,
			description: "新キーと旧キーが両方ある場合はuse_sudoを優先する",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			snap := &SnapUpdater{useSudo: false}
			err := snap.Configure(tc.cfg)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectSudo, snap.useSudo, tc.description)
		})
	}
}

func TestIsSnapdUnavailable(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name: "snapd unavailable",
			output: `snap    2.73+ubuntu25.10
snapd   unavailable
series  -`,
			want: true,
		},
		{
			name: "snapd available",
			output: `snap    2.73+ubuntu25.10
snapd   2.73+ubuntu25.10
series  16`,
			want: false,
		},
		{
			name:   "大文字小文字の揺れを吸収",
			output: "SNAPD UNAVAILABLE",
			want:   true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := isSnapdUnavailable(tc.output)
			if got != tc.want {
				t.Fatalf("isSnapdUnavailable() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSnapUpdater_Check(t *testing.T) {
	testCases := []struct {
		name        string
		mode        string
		wantUpdates int
		wantErr     bool
		errContains string
	}{
		{
			name:        "更新候補あり",
			mode:        "updates",
			wantUpdates: 2,
			wantErr:     false,
		},
		{
			name:        "更新なし",
			mode:        "none",
			wantUpdates: 0,
			wantErr:     false,
		},
		{
			name:        "コマンド失敗",
			mode:        "check_error",
			wantUpdates: 0,
			wantErr:     true,
			errContains: "snap refresh --list の実行に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeSnapCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_SNAP_MODE", tc.mode)

			s := &SnapUpdater{useSudo: false}
			got, err := s.Check(context.Background())

			if tc.wantErr {
				assert.Error(t, err)

				if tc.errContains != "" && err != nil {
					assert.Contains(t, err.Error(), tc.errContains)
				}

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.wantUpdates, got.AvailableUpdates)
		})
	}
}

func TestSnapUpdater_Update(t *testing.T) {
	testCases := []struct {
		name        string
		mode        string
		opts        UpdateOptions
		wantUpdated int
		wantErr     bool
		errContains string
		msgContains string
	}{
		{
			name:        "DryRunは更新せず計画表示",
			mode:        "updates",
			opts:        UpdateOptions{DryRun: true},
			wantUpdated: 0,
			wantErr:     false,
			msgContains: "DryRunモード",
		},
		{
			name:        "対象なし",
			mode:        "none",
			opts:        UpdateOptions{},
			wantUpdated: 0,
			wantErr:     false,
			msgContains: "すべてのスナップは最新です",
		},
		{
			name:        "更新成功",
			mode:        "updates",
			opts:        UpdateOptions{},
			wantUpdated: 2,
			wantErr:     false,
			msgContains: "2 件のスナップを更新しました",
		},
		{
			name:        "更新失敗",
			mode:        "update_error",
			opts:        UpdateOptions{},
			wantUpdated: 0,
			wantErr:     true,
			errContains: "snap refresh に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeSnapCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_SNAP_MODE", tc.mode)

			s := &SnapUpdater{useSudo: false}
			got, err := s.Update(context.Background(), tc.opts)

			if tc.wantErr {
				assert.Error(t, err)
				assert.NotNil(t, got)

				if tc.errContains != "" && err != nil {
					assert.Contains(t, err.Error(), tc.errContains)
				}

				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, got)
			assert.Equal(t, tc.wantUpdated, got.UpdatedCount)

			if tc.msgContains != "" {
				assert.Contains(t, got.Message, tc.msgContains)
			}
		})
	}
}

func writeFakeSnapCommand(t *testing.T, dir string) {
	t.Helper()

	var (
		fileName string
		content  string
	)

	if runtime.GOOS == "windows" {
		fileName = "snap.cmd"
		content = `@echo off
set mode=%DEVSYNC_TEST_SNAP_MODE%
if "%1"=="refresh" goto refresh
if "%1"=="version" goto version
>&2 echo invalid args
exit /b 1
:version
echo snap 2.61
echo snapd 2.61
echo series 16
exit /b 0
:refresh
if "%2"=="--list" goto refresh_list
rem refresh without --list = actual update
if "%mode%"=="update_error" (
  >&2 echo snap refresh failed
  exit /b 1
)
exit /b 0
:refresh_list
if "%mode%"=="check_error" (
  >&2 echo snap refresh --list failed
  exit /b 1
)
if "%mode%"=="none" (
  echo All snaps up to date.
  exit /b 1
)
echo Name    Version    Rev   Size   Publisher   Notes
echo firefox 130.0      4698  300MB  mozilla     -
echo core22  20240903   1663  77MB   canonical   base
exit /b 0
`
	} else {
		fileName = "snap"
		content = `#!/bin/sh
mode="${DEVSYNC_TEST_SNAP_MODE}"
if [ "$1" = "version" ]; then
  echo "snap 2.61"
  echo "snapd 2.61"
  echo "series 16"
  exit 0
fi
if [ "$1" = "refresh" ]; then
  if [ "$2" = "--list" ]; then
    if [ "${mode}" = "check_error" ]; then
      echo "snap refresh --list failed" 1>&2
      exit 1
    fi
    if [ "${mode}" = "none" ]; then
      echo "All snaps up to date."
      exit 1
    fi
    echo "Name    Version    Rev   Size   Publisher   Notes"
    echo "firefox 130.0      4698  300MB  mozilla     -"
    echo "core22  20240903   1663  77MB   canonical   base"
    exit 0
  fi
  # refresh without --list = actual update
  if [ "${mode}" = "update_error" ]; then
    echo "snap refresh failed" 1>&2
    exit 1
  fi
  exit 0
fi
echo "invalid args" 1>&2
exit 1
`
	}

	fullPath := filepath.Join(dir, fileName)
	if err := os.WriteFile(fullPath, []byte(content), 0o755); err != nil {
		t.Fatalf("fake snap command write failed: %v", err)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(fullPath, 0o755); err != nil {
			t.Fatalf("fake snap command chmod failed: %v", err)
		}
	}
}
