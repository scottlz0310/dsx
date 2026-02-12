package updater

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNpmUpdater_parseOutdatedJSON(t *testing.T) {
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
			name: "更新可能パッケージあり",
			output: []byte(`{
  "typescript": { "current": "5.1.0", "wanted": "5.1.0", "latest": "5.2.0", "location": "/usr/local/lib" },
  "@scope/pkg": { "current": "1.0.0", "wanted": "1.0.1", "latest": "1.1.0", "location": "/usr/local/lib" }
}`),
			expected: map[string]PackageInfo{
				"typescript": {
					Name:           "typescript",
					CurrentVersion: "5.1.0",
					NewVersion:     "5.2.0",
				},
				"@scope/pkg": {
					Name:           "@scope/pkg",
					CurrentVersion: "1.0.0",
					NewVersion:     "1.1.0",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &NpmUpdater{}
			got := n.parseOutdatedJSON(tt.output)

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

func TestNpmUpdater_Check(t *testing.T) {
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
			errContains: "npm outdated の実行に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeNpmCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_NPM_MODE", tc.mode)

			n := &NpmUpdater{}
			got, err := n.Check(context.Background())

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

func TestNpmUpdater_Update(t *testing.T) {
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
			mode:        "update_error",
			opts:        UpdateOptions{},
			wantUpdated: 0,
			wantErr:     true,
			errContains: "npm update -g に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeNpmCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_NPM_MODE", tc.mode)

			n := &NpmUpdater{}
			got, err := n.Update(context.Background(), tc.opts)

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

// writeFakeNpmCommand はテスト用のフェイク npm コマンドを指定ディレクトリに作成します。
// 環境変数 DEVSYNC_TEST_NPM_MODE によって動作を切り替えます。
func writeFakeNpmCommand(t *testing.T, dir string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		script := `@echo off
set mode=%DEVSYNC_TEST_NPM_MODE%
if "%1"=="outdated" goto docheck
if "%1"=="update" goto doupdate
echo invalid args 1>&2
exit /b 1
:docheck
if "%mode%"=="check_error" (
  echo npm outdated failed 1>&2
  exit /b 1
)
if "%mode%"=="none" (
  echo {}
  exit /b 0
)
echo {"typescript":{"current":"5.0.0","wanted":"5.3.0","latest":"5.3.0","dependent":"global","location":""},"eslint":{"current":"8.40.0","wanted":"8.56.0","latest":"8.56.0","dependent":"global","location":""}}
exit /b 1
:doupdate
if "%mode%"=="update_error" (
  echo npm update failed 1>&2
  exit /b 1
)
exit /b 0
`
		path := filepath.Join(dir, "npm.cmd")
		err := os.WriteFile(path, []byte(script), 0o755)
		assert.NoError(t, err)
	} else {
		script := `#!/bin/sh
mode="${DEVSYNC_TEST_NPM_MODE}"
case "$1" in
  outdated)
    if [ "${mode}" = "check_error" ]; then
      echo "npm outdated failed" 1>&2
      exit 1
    fi
    if [ "${mode}" = "none" ]; then
      echo "{}"
      exit 0
    fi
    echo '{"typescript":{"current":"5.0.0","wanted":"5.3.0","latest":"5.3.0","dependent":"global","location":""},"eslint":{"current":"8.40.0","wanted":"8.56.0","latest":"8.56.0","dependent":"global","location":""}}'
    exit 1
    ;;
  update)
    if [ "${mode}" = "update_error" ]; then
      echo "npm update failed" 1>&2
      exit 1
    fi
    exit 0
    ;;
  *)
    echo "invalid args" 1>&2
    exit 1
    ;;
esac
`
		path := filepath.Join(dir, "npm")
		err := os.WriteFile(path, []byte(script), 0o755)
		assert.NoError(t, err)
		err = os.Chmod(path, 0o755)
		assert.NoError(t, err)
	}
}
