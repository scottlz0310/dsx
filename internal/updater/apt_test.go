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

func TestAptUpdater_Name(t *testing.T) {
	apt := &AptUpdater{}
	assert.Equal(t, "apt", apt.Name())
}

func TestAptUpdater_DisplayName(t *testing.T) {
	apt := &AptUpdater{}
	assert.Equal(t, "APT (Debian/Ubuntu)", apt.DisplayName())
}

func TestAptUpdater_Configure(t *testing.T) {
	tests := []struct {
		name        string
		cfg         config.ManagerConfig
		expectSudo  bool
		description string
	}{
		{
			name:        "nilの設定",
			cfg:         nil,
			expectSudo:  false, // デフォルト値
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
		{
			name:        "不正な型の値",
			cfg:         config.ManagerConfig{"use_sudo": "true"}, // stringは無視される
			expectSudo:  false,
			description: "不正な型の値は無視される",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apt := &AptUpdater{useSudo: false}
			err := apt.Configure(tt.cfg)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectSudo, apt.useSudo, tt.description)
		})
	}
}

func TestAptUpdater_parseUpgradableList(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected []PackageInfo
	}{
		{
			name:     "空の出力",
			output:   "",
			expected: []PackageInfo{},
		},
		{
			name:     "Listingヘッダーのみ",
			output:   "Listing... Done",
			expected: []PackageInfo{},
		},
		{
			name: "単一のパッケージ",
			output: `Listing... Done
vim/jammy-updates 2:8.2.3995-1ubuntu2.11 amd64 [upgradable from: 2:8.2.3995-1ubuntu2.10]`,
			expected: []PackageInfo{
				{Name: "vim", NewVersion: "2:8.2.3995-1ubuntu2.11", CurrentVersion: "2:8.2.3995-1ubuntu2.10"},
			},
		},
		{
			name: "複数のパッケージ",
			output: `Listing... Done
vim/jammy-updates 2:8.2.3995-1ubuntu2.11 amd64 [upgradable from: 2:8.2.3995-1ubuntu2.10]
curl/jammy-security 7.81.0-1ubuntu1.14 amd64 [upgradable from: 7.81.0-1ubuntu1.13]
git/jammy-updates 1:2.34.1-1ubuntu1.10 amd64 [upgradable from: 1:2.34.1-1ubuntu1.9]`,
			expected: []PackageInfo{
				{Name: "vim", NewVersion: "2:8.2.3995-1ubuntu2.11", CurrentVersion: "2:8.2.3995-1ubuntu2.10"},
				{Name: "curl", NewVersion: "7.81.0-1ubuntu1.14", CurrentVersion: "7.81.0-1ubuntu1.13"},
				{Name: "git", NewVersion: "1:2.34.1-1ubuntu1.10", CurrentVersion: "1:2.34.1-1ubuntu1.9"},
			},
		},
		{
			name: "空行を含む",
			output: `Listing... Done

vim/jammy-updates 2:8.2.3995-1ubuntu2.11 amd64 [upgradable from: 2:8.2.3995-1ubuntu2.10]

curl/jammy-security 7.81.0-1ubuntu1.14 amd64 [upgradable from: 7.81.0-1ubuntu1.13]
`,
			expected: []PackageInfo{
				{Name: "vim", NewVersion: "2:8.2.3995-1ubuntu2.11", CurrentVersion: "2:8.2.3995-1ubuntu2.10"},
				{Name: "curl", NewVersion: "7.81.0-1ubuntu1.14", CurrentVersion: "7.81.0-1ubuntu1.13"},
			},
		},
		{
			name: "古いバージョン情報がない場合",
			output: `Listing... Done
vim/jammy-updates 2:8.2.3995-1ubuntu2.11 amd64`,
			expected: []PackageInfo{
				{Name: "vim", NewVersion: "2:8.2.3995-1ubuntu2.11", CurrentVersion: ""},
			},
		},
		{
			name: "パッケージ名にスラッシュがない場合（不正な形式）",
			output: `Listing... Done
invalid-line-without-space`,
			expected: []PackageInfo{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apt := &AptUpdater{}
			result := apt.parseUpgradableList(tt.output)

			assert.Len(t, result, len(tt.expected))

			for i, expected := range tt.expected {
				assert.Equal(t, expected.Name, result[i].Name, "Package name mismatch at index %d", i)
				assert.Equal(t, expected.NewVersion, result[i].NewVersion, "New version mismatch at index %d", i)
				assert.Equal(t, expected.CurrentVersion, result[i].CurrentVersion, "Current version mismatch at index %d", i)
			}
		})
	}
}

