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

func TestPnpmUpdater_Name(t *testing.T) {
	t.Parallel()

	p := &PnpmUpdater{}
	assert.Equal(t, "pnpm", p.Name())
}

func TestPnpmUpdater_DisplayName(t *testing.T) {
	t.Parallel()

	p := &PnpmUpdater{}
	assert.Equal(t, "pnpm (Node.js グローバルパッケージ)", p.DisplayName())
}

func TestPnpmUpdater_Configure(t *testing.T) {
	t.Parallel()

	p := &PnpmUpdater{}
	err := p.Configure(config.ManagerConfig{"dummy": true})
	assert.NoError(t, err)
}

func TestPnpmUpdater_parseOutdatedJSON(t *testing.T) {
	tests := []struct {
		name        string
		output      []byte
		want        map[string]PackageInfo
		expectErr   bool
		errContains string
	}{
		{
			name:      "空出力",
			output:    nil,
			want:      map[string]PackageInfo{},
			expectErr: false,
		},
		{
			name:        "不正なJSONはエラー",
			output:      []byte("{invalid"),
			expectErr:   true,
			errContains: "JSON の解析に失敗",
		},
		{
			name: "配列形式の出力",
			output: []byte(`[
  {"name":"typescript","current":"5.1.0","latest":"5.2.0"},
  {"packageName":"@scope/pkg","current":"1.0.0","wanted":"1.1.0"}
]`),
			want: map[string]PackageInfo{
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
		{
			name: "オブジェクト形式の出力",
			output: []byte(`{
  "eslint": {"current":"8.0.0","latest":"9.0.0"},
  "pnpm": {"current":"9.0.0","wanted":"9.1.0"}
}`),
			want: map[string]PackageInfo{
				"eslint": {
					Name:           "eslint",
					CurrentVersion: "8.0.0",
					NewVersion:     "9.0.0",
				},
				"pnpm": {
					Name:           "pnpm",
					CurrentVersion: "9.0.0",
					NewVersion:     "9.1.0",
				},
			},
		},
		{
			name: "配列形式で名前が空の要素はスキップ",
			output: []byte(`[
  {"name":"", "packageName":"", "current":"1.0.0", "latest":"2.0.0"}
]`),
			want: map[string]PackageInfo{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := &PnpmUpdater{}
			got, err := p.parseOutdatedJSON(tt.output)

			if tt.expectErr {
				assert.Error(t, err)

				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}

				return
			}

			assert.NoError(t, err)
			assert.Len(t, got, len(tt.want))

			gotMap := make(map[string]PackageInfo, len(got))
			for _, pkg := range got {
				gotMap[pkg.Name] = pkg
			}

			for name, wantPkg := range tt.want {
				gotPkg, ok := gotMap[name]
				assert.True(t, ok, "package %q が見つかりません", name)
				assert.Equal(t, wantPkg.CurrentVersion, gotPkg.CurrentVersion)
				assert.Equal(t, wantPkg.NewVersion, gotPkg.NewVersion)
			}
		})
	}
}

func TestPnpmUpdater_Check(t *testing.T) {
	tests := []struct {
		name          string
		mode          string
		wantCount     int
		expectErr     bool
		errContains   string
		wantFirstName string
	}{
		{
			name:          "更新候補あり（exit code 1）",
			mode:          "outdated_updates",
			wantCount:     1,
			wantFirstName: "typescript",
		},
		{
			name:      "更新候補なし",
			mode:      "outdated_none",
			wantCount: 0,
		},
		{
			name:        "不正JSONは解析エラー",
			mode:        "outdated_invalid_json",
			expectErr:   true,
			errContains: "出力解析に失敗",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakePnpmCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_PNPM_MODE", tc.mode)

			p := &PnpmUpdater{}
			got, err := p.Check(context.Background())

			if tc.expectErr {
				if assert.Error(t, err) && tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.wantCount, got.AvailableUpdates)

			if tc.wantFirstName != "" {
				assert.NotEmpty(t, got.Packages)
				assert.Equal(t, tc.wantFirstName, got.Packages[0].Name)
			}
		})
	}
}

func TestPnpmUpdater_Update(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		opts        UpdateOptions
		expectErr   bool
		errContains string
		wantUpdated int
		wantDryRun  bool
	}{
		{
			name:        "DryRunは更新せず計画のみ返す",
			mode:        "outdated_updates",
			opts:        UpdateOptions{DryRun: true},
			wantDryRun:  true,
			wantUpdated: 0,
		},
		{
			name:        "通常更新成功",
			mode:        "outdated_updates",
			opts:        UpdateOptions{},
			wantUpdated: 1,
		},
		{
			name:        "事前チェック失敗",
			mode:        "outdated_invalid_json",
			opts:        UpdateOptions{},
			expectErr:   true,
			errContains: "出力解析に失敗",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakePnpmCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_PNPM_MODE", tc.mode)

			p := &PnpmUpdater{}
			got, err := p.Update(context.Background(), tc.opts)

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

			assert.Contains(t, got.Message, "更新しました")
		})
	}
}

func writeFakePnpmCommand(t *testing.T, dir string) {
	t.Helper()

	var (
		fileName string
		content  string
	)

	if runtime.GOOS == "windows" {
		fileName = "pnpm.cmd"
		content = `@echo off
set subcmd=%1

if "%subcmd%"=="outdated" (
  if "%DEVSYNC_TEST_PNPM_MODE%"=="outdated_updates" (
    echo [{"name":"typescript","current":"5.1.0","latest":"5.2.0"}]
    exit /b 1
  )
  if "%DEVSYNC_TEST_PNPM_MODE%"=="outdated_none" (
    echo []
    exit /b 0
  )
  if "%DEVSYNC_TEST_PNPM_MODE%"=="outdated_invalid_json" (
    echo {invalid
    exit /b 1
  )
  if "%DEVSYNC_TEST_PNPM_MODE%"=="outdated_command_error" (
    >&2 echo fatal error
    exit /b 2
  )
  if "%DEVSYNC_TEST_PNPM_MODE%"=="update_fail" (
    echo [{"name":"typescript","current":"5.1.0","latest":"5.2.0"}]
    exit /b 1
  )
)

if "%subcmd%"=="update" (
  if "%DEVSYNC_TEST_PNPM_MODE%"=="update_fail" (
    >&2 echo update failed
    exit /b 1
  )
  echo updated
  exit /b 0
)

echo []
exit /b 0
`
	} else {
		fileName = "pnpm"
		content = `#!/bin/sh
subcmd="$1"
mode="${DEVSYNC_TEST_PNPM_MODE}"

if [ "${subcmd}" = "outdated" ]; then
  if [ "${mode}" = "outdated_updates" ]; then
    echo '[{"name":"typescript","current":"5.1.0","latest":"5.2.0"}]'
    exit 1
  fi
  if [ "${mode}" = "outdated_none" ]; then
    echo '[]'
    exit 0
  fi
  if [ "${mode}" = "outdated_invalid_json" ]; then
    echo '{invalid'
    exit 1
  fi
  if [ "${mode}" = "outdated_command_error" ]; then
    echo 'fatal error' 1>&2
    exit 2
  fi
  if [ "${mode}" = "update_fail" ]; then
    echo '[{"name":"typescript","current":"5.1.0","latest":"5.2.0"}]'
    exit 1
  fi
fi

if [ "${subcmd}" = "update" ]; then
  if [ "${mode}" = "update_fail" ]; then
    echo 'update failed' 1>&2
    exit 1
  fi
  echo 'updated'
  exit 0
fi

echo '[]'
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
}
