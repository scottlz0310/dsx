package updater

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCargoUpdater_parseInstallList(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected []PackageInfo
	}{
		{
			name:     "空の出力",
			output:   "",
			expected: nil,
		},
		{
			name: "単一パッケージ",
			output: `ripgrep v13.0.0:
    rg
`,
			expected: []PackageInfo{
				{Name: "ripgrep", CurrentVersion: "13.0.0"},
			},
		},
		{
			name: "複数パッケージ",
			output: `ripgrep v13.0.0:
    rg

cargo-update v16.0.0:
    cargo-install-update
    cargo-install-update-config
`,
			expected: []PackageInfo{
				{Name: "ripgrep", CurrentVersion: "13.0.0"},
				{Name: "cargo-update", CurrentVersion: "16.0.0"},
			},
		},
		{
			name: "不正な行は無視される",
			output: `no-colon-line
pkg-only:
pkg 1.2.3
pkg 1.2.3:
    bin
`,
			expected: []PackageInfo{
				{Name: "pkg", CurrentVersion: "1.2.3"},
			},
		},
		{
			name: "versionにvが無い場合も許容",
			output: `pkg 1.2.3:
    bin
`,
			expected: []PackageInfo{
				{Name: "pkg", CurrentVersion: "1.2.3"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &CargoUpdater{}
			got := c.parseInstallList(tt.output)

			assert.Len(t, got, len(tt.expected))

			for i := range tt.expected {
				assert.Equal(t, tt.expected[i].Name, got[i].Name)
				assert.Equal(t, tt.expected[i].CurrentVersion, got[i].CurrentVersion)
				assert.Equal(t, "", got[i].NewVersion)
			}
		})
	}
}

func TestCargoUpdater_Check(t *testing.T) {
	testCases := []struct {
		name        string
		mode        string
		wantPkgs    int
		wantErr     bool
		errContains string
	}{
		{
			name:     "パッケージあり",
			mode:     "updates",
			wantPkgs: 2,
			wantErr:  false,
		},
		{
			name:     "パッケージなし",
			mode:     "none",
			wantPkgs: 0,
			wantErr:  false,
		},
		{
			name:        "コマンド失敗",
			mode:        "check_error",
			wantPkgs:    0,
			wantErr:     true,
			errContains: "cargo install --list の実行に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeCargoCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_CARGO_MODE", tc.mode)

			c := &CargoUpdater{}
			got, err := c.Check(context.Background())

			if tc.wantErr {
				assert.Error(t, err)

				if tc.errContains != "" && err != nil {
					assert.Contains(t, err.Error(), tc.errContains)
				}

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, 0, got.AvailableUpdates)
			assert.Len(t, got.Packages, tc.wantPkgs)
		})
	}
}

func TestCargoUpdater_Update(t *testing.T) {
	testCases := []struct {
		name        string
		mode        string
		opts        UpdateOptions
		wantErr     bool
		errContains string
		msgContains string
	}{
		{
			name:        "DryRunは更新せず計画表示",
			mode:        "updates",
			opts:        UpdateOptions{DryRun: true},
			wantErr:     false,
			msgContains: "DryRunモード",
		},
		{
			name:        "対象なし",
			mode:        "none",
			opts:        UpdateOptions{},
			wantErr:     false,
			msgContains: "パッケージがありません",
		},
		{
			name:        "更新成功",
			mode:        "updates",
			opts:        UpdateOptions{},
			wantErr:     false,
			msgContains: "確認・更新しました",
		},
		{
			name:        "更新失敗",
			mode:        "update_error",
			opts:        UpdateOptions{},
			wantErr:     true,
			errContains: "cargo install-update -a に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeCargoCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_CARGO_MODE", tc.mode)

			c := &CargoUpdater{}
			got, err := c.Update(context.Background(), tc.opts)

			if tc.wantErr {
				assert.Error(t, err)

				if tc.errContains != "" && err != nil {
					assert.Contains(t, err.Error(), tc.errContains)
				}

				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, got)

			if tc.msgContains != "" {
				assert.Contains(t, got.Message, tc.msgContains)
			}
		})
	}
}

