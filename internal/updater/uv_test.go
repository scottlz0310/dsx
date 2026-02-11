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

func TestUVUpdater_Name(t *testing.T) {
	u := &UVUpdater{}
	assert.Equal(t, "uv", u.Name())
}

func TestUVUpdater_DisplayName(t *testing.T) {
	u := &UVUpdater{}
	assert.Equal(t, "uv tool (Python CLI ツール)", u.DisplayName())
}

func TestUVUpdater_Configure(t *testing.T) {
	u := &UVUpdater{}
	err := u.Configure(config.ManagerConfig{"dummy": true})
	assert.NoError(t, err)
}

func TestUVUpdater_parseToolListOutput(t *testing.T) {
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
			name:   "未インストールメッセージは対象外",
			output: "No tools installed\n",
			want:   []PackageInfo{},
		},
		{
			name: "通常の一覧",
			output: `ruff v0.6.2
- ruff
httpie v3.2.2
- http
`,
			want: []PackageInfo{
				{Name: "ruff", CurrentVersion: "0.6.2"},
				{Name: "httpie", CurrentVersion: "3.2.2"},
			},
		},
		{
			name: "バージョンなし行も名前だけ保持",
			output: `custom-tool
`,
			want: []PackageInfo{
				{Name: "custom-tool"},
			},
		},
		{
			name: "コロンや括弧を含む形式",
			output: `black v24.10.0:
pkg 1.2.3 (/tmp/path)
`,
			want: []PackageInfo{
				{Name: "black", CurrentVersion: "24.10.0"},
				{Name: "pkg", CurrentVersion: "1.2.3"},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			u := &UVUpdater{}
			got := u.parseToolListOutput(tc.output)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestUVUpdater_Check(t *testing.T) {
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
			wantUpdates: 0,
			wantErr:     false,
		},
		{
			name:        "未インストール",
			mode:        "none",
			wantUpdates: 0,
			wantErr:     false,
		},
		{
			name:        "check失敗",
			mode:        "list_error",
			wantUpdates: 0,
			wantErr:     true,
			errContains: "uv tool list の実行に失敗",
		},
		{
			name:        "check失敗時にstderrを含む",
			mode:        "list_error",
			wantUpdates: 0,
			wantErr:     true,
			errContains: "uv tool list stderr",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeUVCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_UV_MODE", tc.mode)

			u := &UVUpdater{}
			got, err := u.Check(context.Background())

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

func TestUVUpdater_Update(t *testing.T) {
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
			msgContains: "インストールされたツールがありません",
		},
		{
			name:        "更新成功",
			mode:        "updates",
			opts:        UpdateOptions{},
			wantUpdated: 2,
			wantErr:     false,
			msgContains: "2 件のツールを確認・更新しました",
		},
		{
			name:        "更新失敗",
			mode:        "update_error",
			opts:        UpdateOptions{},
			wantUpdated: 0,
			wantErr:     true,
			errContains: "uv tool upgrade --all に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeUVCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_UV_MODE", tc.mode)

			u := &UVUpdater{}
			got, err := u.Update(context.Background(), tc.opts)

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

func writeFakeUVCommand(t *testing.T, dir string) {
	t.Helper()

	var (
		fileName string
		content  string
	)

	if runtime.GOOS == "windows" {
		fileName = "uv.cmd"
		content = `@echo off
set mode=%DEVSYNC_TEST_UV_MODE%
if "%1"=="tool" if "%2"=="list" goto list
if "%1"=="tool" if "%2"=="upgrade" goto upgrade
>&2 echo invalid args
exit /b 1
:list
if "%mode%"=="list_error" (
  >&2 echo uv tool list stderr
  exit /b 1
)
if "%mode%"=="none" (
  echo No tools installed
  exit /b 0
)
echo ruff v0.6.2
echo - ruff
echo httpie v3.2.2
echo - http
exit /b 0
:upgrade
if "%mode%"=="update_error" (
  >&2 echo uv tool upgrade failed
  exit /b 1
)
exit /b 0
`
	} else {
		fileName = "uv"
		content = `#!/bin/sh
mode="${DEVSYNC_TEST_UV_MODE}"
if [ "$1" = "tool" ] && [ "$2" = "list" ]; then
  if [ "${mode}" = "list_error" ]; then
    echo "uv tool list stderr" 1>&2
    exit 1
  fi
  if [ "${mode}" = "none" ]; then
    echo "No tools installed"
    exit 0
  fi
  echo "ruff v0.6.2"
  echo "- ruff"
  echo "httpie v3.2.2"
  echo "- http"
  exit 0
fi
if [ "$1" = "tool" ] && [ "$2" = "upgrade" ]; then
  if [ "${mode}" = "update_error" ]; then
    echo "uv tool upgrade failed" 1>&2
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
		t.Fatalf("fake uv command write failed: %v", err)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(fullPath, 0o755); err != nil {
			t.Fatalf("fake uv command chmod failed: %v", err)
		}
	}
}