func TestAptUpdater_Check(t *testing.T) {
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
			name:        "apt update 失敗",
			mode:        "update_error",
			wantUpdates: 0,
			wantErr:     true,
			errContains: "apt update に失敗",
		},
		{
			name:        "apt list 失敗",
			mode:        "list_error",
			wantUpdates: 0,
			wantErr:     true,
			errContains: "更新可能パッケージの取得に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeAptCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_APT_MODE", tc.mode)

			a := &AptUpdater{useSudo: false}
			got, err := a.Check(context.Background())

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

func TestAptUpdater_Update(t *testing.T) {
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
			msgContains: "すべてのパッケージは最新です",
		},
		{
			name:        "更新成功",
			mode:        "updates",
			opts:        UpdateOptions{},
			wantUpdated: 2,
			wantErr:     false,
			msgContains: "2 件のパッケージを更新しました",
		},
		{
			name:        "更新失敗",
			mode:        "upgrade_error",
			opts:        UpdateOptions{},
			wantUpdated: 0,
			wantErr:     true,
			errContains: "apt upgrade に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeAptCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_APT_MODE", tc.mode)

			a := &AptUpdater{useSudo: false}
			got, err := a.Update(context.Background(), tc.opts)

			if tc.wantErr {
				assert.Error(t, err)

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

func writeFakeAptCommand(t *testing.T, dir string) {
	t.Helper()

	var (
		fileName string
		content  string
	)

	if runtime.GOOS == "windows" {
		fileName = "apt.cmd"
		content = `@echo off
set mode=%DEVSYNC_TEST_APT_MODE%
if "%1"=="update" goto doupdate
if "%1"=="list" goto dolist
if "%1"=="upgrade" goto doupgrade
echo invalid args 1>&2
exit /b 1
:doupdate
if "%mode%"=="update_error" (
  echo apt update failed 1>&2
  exit /b 1
)
exit /b 0
:dolist
if "%mode%"=="list_error" (
  echo apt list failed 1>&2
  exit /b 1
)
if "%mode%"=="none" (
  echo Listing... Done
  exit /b 0
)
echo Listing... Done
echo vim/stable 9.0.2 amd64 [upgradable from: 8.2.1]
echo curl/stable 8.5.0 amd64 [upgradable from: 7.88.1]
exit /b 0
:doupgrade
if "%mode%"=="upgrade_error" (
  echo apt upgrade failed 1>&2
  exit /b 1
)
exit /b 0
`
	} else {
		fileName = "apt"
		content = `#!/bin/sh
mode="${DEVSYNC_TEST_APT_MODE}"
if [ "$1" = "update" ]; then
  if [ "${mode}" = "update_error" ]; then
    echo "apt update failed" 1>&2
    exit 1
  fi
  exit 0
fi
if [ "$1" = "list" ]; then
  if [ "${mode}" = "list_error" ]; then
    echo "apt list failed" 1>&2
    exit 1
  fi
  if [ "${mode}" = "none" ]; then
    echo "Listing... Done"
    exit 0
  fi
  echo "Listing... Done"
  echo "vim/stable 9.0.2 amd64 [upgradable from: 8.2.1]"
  echo "curl/stable 8.5.0 amd64 [upgradable from: 7.88.1]"
  exit 0
fi
if [ "$1" = "upgrade" ]; then
  if [ "${mode}" = "upgrade_error" ]; then
    echo "apt upgrade failed" 1>&2
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
		t.Fatalf("fake apt command write failed: %v", err)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(fullPath, 0o755); err != nil {
			t.Fatalf("fake apt command chmod failed: %v", err)
		}
	}
}
