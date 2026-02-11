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

func TestRustupUpdater_Name(t *testing.T) {
	r := &RustupUpdater{}
	assert.Equal(t, "rustup", r.Name())
}

func TestRustupUpdater_DisplayName(t *testing.T) {
	r := &RustupUpdater{}
	assert.Equal(t, "rustup (Rust ツールチェーン)", r.DisplayName())
}

func TestRustupUpdater_Configure(t *testing.T) {
	r := &RustupUpdater{}
	err := r.Configure(config.ManagerConfig{"dummy": true})
	assert.NoError(t, err)
}

func TestRustupUpdater_parseCheckOutput(t *testing.T) {
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
			name: "更新なし",
			output: `stable-x86_64-unknown-linux-gnu - Up to date : 1.81.0
rustup - Up to date : 1.28.1
`,
			want: []PackageInfo{},
		},
		{
			name: "更新あり",
			output: `stable-x86_64-unknown-linux-gnu - Update available : 1.81.0 -> 1.82.0
rustup - Update available : 1.28.1 -> 1.29.0
`,
			want: []PackageInfo{
				{
					Name:           "stable-x86_64-unknown-linux-gnu",
					CurrentVersion: "1.81.0",
					NewVersion:     "1.82.0",
				},
				{
					Name:           "rustup",
					CurrentVersion: "1.28.1",
					NewVersion:     "1.29.0",
				},
			},
		},
		{
			name: "付加情報付きの更新行",
			output: `nightly-x86_64-unknown-linux-gnu - Update available : 2025-01-01 (abcd123) -> 2025-01-08 (efgh456)
`,
			want: []PackageInfo{
				{
					Name:           "nightly-x86_64-unknown-linux-gnu",
					CurrentVersion: "2025-01-01",
					NewVersion:     "2025-01-08",
				},
			},
		},
		{
			name: "不正な行は無視",
			output: `broken line
toolchain - Update available
`,
			want: []PackageInfo{},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := &RustupUpdater{}
			got := r.parseCheckOutput(tc.output)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRustupUpdater_Check(t *testing.T) {
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
			errContains: "rustup check の実行に失敗",
		},
		{
			name:        "check失敗時にstderrを含む",
			mode:        "check_error",
			wantUpdates: 0,
			wantErr:     true,
			errContains: "rustup check stderr",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeRustupCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_RUSTUP_MODE", tc.mode)

			r := &RustupUpdater{}
			got, err := r.Check(context.Background())

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

func TestRustupUpdater_Update(t *testing.T) {
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
			msgContains: "更新可能なツールチェーンはありません",
		},
		{
			name:        "更新成功",
			mode:        "updates",
			opts:        UpdateOptions{},
			wantUpdated: 2,
			wantErr:     false,
			msgContains: "2 件の Rust ツールチェーンを更新しました",
		},
		{
			name:        "更新失敗",
			mode:        "update_error",
			opts:        UpdateOptions{},
			wantUpdated: 0,
			wantErr:     true,
			errContains: "rustup update に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeRustupCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_RUSTUP_MODE", tc.mode)

			r := &RustupUpdater{}
			got, err := r.Update(context.Background(), tc.opts)

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

func writeFakeRustupCommand(t *testing.T, dir string) {
	t.Helper()

	var (
		fileName string
		content  string
	)

	if runtime.GOOS == "windows" {
		fileName = "rustup.cmd"
		content = `@echo off
set mode=%DEVSYNC_TEST_RUSTUP_MODE%
if "%1"=="check" goto check
if "%1"=="update" goto update
>&2 echo invalid args
exit /b 1
:check
if "%mode%"=="check_error" (
  >&2 echo rustup check stderr
  exit /b 1
)
if "%mode%"=="none" (
  echo stable-x86_64-unknown-linux-gnu - Up to date : 1.81.0
  echo rustup - Up to date : 1.28.1
  exit /b 0
)
echo stable-x86_64-unknown-linux-gnu - Update available : 1.81.0 -^> 1.82.0
echo rustup - Update available : 1.28.1 -^> 1.29.0
exit /b 0
:update
if "%mode%"=="update_error" (
  >&2 echo rustup update failed
  exit /b 1
)
exit /b 0
`
	} else {
		fileName = "rustup"
		content = `#!/bin/sh
mode="${DEVSYNC_TEST_RUSTUP_MODE}"
if [ "$1" = "check" ]; then
  if [ "${mode}" = "check_error" ]; then
    echo "rustup check stderr" 1>&2
    exit 1
  fi
  if [ "${mode}" = "none" ]; then
    echo "stable-x86_64-unknown-linux-gnu - Up to date : 1.81.0"
    echo "rustup - Up to date : 1.28.1"
    exit 0
  fi
  echo "stable-x86_64-unknown-linux-gnu - Update available : 1.81.0 -> 1.82.0"
  echo "rustup - Update available : 1.28.1 -> 1.29.0"
  exit 0
fi
if [ "$1" = "update" ]; then
  if [ "${mode}" = "update_error" ]; then
    echo "rustup update failed" 1>&2
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
		t.Fatalf("fake rustup command write failed: %v", err)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(fullPath, 0o755); err != nil {
			t.Fatalf("fake rustup command chmod failed: %v", err)
		}
	}
}
