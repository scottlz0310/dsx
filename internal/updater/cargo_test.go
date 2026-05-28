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

func TestCargoUpdater_parseUpdateCount(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected int
	}{
		// 共通
		{name: "空の出力", output: "", expected: 0},
		{name: "更新なし（サマリなし）", output: "ripgrep - already at newest version\n", expected: 0},
		// v19- 形式: "Updated N package(s)."
		{name: "v19/1件更新", output: "Updated 1 package.\n", expected: 1},
		{name: "v19/2件更新", output: "Updated 2 packages.\n", expected: 2},
		{name: "v19/大きい数字", output: "Updated 10 packages.\n", expected: 10},
		{name: "v19/CRLF改行", output: "Updated 2 packages.\r\n", expected: 2},
		{name: "v19/テーブル付き出力", output: "  ripgrep  14.0.0  14.1.0  Yes\n  bat      0.24.0  0.24.0  No\nUpdated 1 package.\n", expected: 1},
		// v20+ 形式: "Overall updated N package(s)."
		{name: "v20/更新なし", output: "No packages need updating.\nOverall updated 0 packages.\n", expected: 0},
		{name: "v20/1件更新", output: "Overall updated 1 package.\n", expected: 1},
		{name: "v20/2件更新", output: "Overall updated 2 packages.\n", expected: 2},
		{name: "v20/大きい数字", output: "Overall updated 10 packages.\n", expected: 10},
		{name: "v20/CRLF改行", output: "Overall updated 2 packages.\r\n", expected: 2},
		{name: "v20/テーブル付き出力", output: "Package       Installed  Latest   Needs update\ncargo-update  v20.0.0    v20.0.1  Yes\n\nOverall updated 1 package.\n", expected: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &CargoUpdater{}
			assert.Equal(t, tt.expected, c.parseUpdateCount(tt.output))
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
		v19         bool // true: v19 スタイルの cargo-install-update を使用（バージョン互換テスト用）
		opts        UpdateOptions
		wantErr     bool
		errContains string
		msgContains string
		wantUpdated int
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
			msgContains: "件のパッケージを更新しました",
			wantUpdated: 2,
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
			msgContains: "件のパッケージを更新しました",
			wantUpdated: 2,
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
		{
			name:        "v19環境での更新成功",
			mode:        "updates",
			v19:         true,
			opts:        UpdateOptions{},
			wantErr:     false,
			msgContains: "件のパッケージを更新しました",
			wantUpdated: 2,
		},
		{
			name:        "v19環境での更新なし",
			mode:        "none",
			v19:         true,
			opts:        UpdateOptions{},
			wantErr:     false,
			msgContains: "更新なし",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()

			switch {
			case tc.v19:
				// v19: cargo のみ書き、cargo-install-update は v19 スタイルで別途作成
				writeFakeCargoCommandImpl(t, fakeDir, false)
				writeFakeCargoInstallUpdateV19Binary(t, fakeDir)
				t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			case tc.noUpdate:
				// noUpdate=true の場合は PATH を fakeDir のみに絞り、
				// 実環境の cargo-install-update が LookPath に引っかからないようにする。
				// また CARGO_HOME を設定して cargoInstallUpdateBinPath のフォールバックが機能するよう
				// CARGO_HOME/bin にダミーバイナリを配置する（cargo install 後の状態をシミュレート）。
				writeFakeCargoCommandImpl(t, fakeDir, false)
				cargoHomeDir := filepath.Join(fakeDir, "cargo_home")
				if !tc.noCargoBin {
					writeFakeCIUBinary(t, filepath.Join(cargoHomeDir, "bin"))
				}
				t.Setenv("CARGO_HOME", cargoHomeDir)
				t.Setenv("PATH", fakeDir)
			default:
				writeFakeCargoCommandImpl(t, fakeDir, true)
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

			if !tc.opts.DryRun {
				assert.Equal(t, tc.wantUpdated, got.UpdatedCount)
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

		updateContent := "@echo off\r\nset mode=%DSX_TEST_CARGO_MODE%\r\nif \"%1\"==\"install-update\" goto doinstallupdate\r\necho invalid args 1>&2\r\nexit /b 1\r\n:doinstallupdate\r\nif \"%2\"==\"-a\" goto doupdate\r\necho invalid args 1>&2\r\nexit /b 1\r\n:doupdate\r\nif \"%mode%\"==\"update_error\" (\r\n  echo cargo install-update failed 1>&2\r\n  exit /b 1\r\n)\r\nif \"%mode%\"==\"updates\" (\r\n  echo Overall updated 2 packages.\r\n)\r\nexit /b 0\r\n"

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

		updateContent := "#!/bin/sh\nmode=\"${DSX_TEST_CARGO_MODE}\"\ncase \"$1\" in\n  install-update)\n    case \"$2\" in\n      -a)\n        if [ \"${mode}\" = \"update_error\" ]; then\n          echo \"cargo install-update failed\" 1>&2\n          exit 1\n        fi\n        if [ \"${mode}\" = \"updates\" ]; then\n          echo \"Overall updated 2 packages.\"\n        fi\n        exit 0\n        ;;\n      *)\n        echo \"invalid args\" 1>&2\n        exit 1\n        ;;\n    esac\n    ;;\n  *)\n    echo \"invalid args\" 1>&2\n    exit 1\n    ;;\nesac\n"

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
		content := "@echo off\r\nset mode=%DSX_TEST_CARGO_MODE%\r\nif \"%1\"==\"install-update\" goto doinstallupdate\r\nexit /b 1\r\n:doinstallupdate\r\nif \"%2\"==\"-a\" goto doupdate\r\nexit /b 1\r\n:doupdate\r\nif \"%mode%\"==\"updates\" (\r\n  echo Overall updated 2 packages.\r\n)\r\nexit /b 0\r\n"
		path := filepath.Join(dir, "cargo-install-update.cmd")

		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("fake cargo-install-update.cmd write failed: %v", err)
		}
	} else {
		content := "#!/bin/sh\nmode=\"${DSX_TEST_CARGO_MODE}\"\ncase \"$1\" in\n  install-update)\n    case \"$2\" in\n      -a)\n        if [ \"${mode}\" = \"updates\" ]; then\n          echo \"Overall updated 2 packages.\"\n        fi\n        exit 0\n        ;;\n      *)\n        exit 1\n        ;;\n    esac\n    ;;\n  *)\n    exit 1\n    ;;\nesac\n"
		path := filepath.Join(dir, "cargo-install-update")

		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("fake cargo-install-update write failed: %v", err)
		}

		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatalf("fake cargo-install-update chmod failed: %v", err)
		}
	}
}

