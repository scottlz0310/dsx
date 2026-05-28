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
			t.Setenv("DSX_TEST_CARGO_MODE", tc.mode)

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
		noUpdate    bool // true: cargo-install-update バイナリを PATH に含めない
		noCargoBin  bool // true: CARGO_HOME/bin にもバイナリを配置しない（フォールバック失敗のテスト用）
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
			name:        "対象なし（DryRun）",
			mode:        "none",
			opts:        UpdateOptions{DryRun: true},
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
		{
			name:        "cargo-update 未インストール → 自動インストール → 更新成功",
			mode:        "updates",
			noUpdate:    true,
			opts:        UpdateOptions{},
			wantErr:     false,
			msgContains: "確認・更新しました",
		},
		{
			name:        "cargo-update 未インストール → 自動インストール失敗",
			mode:        "install_update_error",
			noUpdate:    true,
			opts:        UpdateOptions{},
			wantErr:     true,
			errContains: "cargo-update のインストールに失敗",
		},
		{
			name:        "cargo-update 未インストール → 自動インストール成功 → PATH にも CARGO_HOME にも見つからない",
			mode:        "updates",
			noUpdate:    true,
			noCargoBin:  true,
			opts:        UpdateOptions{},
			wantErr:     true,
			errContains: "PATH に見つかりません",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeCargoCommandImpl(t, fakeDir, !tc.noUpdate)

			// noUpdate=true の場合は PATH を fakeDir のみに絞り、
			// 実環境の cargo-install-update が LookPath に引っかからないようにする。
			// また CARGO_HOME を設定して cargoInstallUpdateBinPath のフォールバックが機能するよう
			// CARGO_HOME/bin にダミーバイナリを配置する（cargo install 後の状態をシミュレート）。
			if tc.noUpdate {
				cargoHomeDir := filepath.Join(fakeDir, "cargo_home")
				if !tc.noCargoBin {
					writeFakeCIUBinary(t, filepath.Join(cargoHomeDir, "bin"))
				}

				t.Setenv("CARGO_HOME", cargoHomeDir)
				t.Setenv("PATH", fakeDir)
			} else {
				t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			}

			t.Setenv("DSX_TEST_CARGO_MODE", tc.mode)

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
	writeFakeCargoCommandImpl(t, dir, true)
}

// writeFakeCargoCommandImpl は fake cargo コマンドを作成します。
// withUpdate が false の場合、cargo-install-update バイナリを作成しません（LookPath が失敗する）。
func writeFakeCargoCommandImpl(t *testing.T, dir string, withUpdate bool) {
	t.Helper()

	if runtime.GOOS == "windows" {
		cargoContent := `@echo off
set mode=%DSX_TEST_CARGO_MODE%
if "%1"=="install" goto doinstall
if "%1"=="install-update" goto doinstallupdate
echo invalid args 1>&2
exit /b 1
:doinstall
if "%2"=="--list" goto dolist
if "%2"=="--force" goto doforce
if "%2"=="cargo-update" goto doinstallcargoupdate
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
:doinstallcargoupdate
if "%mode%"=="install_update_error" (
  echo cargo install cargo-update failed 1>&2
  exit /b 1
)
exit /b 0
:doinstallupdate
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

		if !withUpdate {
			return
		}

		updateContent := "@echo off\r\nset mode=%DSX_TEST_CARGO_MODE%\r\nif \"%1\"==\"-a\" goto doupdate\r\necho invalid args 1>&2\r\nexit /b 1\r\n:doupdate\r\nif \"%mode%\"==\"update_error\" (\r\n  echo cargo install-update failed 1>&2\r\n  exit /b 1\r\n)\r\nexit /b 0\r\n"

		updatePath := filepath.Join(dir, "cargo-install-update.cmd")
		if err := os.WriteFile(updatePath, []byte(updateContent), 0o755); err != nil {
			t.Fatalf("fake cargo-install-update command write failed: %v", err)
		}
	} else {
		cargoContent := `#!/bin/sh
mode="${DSX_TEST_CARGO_MODE}"
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
      cargo-update)
        if [ "${mode}" = "install_update_error" ]; then
          echo "cargo install cargo-update failed" 1>&2
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
  install-update)
    case "$2" in
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

		if !withUpdate {
			return
		}

		updateContent := "#!/bin/sh\nmode=\"${DSX_TEST_CARGO_MODE}\"\ncase \"$1\" in\n  -a)\n    if [ \"${mode}\" = \"update_error\" ]; then\n      echo \"cargo install-update failed\" 1>&2\n      exit 1\n    fi\n    exit 0\n    ;;\n  *)\n    echo \"invalid args\" 1>&2\n    exit 1\n    ;;\nesac\n"

		updatePath := filepath.Join(dir, "cargo-install-update")
		if err := os.WriteFile(updatePath, []byte(updateContent), 0o755); err != nil {
			t.Fatalf("fake cargo-install-update command write failed: %v", err)
		}

		if err := os.Chmod(updatePath, 0o755); err != nil {
			t.Fatalf("fake cargo-install-update command chmod failed: %v", err)
		}
	}
}

// writeFakeCIUBinary は指定ディレクトリに fake cargo-install-update バイナリを作成します。
// noUpdate=true のテストで CARGO_HOME/bin へのフォールバックを検証するために使用します。
func writeFakeCIUBinary(t *testing.T, dir string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("dir creation failed: %v", err)
	}

	if runtime.GOOS == "windows" {
		content := "@echo off\r\nexit /b 0\r\n"
		path := filepath.Join(dir, "cargo-install-update.cmd")

		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("fake cargo-install-update.cmd write failed: %v", err)
		}
	} else {
		content := "#!/bin/sh\nexit 0\n"
		path := filepath.Join(dir, "cargo-install-update")

		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("fake cargo-install-update write failed: %v", err)
		}

		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatalf("fake cargo-install-update chmod failed: %v", err)
		}
	}
}
