package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/scottlz0310/devsync/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestFwupdmgrUpdater_Name(t *testing.T) {
	f := &FwupdmgrUpdater{}
	assert.Equal(t, "fwupdmgr", f.Name())
}

func TestFwupdmgrUpdater_DisplayName(t *testing.T) {
	f := &FwupdmgrUpdater{}
	assert.Equal(t, "fwupdmgr (Linux Firmware)", f.DisplayName())
}

func TestFwupdmgrUpdater_Configure(t *testing.T) {
	f := &FwupdmgrUpdater{}
	err := f.Configure(config.ManagerConfig{"dummy": true})
	assert.NoError(t, err)
}

func TestFwupdmgrUpdater_parseGetUpdatesJSON(t *testing.T) {
	testCases := []struct {
		name        string
		output      []byte
		want        []PackageInfo
		expectErr   bool
		errContains string
	}{
		{
			name:        "空出力はエラー",
			output:      nil,
			expectErr:   true,
			errContains: "出力が空",
		},
		{
			name:        "不正なJSONはエラー",
			output:      []byte("{invalid"),
			expectErr:   true,
			errContains: "JSON の解析に失敗",
		},
		{
			name:        "devicesキーがない場合はエラー",
			output:      []byte(`{"status":"ok"}`),
			expectErr:   true,
			errContains: "devices キーが見つかりません",
		},
		{
			name:        "devicesの型が不正な場合はエラー",
			output:      []byte(`{"devices":{"name":"not-array"}}`),
			expectErr:   true,
			errContains: "devices の型が不正",
		},
		{
			name: "更新対象1件",
			output: []byte(`{
  "Devices": [
    {
      "Name": "USB-C Dock",
      "CurrentVersion": "1.0.0",
      "Releases": [
        {"Version": "1.1.0"}
      ]
    }
  ]
}`),
			want: []PackageInfo{
				{
					Name:           "USB-C Dock",
					CurrentVersion: "1.0.0",
					NewVersion:     "1.1.0",
				},
			},
		},
		{
			name: "無効データはスキップ",
			output: []byte(`{
  "devices": [
    {"name":"NoRelease", "releases":[]},
    {"releases":[{"version":"2.0.0"}]},
    {"name":"NoVersion", "releases":[{}]}
  ]
}`),
			want: []PackageInfo{},
		},
		{
			name: "nameが空でもguidにフォールバック",
			output: []byte(`{
  "devices": [
    {
      "guid": "abcd-efgh",
      "version": "1.0",
      "releases": [{"version":"1.1"}]
    }
  ]
}`),
			want: []PackageInfo{
				{
					Name:           "abcd-efgh",
					CurrentVersion: "1.0",
					NewVersion:     "1.1",
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := &FwupdmgrUpdater{}
			got, err := f.parseGetUpdatesJSON(tc.output)

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

func TestIsNoFwupdmgrUpdatesMessage(t *testing.T) {
	testCases := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "no updatable devices",
			output: "No updatable devices",
			want:   true,
		},
		{
			name:   "no updates available",
			output: "There are no updates available",
			want:   true,
		},
		{
			name:   "no upgrades for",
			output: "No upgrades for device",
			want:   true,
		},
		{
			name:   "それ以外はfalse",
			output: "device update failed",
			want:   false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := isNoFwupdmgrUpdatesMessage(tc.output)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBuildCommandOutputErr(t *testing.T) {
	baseErr := errors.New("base error")

	t.Run("出力なしは元エラーを返す", func(t *testing.T) {
		t.Parallel()

		got := buildCommandOutputErr(baseErr, nil)
		assert.ErrorIs(t, got, baseErr)
		assert.Equal(t, "base error", got.Error())
	})

	t.Run("出力ありはメッセージを連結", func(t *testing.T) {
		t.Parallel()

		got := buildCommandOutputErr(baseErr, []byte("details"))
		assert.ErrorIs(t, got, baseErr)
		assert.Contains(t, got.Error(), "details")
	})
}

func TestCombineCommandOutputs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		stdout []byte
		stderr []byte
		want   []byte
	}{
		{
			name:   "stdout/stderrとも空",
			stdout: nil,
			stderr: nil,
			want:   nil,
		},
		{
			name:   "stderrのみ",
			stdout: nil,
			stderr: []byte("error details\n"),
			want:   []byte("error details"),
		},
		{
			name:   "stdoutのみ",
			stdout: []byte("json output\n"),
			stderr: nil,
			want:   []byte("json output"),
		},
		{
			name:   "stdout/stderr両方",
			stdout: []byte("json output\n"),
			stderr: []byte("warning message\n"),
			want:   []byte("json output\nwarning message"),
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := combineCommandOutputs(tc.stdout, tc.stderr)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestFwupdmgrUpdater_Check(t *testing.T) {
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
			wantUpdates: 1,
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
			errContains: "fwupdmgr get-updates の実行に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeFwupdmgrCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_FWUPDMGR_MODE", tc.mode)

			f := &FwupdmgrUpdater{}
			got, err := f.Check(context.Background())

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

func TestFwupdmgrUpdater_Update(t *testing.T) {
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
			name:        "DryRun",
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
			msgContains: "適用可能なファームウェア更新はありません",
		},
		{
			name:        "更新成功",
			mode:        "updates",
			opts:        UpdateOptions{},
			wantUpdated: 1,
			wantErr:     false,
			msgContains: "1 件のファームウェア更新を実行しました",
		},
		{
			name:        "更新失敗",
			mode:        "update_error",
			opts:        UpdateOptions{},
			wantUpdated: 0,
			wantErr:     true,
			errContains: "fwupdmgr update に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeFwupdmgrCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_FWUPDMGR_MODE", tc.mode)

			f := &FwupdmgrUpdater{}
			got, err := f.Update(context.Background(), tc.opts)

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

func writeFakeFwupdmgrCommand(t *testing.T, dir string) {
	t.Helper()

	var (
		fileName string
		content  string
	)

	if runtime.GOOS == "windows" {
		fileName = "fwupdmgr.cmd"
		content = `@echo off
set mode=%DEVSYNC_TEST_FWUPDMGR_MODE%
if "%1"=="get-updates" goto get_updates
if "%1"=="update" goto update
>&2 echo invalid args
exit /b 1
:get_updates
if "%mode%"=="check_error" (
  >&2 echo fwupdmgr get-updates error
  exit /b 1
)
if "%mode%"=="none" (
  >&2 echo No updatable devices
  exit /b 2
)
echo {"Devices":[{"Name":"USB-C Dock","CurrentVersion":"1.0.0","Releases":[{"Version":"1.1.0"}]}]}
exit /b 0
:update
if "%mode%"=="update_error" (
  >&2 echo fwupdmgr update error
  exit /b 1
)
exit /b 0
`
	} else {
		fileName = "fwupdmgr"
		content = `#!/bin/sh
mode="${DEVSYNC_TEST_FWUPDMGR_MODE}"
if [ "$1" = "get-updates" ]; then
  if [ "${mode}" = "check_error" ]; then
    echo "fwupdmgr get-updates error" 1>&2
    exit 1
  fi
  if [ "${mode}" = "none" ]; then
    echo "No updatable devices" 1>&2
    exit 2
  fi
  echo '{"Devices":[{"Name":"USB-C Dock","CurrentVersion":"1.0.0","Releases":[{"Version":"1.1.0"}]}]}'
  exit 0
fi
if [ "$1" = "update" ]; then
  if [ "${mode}" = "update_error" ]; then
    echo "fwupdmgr update error" 1>&2
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
		t.Fatalf("fake fwupdmgr command write failed: %v", err)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(fullPath, 0o755); err != nil {
			t.Fatalf("fake fwupdmgr command chmod failed: %v", err)
		}
	}
}
