package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/scottlz0310/dsx/internal/config"
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

func TestUVUpdater_parseSelfUpdateDryRunOutput(t *testing.T) {
	testCases := []struct {
		name   string
		output string
		want   []PackageInfo
	}{
		{
			name:   "更新あり",
			output: "info: Checking for updates...\nWould update uv from v0.11.16 to v0.11.17\n",
			want: []PackageInfo{
				{Name: "uv", CurrentVersion: "0.11.16", NewVersion: "0.11.17"},
			},
		},
		{
			name:   "vなしでも解析する",
			output: "Would update uv from 0.11.16 to 0.11.17\n",
			want: []PackageInfo{
				{Name: "uv", CurrentVersion: "0.11.16", NewVersion: "0.11.17"},
			},
		},
		{
			name:   "最新版",
			output: "uv is already up-to-date\n",
			want:   []PackageInfo{},
		},
		{
			name:   "空出力",
			output: "",
			want:   []PackageInfo{},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			u := &UVUpdater{}
			got := u.parseSelfUpdateDryRunOutput(tc.output)
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
			t.Setenv("DSX_TEST_UV_MODE", tc.mode)

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

func TestUVUpdater_CheckSelfUpdate(t *testing.T) {
	testCases := []struct {
		name        string
		mode        string
		wantUpdates int
		wantPackage PackageInfo
		wantErr     bool
		errContains string
		msgContains string
	}{
		{
			name:        "本体更新あり",
			mode:        "self_update_available",
			wantUpdates: 1,
			wantPackage: PackageInfo{Name: "uv", CurrentVersion: "0.11.16", NewVersion: "0.11.17"},
			msgContains: "更新が可能",
		},
		{
			name:        "本体は最新版",
			mode:        "self_latest",
			wantUpdates: 0,
			msgContains: "最新",
		},
		{
			name:        "dry-run失敗",
			mode:        "self_error",
			wantErr:     true,
			errContains: "uv self update --dry-run の実行に失敗",
		},
		{
			name:        "self update非対応のインストール経路はスキップ",
			mode:        "self_unsupported",
			wantUpdates: 0,
			msgContains: "利用できない",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeUVCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DSX_TEST_UV_MODE", tc.mode)

			u := &UVUpdater{}
			got, err := u.CheckSelfUpdate(context.Background())

			if tc.wantErr {
				assert.Error(t, err)

				if tc.errContains != "" && err != nil {
					assert.Contains(t, err.Error(), tc.errContains)
				}

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.wantUpdates, got.AvailableUpdates)
			assert.Contains(t, got.Message, tc.msgContains)

			if tc.wantUpdates > 0 {
				assert.Equal(t, []PackageInfo{tc.wantPackage}, got.Packages)
			}
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
			t.Setenv("DSX_TEST_UV_MODE", tc.mode)

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

func TestUVUpdater_SelfUpdate(t *testing.T) {
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
			name:        "DryRunは実更新せず計画表示",
			mode:        "self_update_available",
			opts:        UpdateOptions{DryRun: true},
			wantUpdated: 0,
			msgContains: "DryRun",
		},
		{
			name:        "本体は最新版",
			mode:        "self_latest",
			opts:        UpdateOptions{},
			wantUpdated: 0,
			msgContains: "最新",
		},
		{
			name:        "本体更新成功",
			mode:        "self_update_available",
			opts:        UpdateOptions{},
			wantUpdated: 1,
			msgContains: "uv 本体を更新しました",
		},
		{
			name:        "本体更新失敗",
			mode:        "self_update_error",
			opts:        UpdateOptions{},
			wantErr:     true,
			errContains: "uv self update に失敗",
		},
		{
			name:        "self update非対応のインストール経路はエラーにしない",
			mode:        "self_unsupported",
			opts:        UpdateOptions{},
			wantUpdated: 0,
			msgContains: "利用できない",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeUVCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DSX_TEST_UV_MODE", tc.mode)

			u := &UVUpdater{}
			got, err := u.SelfUpdate(context.Background(), tc.opts)

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
			assert.Equal(t, ContinueNormalUpdate, got.Continuation)

			if tc.msgContains != "" {
				assert.Contains(t, got.Message, tc.msgContains)
			}
		})
	}
}

// 実行中の uv / uvx がある場合は、`uv self update` を実行せずに見送る（Windows は実行中の exe を置き換えられない）。
// 見送りが失敗ではないこと、実行しないこと（self_update_error なら実行すると失敗する）を固定する。
func TestUVUpdater_SelfUpdate_InUse(t *testing.T) {
	testCases := []struct {
		name        string
		mode        string
		opts        UpdateOptions
		listRunning func(dir string, names []string) ([]string, error)
		wantInUse   bool
		wantUpdated int
		wantErr     string
		wantMsg     []string
	}{
		{
			name: "実行中なら更新を見送る（失敗にしない）",
			mode: "self_update_error",
			listRunning: func(string, []string) ([]string, error) {
				return []string{"uvx.exe (PID 17992)", "uv.exe (PID 21000)"}, nil
			},
			wantInUse: true,
			wantMsg:   []string{"uv / uvx が実行中", "v0.11.16 → v0.11.17", "uvx.exe (PID 17992), uv.exe (PID 21000)"},
		},
		{
			name: "実行中でなければ更新する",
			mode: "self_update_available",
			listRunning: func(string, []string) ([]string, error) {
				return nil, nil
			},
			wantUpdated: 1,
			wantMsg:     []string{"uv 本体を更新しました"},
		},
		{
			name: "DryRun は実行中かを調べない",
			mode: "self_update_available",
			opts: UpdateOptions{DryRun: true},
			listRunning: func(string, []string) ([]string, error) {
				return nil, errors.New("呼ばれてはいけない")
			},
			wantMsg: []string{"DryRun"},
		},
		{
			name: "最新版なら実行中かを調べない",
			mode: "self_latest",
			listRunning: func(string, []string) ([]string, error) {
				return nil, errors.New("呼ばれてはいけない")
			},
			wantMsg: []string{"最新"},
		},
		{
			name: "実行中かを確認できないときはエラー（更新しない）",
			mode: "self_update_available",
			listRunning: func(string, []string) ([]string, error) {
				return nil, errors.New("access denied")
			},
			wantErr: "uv の実行中のプロセスを確認できません: access denied",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeUVCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DSX_TEST_UV_MODE", tc.mode)

			var gotDir string

			u := &UVUpdater{listRunning: func(dir string, names []string) ([]string, error) {
				gotDir = dir

				assert.Equal(t, uvExecutableNames, names)

				return tc.listRunning(dir, names)
			}}

			got, err := u.SelfUpdate(context.Background(), tc.opts)

			if tc.wantErr != "" {
				assert.EqualError(t, err, tc.wantErr)

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.wantInUse, got.InUse)
			assert.Equal(t, ContinueNormalUpdate, got.Continuation)
			assert.Equal(t, tc.wantUpdated, got.UpdatedCount)

			for _, want := range tc.wantMsg {
				assert.Contains(t, got.Message, want)
			}

			if gotDir != "" {
				// 確認する場所は、更新で置き換える uv のあるディレクトリ。
				wantDir, err := filepath.EvalSymlinks(fakeDir)
				assert.NoError(t, err)

				gotResolved, err := filepath.EvalSymlinks(gotDir)
				assert.NoError(t, err)
				assert.Equal(t, wantDir, gotResolved)
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
set mode=%DSX_TEST_UV_MODE%
if "%1"=="tool" if "%2"=="list" goto list
if "%1"=="tool" if "%2"=="upgrade" goto upgrade
if "%1"=="self" if "%2"=="update" goto selfupdate
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
:selfupdate
if "%3"=="--dry-run" goto selfupdatedryrun
if "%mode%"=="self_update_error" (
  >&2 echo uv self update failed
  exit /b 1
)
exit /b 0
:selfupdatedryrun
if "%mode%"=="self_error" goto selferror
if "%mode%"=="self_unsupported" goto selfunsupported
if "%mode%"=="self_update_available" if not "%DSX_TEST_UV_SELF_UPDATED%"=="1" goto selfavailable
if "%mode%"=="self_update_error" goto selfavailable
echo uv is already up-to-date
exit /b 0
:selferror
echo uv self update dry-run failed 1>&2
exit /b 1
:selfunsupported
echo warning: Self-update is only available for uv binaries installed via the standalone installation scripts. 1>&2
exit /b 1
:selfavailable
echo info: Checking for updates...
echo Would update uv from v0.11.16 to v0.11.17
exit /b 0
`
	} else {
		fileName = "uv"
		content = `#!/bin/sh
mode="${DSX_TEST_UV_MODE}"
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
if [ "$1" = "self" ] && [ "$2" = "update" ]; then
  if [ "$3" = "--dry-run" ]; then
    if [ "${mode}" = "self_error" ]; then
      echo "uv self update dry-run failed" 1>&2
      exit 1
    fi
    if [ "${mode}" = "self_unsupported" ]; then
      echo "warning: Self-update is only available for uv binaries installed via the standalone installation scripts." 1>&2
      exit 1
    fi
    if [ "${mode}" = "self_update_available" ] && [ "${DSX_TEST_UV_SELF_UPDATED}" != "1" ]; then
      echo "info: Checking for updates..."
      echo "Would update uv from v0.11.16 to v0.11.17"
      exit 0
    fi
    if [ "${mode}" = "self_update_error" ]; then
      echo "info: Checking for updates..."
      echo "Would update uv from v0.11.16 to v0.11.17"
      exit 0
    fi
    echo "uv is already up-to-date"
    exit 0
  fi
  if [ "${mode}" = "self_update_error" ]; then
    echo "uv self update failed" 1>&2
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
