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

func TestGemUpdater_Name(t *testing.T) {
	g := &GemUpdater{}
	assert.Equal(t, "gem", g.Name())
}

func TestGemUpdater_DisplayName(t *testing.T) {
	g := &GemUpdater{}
	assert.Equal(t, "gem (Ruby Gems)", g.DisplayName())
}

func TestGemUpdater_Configure(t *testing.T) {
	g := &GemUpdater{}
	err := g.Configure(config.ManagerConfig{"dummy": true})
	assert.NoError(t, err)
}

func TestGemUpdater_parseOutdatedOutput(t *testing.T) {
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
			name: "通常形式",
			output: `rake (13.1.0 < 13.2.1)
rubocop (1.65.0 < 1.69.1)
`,
			want: []PackageInfo{
				{Name: "rake", CurrentVersion: "13.1.0", NewVersion: "13.2.1"},
				{Name: "rubocop", CurrentVersion: "1.65.0", NewVersion: "1.69.1"},
			},
		},
		{
			name: "current側が複数候補",
			output: `foo (1.0.0, 1.1.0 < 2.0.0)
`,
			want: []PackageInfo{
				{Name: "foo", CurrentVersion: "1.0.0", NewVersion: "2.0.0"},
			},
		},
		{
			name: "defaultラベル付き",
			output: `bundler (default: 2.5.0 < 2.5.12)
`,
			want: []PackageInfo{
				{Name: "bundler", CurrentVersion: "2.5.0", NewVersion: "2.5.12"},
			},
		},
		{
			name: "不正な行は無視",
			output: `invalid line
pkg (1.0.0)
`,
			want: []PackageInfo{},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := &GemUpdater{}
			got := g.parseOutdatedOutput(tc.output)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGemUpdater_Check(t *testing.T) {
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
			name:        "check失敗",
			mode:        "check_error",
			wantUpdates: 0,
			wantErr:     true,
			errContains: "gem outdated の実行に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeGemCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_GEM_MODE", tc.mode)

			g := &GemUpdater{}
			got, err := g.Check(context.Background())

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

func TestGemUpdater_Update(t *testing.T) {
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
			msgContains: "更新可能なパッケージはありません",
		},
		{
			name:        "更新成功",
			mode:        "updates",
			opts:        UpdateOptions{},
			wantUpdated: 2,
			wantErr:     false,
			msgContains: "2 件の gem パッケージを更新しました",
		},
		{
			name:        "更新失敗",
			mode:        "update_error",
			opts:        UpdateOptions{},
			wantUpdated: 0,
			wantErr:     true,
			errContains: "gem update に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeGemCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_GEM_MODE", tc.mode)

			g := &GemUpdater{}
			got, err := g.Update(context.Background(), tc.opts)

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

func writeFakeGemCommand(t *testing.T, dir string) {
	t.Helper()

	var (
		fileName string
		content  string
	)

	if runtime.GOOS == "windows" {
		fileName = "gem.cmd"
		content = `@echo off
set mode=%DEVSYNC_TEST_GEM_MODE%
if "%1"=="outdated" goto outdated
if "%1"=="update" goto update
>&2 echo invalid args
exit /b 1
:outdated
if "%mode%"=="check_error" (
  >&2 echo gem outdated failed
  exit /b 1
)
if "%mode%"=="none" (
  exit /b 0
)
echo rake ^(13.1.0 ^< 13.2.1^)
echo rubocop ^(1.65.0 ^< 1.69.1^)
exit /b 0
:update
if "%mode%"=="update_error" (
  >&2 echo gem update failed
  exit /b 1
)
exit /b 0
`
	} else {
		fileName = "gem"
		content = `#!/bin/sh
mode="${DEVSYNC_TEST_GEM_MODE}"
if [ "$1" = "outdated" ]; then
  if [ "${mode}" = "check_error" ]; then
    echo "gem outdated failed" 1>&2
    exit 1
  fi
  if [ "${mode}" = "none" ]; then
    exit 0
  fi
  echo "rake (13.1.0 < 13.2.1)"
  echo "rubocop (1.65.0 < 1.69.1)"
  exit 0
fi
if [ "$1" = "update" ]; then
  if [ "${mode}" = "update_error" ]; then
    echo "gem update failed" 1>&2
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
		t.Fatalf("fake gem command write failed: %v", err)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(fullPath, 0o755); err != nil {
			t.Fatalf("fake gem command chmod failed: %v", err)
		}
	}
}
