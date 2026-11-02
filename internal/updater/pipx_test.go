package updater

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPipxUpdater_parsePipxListJSON(t *testing.T) {
	tests := []struct {
		name     string
		output   []byte
		expected map[string]PackageInfo
	}{
		{
			name:     "空の出力",
			output:   nil,
			expected: nil,
		},
		{
			name:     "不正なJSON",
			output:   []byte("{not-json"),
			expected: nil,
		},
		{
			name: "インストール済みパッケージあり",
			output: []byte(`{
  "venvs": {
    "httpie": { "metadata": { "main_package": { "package_version": "3.0.0" } } },
    "black": { "metadata": { "main_package": { "package_version": "23.1.0" } } }
  }
}`),
			expected: map[string]PackageInfo{
				"httpie": {Name: "httpie", CurrentVersion: "3.0.0", NewVersion: ""},
				"black":  {Name: "black", CurrentVersion: "23.1.0", NewVersion: ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PipxUpdater{}
			got := p.parsePipxListJSON(tt.output)

			if tt.expected == nil {
				assert.Empty(t, got)
				return
			}

			assert.Len(t, got, len(tt.expected))

			gotMap := make(map[string]PackageInfo, len(got))
			for _, pkg := range got {
				gotMap[pkg.Name] = pkg
			}

			for name, expectedPkg := range tt.expected {
				pkg, ok := gotMap[name]
				assert.True(t, ok, "package %q が見つかりません", name)
				assert.Equal(t, expectedPkg.CurrentVersion, pkg.CurrentVersion)
				assert.Equal(t, expectedPkg.NewVersion, pkg.NewVersion)
			}
		})
	}
}

func TestPipxUpdater_Check(t *testing.T) {
	testCases := []struct {
		name         string
		mode         string
		wantPackages int
		wantErr      bool
		errContains  string
	}{
		{
			name:         "パッケージあり",
			mode:         "updates",
			wantPackages: 2,
			wantErr:      false,
		},
		{
			name:         "パッケージなし",
			mode:         "none",
			wantPackages: 0,
			wantErr:      false,
		},
		{
			name:         "コマンド失敗",
			mode:         "check_error",
			wantPackages: 0,
			wantErr:      true,
			errContains:  "pipx list の実行に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakePipxCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DSX_TEST_PIPX_MODE", tc.mode)

			p := &PipxUpdater{}
			got, err := p.Check(context.Background())

			if tc.wantErr {
				assert.Error(t, err)

				if tc.errContains != "" && err != nil {
					assert.Contains(t, err.Error(), tc.errContains)
				}

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, 0, got.AvailableUpdates) // pipx は常に 0
			assert.Equal(t, tc.wantPackages, len(got.Packages))
		})
	}
}

func TestPipxUpdater_Update(t *testing.T) {
	testCases := []struct {
		name        string
		mode        string
		opts        UpdateOptions
		wantUpdated int
		wantErr     bool
		errContains string
		msgContains string
		// wantPackages は nil のとき検証しません。
		wantPackages []PackageInfo
		wantSuspect  string
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
			msgContains: "pipx でインストールされたパッケージがありません",
		},
		{
			name:        "版が変わらなければ更新なし（インストール済みの件数は数えない）",
			mode:        "updates",
			opts:        UpdateOptions{},
			wantUpdated: 0,
			wantErr:     false,
			msgContains: "更新なし",
		},
		{
			name:         "版が変わったパッケージだけを数える（新規パッケージは数えない）",
			mode:         "upgrade_changes",
			opts:         UpdateOptions{},
			wantUpdated:  1,
			wantErr:      false,
			msgContains:  "1 件のパッケージを更新しました",
			wantPackages: []PackageInfo{{Name: "ruff", CurrentVersion: "0.2.0", NewVersion: "0.2.1"}},
		},
		{
			name:        "すべて版が変われば全件を数える",
			mode:        "upgrade_changes_all",
			opts:        UpdateOptions{},
			wantUpdated: 2,
			wantErr:     false,
			msgContains: "2 件のパッケージを更新しました",
			wantPackages: []PackageInfo{
				{Name: "black", CurrentVersion: "24.1.0", NewVersion: "24.2.0"},
				{Name: "ruff", CurrentVersion: "0.2.0", NewVersion: "0.2.1"},
			},
		},
		{
			name:        "更新後の一覧を取得できなければ件数を作らず警告する",
			mode:        "after_list_error",
			opts:        UpdateOptions{},
			wantUpdated: 0,
			wantErr:     false,
			msgContains: "更新件数は確定できません",
			wantSuspect: "更新後の一覧を取得できず",
		},
		{
			name:        "更新後の一覧からパッケージを読み取れなければ件数を作らず警告する",
			mode:        "after_list_empty",
			opts:        UpdateOptions{},
			wantUpdated: 0,
			wantErr:     false,
			msgContains: "更新件数は確定できません",
			wantSuspect: "1 件も読み取れず",
		},
		{
			name:        "更新失敗",
			mode:        "update_error",
			opts:        UpdateOptions{},
			wantUpdated: 0,
			wantErr:     true,
			errContains: "pipx upgrade-all に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakePipxCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DSX_TEST_PIPX_MODE", tc.mode)

			p := &PipxUpdater{}
			got, err := p.Update(context.Background(), tc.opts)

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

			if tc.wantPackages != nil {
				assert.Equal(t, tc.wantPackages, got.Packages)
			}

			if tc.wantSuspect == "" {
				assert.Empty(t, got.Suspect)
			} else {
				assert.Contains(t, got.Suspect, tc.wantSuspect)
			}
		})
	}
}