// writeFakeCargoInstallUpdateV19Binary は v19 スタイルの fake cargo-install-update を作成します。
// --version は "cargo-install-update 16.0.0" を返し、-a を直接受け付けます。
func writeFakeCargoInstallUpdateV19Binary(t *testing.T, dir string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		content := "@echo off\r\nset mode=%DSX_TEST_CARGO_MODE%\r\nif \"%1\"==\"--version\" (\r\n  echo cargo-install-update 16.0.0\r\n  exit /b 0\r\n)\r\nif \"%1\"==\"-a\" goto doupdate\r\necho invalid args 1>&2\r\nexit /b 1\r\n:doupdate\r\nif \"%mode%\"==\"update_error\" (\r\n  echo cargo install-update failed 1>&2\r\n  exit /b 1\r\n)\r\nif \"%mode%\"==\"updates\" (\r\n  echo Updated 2 packages.\r\n)\r\nexit /b 0\r\n"
		path := filepath.Join(dir, "cargo-install-update.cmd")

		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("writeFakeCargoInstallUpdateV19Binary write failed: %v", err)
		}
	} else {
		content := "#!/bin/sh\nmode=\"${DSX_TEST_CARGO_MODE}\"\ncase \"$1\" in\n  --version)\n    echo \"cargo-install-update 16.0.0\"\n    exit 0\n    ;;\n  -a)\n    if [ \"${mode}\" = \"update_error\" ]; then\n      echo \"cargo install-update failed\" 1>&2\n      exit 1\n    fi\n    if [ \"${mode}\" = \"updates\" ]; then\n      echo \"Updated 2 packages.\"\n    fi\n    exit 0\n    ;;\n  *)\n    echo \"invalid args\" 1>&2\n    exit 1\n    ;;\nesac\n"
		path := filepath.Join(dir, "cargo-install-update")

		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("writeFakeCargoInstallUpdateV19Binary write failed: %v", err)
		}

		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatalf("writeFakeCargoInstallUpdateV19Binary chmod failed: %v", err)
		}
	}
}

func TestCargoUpdateArgs(t *testing.T) {
	tests := []struct {
		name       string
		versionOut string // --version の出力（空なら exit 1）
		wantArgs   []string
	}{
		{
			name:       "v20+（cargo N.N.N 形式）",
			versionOut: "cargo 20.0.0",
			wantArgs:   []string{"install-update", "-a"},
		},
		{
			name:       "v19-（cargo-install-update N.N.N 形式）",
			versionOut: "cargo-install-update 16.0.0",
			wantArgs:   []string{"-a"},
		},
		{
			name:       "--version 失敗時は v20+ 形式にフォールバック",
			versionOut: "", // exit 1
			wantArgs:   []string{"install-update", "-a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			binPath := writeFakeVersionBinary(t, dir, tt.versionOut)
			got := cargoUpdateArgs(binPath)
			assert.Equal(t, tt.wantArgs, got)
		})
	}
}

// writeFakeVersionBinary は --version に対して versionOut を返す fake バイナリを作成し、そのパスを返します。
// versionOut が空の場合は --version で exit 1 を返します。
func writeFakeVersionBinary(t *testing.T, dir string, versionOut string) string {
	t.Helper()

	if runtime.GOOS == "windows" {
		var content string
		if versionOut == "" {
			content = "@echo off\r\nexit /b 1\r\n"
		} else {
			content = "@echo off\r\nif \"%1\"==\"--version\" (\r\n  echo " + versionOut + "\r\n  exit /b 0\r\n)\r\nexit /b 1\r\n"
		}

		path := filepath.Join(dir, "fake-bin.cmd")
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("writeFakeVersionBinary write failed: %v", err)
		}

		return path
	}

	var content string
	if versionOut == "" {
		content = "#!/bin/sh\nexit 1\n"
	} else {
		content = "#!/bin/sh\ncase \"$1\" in\n  --version)\n    echo \"" + versionOut + "\"\n    exit 0\n    ;;\nesac\nexit 1\n"
	}

	path := filepath.Join(dir, "fake-bin")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("writeFakeVersionBinary write failed: %v", err)
	}

	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("writeFakeVersionBinary chmod failed: %v", err)
	}

	return path
}