func writeFakeCargoCommand(t *testing.T, dir string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		// fake cargo.cmd
		cargoContent := `@echo off
set mode=%DEVSYNC_TEST_CARGO_MODE%
if "%1"=="install" goto doinstall
if "%1"=="install-update" goto doinstallupdate
echo invalid args 1>&2
exit /b 1
:doinstall
if "%2"=="--list" goto dolist
if "%2"=="--force" goto doforce
echo invalid args 1>&2
exit /b 1
:dolist
if "%mode%"=="check_error" (
  echo cargo install --list failed 1>&2
  exit /b 1
)
if "%mode%"=="none" (
  exit /b 0
)
echo ripgrep v14.0.0:
echo     rg
echo bat v0.24.0:
echo     bat
exit /b 0
:doforce
exit /b 0
:doinstallupdate
if "%2"=="--help" exit /b 0
if "%2"=="-a" goto doupdate
echo invalid args 1>&2
exit /b 1
:doupdate
if "%mode%"=="update_error" (
  echo cargo install-update failed 1>&2
  exit /b 1
)
exit /b 0
`

		cargoPath := filepath.Join(dir, "cargo.cmd")
		if err := os.WriteFile(cargoPath, []byte(cargoContent), 0o755); err != nil {
			t.Fatalf("fake cargo command write failed: %v", err)
		}

		// fake cargo-install-update.cmd（LookPath 用）
		updateContent := `@echo off
exit /b 0
`

		updatePath := filepath.Join(dir, "cargo-install-update.cmd")
		if err := os.WriteFile(updatePath, []byte(updateContent), 0o755); err != nil {
			t.Fatalf("fake cargo-install-update command write failed: %v", err)
		}
	} else {
		// fake cargo
		cargoContent := `#!/bin/sh
mode="${DEVSYNC_TEST_CARGO_MODE}"
case "$1" in
  install)
    case "$2" in
      --list)
        if [ "${mode}" = "check_error" ]; then
          echo "cargo install --list failed" 1>&2
          exit 1
        fi
        if [ "${mode}" = "none" ]; then
          exit 0
        fi
        echo "ripgrep v14.0.0:"
        echo "    rg"
        echo "bat v0.24.0:"
        echo "    bat"
        exit 0
        ;;
      --force)
        exit 0
        ;;
      *)
        echo "invalid args" 1>&2
        exit 1
        ;;
    esac
    ;;
  install-update)
    case "$2" in
      --help)
        exit 0
        ;;
      -a)
        if [ "${mode}" = "update_error" ]; then
          echo "cargo install-update failed" 1>&2
          exit 1
        fi
        exit 0
        ;;
      *)
        echo "invalid args" 1>&2
        exit 1
        ;;
    esac
    ;;
  *)
    echo "invalid args" 1>&2
    exit 1
    ;;
esac
`

		cargoPath := filepath.Join(dir, "cargo")
		if err := os.WriteFile(cargoPath, []byte(cargoContent), 0o755); err != nil {
			t.Fatalf("fake cargo command write failed: %v", err)
		}

		if err := os.Chmod(cargoPath, 0o755); err != nil {
			t.Fatalf("fake cargo command chmod failed: %v", err)
		}

		// fake cargo-install-update（LookPath 用）
		updateContent := `#!/bin/sh
exit 0
`

		updatePath := filepath.Join(dir, "cargo-install-update")
		if err := os.WriteFile(updatePath, []byte(updateContent), 0o755); err != nil {
			t.Fatalf("fake cargo-install-update command write failed: %v", err)
		}

		if err := os.Chmod(updatePath, 0o755); err != nil {
			t.Fatalf("fake cargo-install-update command chmod failed: %v", err)
		}
	}
}