func writeFakePipxCommand(t *testing.T, dir string) {
	t.Helper()

	var (
		fileName string
		content  string
	)

	if runtime.GOOS == "windows" {
		fileName = "pipx.cmd"
		content = `@echo off
set mode=%DSX_TEST_PIPX_MODE%
if "%1"=="list" goto dolist
if "%1"=="upgrade-all" goto doupgrade
echo invalid args 1>&2
exit /b 1
:dolist
if "%mode%"=="check_error" (
  echo pipx list failed 1>&2
  exit /b 1
)
if "%mode%"=="none" (
  echo {"venvs":{}}
  exit /b 0
)
if exist "%~dp0upgraded" goto listafter
echo {"venvs":{"black":{"metadata":{"main_package":{"package":"black","package_version":"24.1.0"}}},"ruff":{"metadata":{"main_package":{"package":"ruff","package_version":"0.2.0"}}}}}
exit /b 0
:listafter
if "%mode%"=="after_list_error" (
  echo pipx list failed 1>&2
  exit /b 1
)
if "%mode%"=="after_list_empty" (
  echo {"venvs":{}}
  exit /b 0
)
if "%mode%"=="upgrade_changes" (
  echo {"venvs":{"black":{"metadata":{"main_package":{"package":"black","package_version":"24.1.0"}}},"ruff":{"metadata":{"main_package":{"package":"ruff","package_version":"0.2.1"}}},"newtool":{"metadata":{"main_package":{"package":"newtool","package_version":"1.0.0"}}}}}
  exit /b 0
)
if "%mode%"=="upgrade_changes_all" (
  echo {"venvs":{"black":{"metadata":{"main_package":{"package":"black","package_version":"24.2.0"}}},"ruff":{"metadata":{"main_package":{"package":"ruff","package_version":"0.2.1"}}}}}
  exit /b 0
)
echo {"venvs":{"black":{"metadata":{"main_package":{"package":"black","package_version":"24.1.0"}}},"ruff":{"metadata":{"main_package":{"package":"ruff","package_version":"0.2.0"}}}}}
exit /b 0
:doupgrade
if "%mode%"=="update_error" (
  echo pipx upgrade-all failed 1>&2
  exit /b 1
)
type nul > "%~dp0upgraded"
exit /b 0
`
	} else {
		fileName = "pipx"
		content = `#!/bin/sh
mode="${DSX_TEST_PIPX_MODE}"
case "$1" in
  list)
    if [ "${mode}" = "check_error" ]; then
      echo "pipx list failed" 1>&2
      exit 1
    fi
    if [ "${mode}" = "none" ]; then
      echo '{"venvs":{}}'
      exit 0
    fi
    if [ -e "$(dirname "$0")/upgraded" ]; then
      if [ "${mode}" = "after_list_error" ]; then
        echo "pipx list failed" 1>&2
        exit 1
      fi
      if [ "${mode}" = "after_list_empty" ]; then
        echo '{"venvs":{}}'
        exit 0
      fi
      if [ "${mode}" = "upgrade_changes" ]; then
        echo '{"venvs":{"black":{"metadata":{"main_package":{"package":"black","package_version":"24.1.0"}}},"ruff":{"metadata":{"main_package":{"package":"ruff","package_version":"0.2.1"}}},"newtool":{"metadata":{"main_package":{"package":"newtool","package_version":"1.0.0"}}}}}'
        exit 0
      fi
      if [ "${mode}" = "upgrade_changes_all" ]; then
        echo '{"venvs":{"black":{"metadata":{"main_package":{"package":"black","package_version":"24.2.0"}}},"ruff":{"metadata":{"main_package":{"package":"ruff","package_version":"0.2.1"}}}}}'
        exit 0
      fi
    fi
    echo '{"venvs":{"black":{"metadata":{"main_package":{"package":"black","package_version":"24.1.0"}}},"ruff":{"metadata":{"main_package":{"package":"ruff","package_version":"0.2.0"}}}}}'
    exit 0
    ;;
  upgrade-all)
    if [ "${mode}" = "update_error" ]; then
      echo "pipx upgrade-all failed" 1>&2
      exit 1
    fi
    : > "$(dirname "$0")/upgraded"
    exit 0
    ;;
  *)
    echo "invalid args" 1>&2
    exit 1
    ;;
esac
`
	}

	fullPath := filepath.Join(dir, fileName)
	if err := os.WriteFile(fullPath, []byte(content), 0o755); err != nil {
		t.Fatalf("fake pipx command write failed: %v", err)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(fullPath, 0o755); err != nil {
			t.Fatalf("fake pipx command chmod failed: %v", err)
		}
	}
}
