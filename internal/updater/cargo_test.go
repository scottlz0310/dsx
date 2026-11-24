package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/scottlz0310/dsx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCargoUpdater_Configure(t *testing.T) {
	tests := []struct {
		name        string
		initialCfg  config.ManagerConfig
		cfg         config.ManagerConfig
		wantGit     bool
		wantLocked  bool
		wantErr     bool
		errContains string
	}{
		{
			name:       "nilの設定は既定値（git=true, locked=false）",
			cfg:        nil,
			wantGit:    true,
			wantLocked: false,
		},
		{
			name:       "空マップは既定値（git=true, locked=false）",
			cfg:        config.ManagerConfig{},
			wantGit:    true,
			wantLocked: false,
		},
		{
			name:       "git=falseでGit更新を無効化できる",
			cfg:        config.ManagerConfig{"git": false},
			wantGit:    false,
			wantLocked: false,
		},
		{
			name:       "locked=trueで--lockedを有効化できる",
			cfg:        config.ManagerConfig{"locked": true},
			wantGit:    true,
			wantLocked: true,
		},
		{
			name:       "git=falseかつlocked=true",
			cfg:        config.ManagerConfig{"git": false, "locked": true},
			wantGit:    false,
			wantLocked: true,
		},
		{
			name:       "前回設定後にnilで再適用すると既定値へリセットされる",
			initialCfg: config.ManagerConfig{"git": false, "locked": true},
			cfg:        nil,
			wantGit:    true,
			wantLocked: false,
		},
		{
			name:       "未知キーは無視される",
			cfg:        config.ManagerConfig{"unknown_option": true},
			wantGit:    true,
			wantLocked: false,
		},
		{
			name:        "gitが文字列ならエラー",
			cfg:         config.ManagerConfig{"git": "false"},
			wantErr:     true,
			errContains: "sys.managers.cargo.git",
		},
		{
			name:        "gitが数値ならエラー",
			cfg:         config.ManagerConfig{"git": 1},
			wantErr:     true,
			errContains: "sys.managers.cargo.git",
		},
		{
			name:        "gitがnilならエラー",
			cfg:         config.ManagerConfig{"git": nil},
			wantErr:     true,
			errContains: "sys.managers.cargo.git",
		},
		{
			name:        "lockedが文字列ならエラー",
			cfg:         config.ManagerConfig{"locked": "true"},
			wantErr:     true,
			errContains: "sys.managers.cargo.locked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &CargoUpdater{}
			if tt.initialCfg != nil {
				require.NoError(t, c.Configure(tt.initialCfg))
			}

			err := c.Configure(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantGit, c.includeGit())
			assert.Equal(t, tt.wantLocked, c.locked)
		})
	}
}

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
			name: "複数パッケージ（Git由来を含む）",
			output: `agent-statusline v0.3.1 (https://github.com/scottlz0310/agent-statusline?tag=v0.3.1#452b4c9c):
    agent-statusline.exe
cargo-update v22.1.5:
    cargo-install-update-config.exe
    cargo-install-update.exe
`,
			expected: []PackageInfo{
				{Name: "agent-statusline", CurrentVersion: "0.3.1"},
				{Name: "cargo-update", CurrentVersion: "22.1.5"},
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
		{name: "関係ないUpdated行は無視する", output: "Updated 2026 index cache\n", expected: 0},
		// v19- 形式: "Updated N [git ]package(s)."
		{name: "v19/1件更新", output: "Updated 1 package.\n", expected: 1},
		{name: "v19/2件更新", output: "Updated 2 packages.\n", expected: 2},
		{name: "v19/大きい数字", output: "Updated 10 packages.\n", expected: 10},
		{name: "v19/CRLF改行", output: "Updated 2 packages.\r\n", expected: 2},
		{name: "v19/テーブル付き出力", output: "  ripgrep  14.0.0  14.1.0  Yes\n  bat      0.24.0  0.24.0  No\nUpdated 1 package.\n", expected: 1},
		{name: "v19/Gitパッケージのみ更新", output: "Updated 1 git package.\n", expected: 1},
		{name: "v19/crates.ioとGit混在は合算", output: "Updated 1 package.\n\nUpdated 2 git packages.\n", expected: 3},
		// v20+ 形式: "Overall updated N package(s)..."
		{name: "v20/更新なし", output: "No packages need updating.\nOverall updated 0 packages.\n", expected: 0},
		{name: "v20/1件更新", output: "Overall updated 1 package.\n", expected: 1},
		{name: "v20/2件更新", output: "Overall updated 2 packages.\n", expected: 2},
		{name: "v20/大きい数字", output: "Overall updated 10 packages.\n", expected: 10},
		{name: "v20/CRLF改行", output: "Overall updated 2 packages.\r\n", expected: 2},
		{name: "v20/テーブル付き出力", output: "Package       Installed  Latest   Needs update\ncargo-update  v20.0.0    v20.0.1  Yes\n\nOverall updated 1 package.\n", expected: 1},
		{
			name: "v20/crates.ioとGit混在でOverall updatedを最優先する",
			output: `    Polling registry 'https://index.crates.io/'.

Package       Installed  Latest   Needs update
cargo-update  v22.1.4    v22.1.5  Yes

    Checking 1 git package.

Package           Installed                                 Latest                                    Needs update
agent-statusline  452b4c9c1cb9dcbfc6d2a24328eec8e228749843  6844ff2851a3ae05616ddf54495bbbc93e9e244a  Yes

Updated 1 package.

Updated 1 git package.

Overall updated 2 packages: cargo-update, agent-statusline.
`,
			expected: 2,
		},
		{
			name: "v20/crates.io失敗0件かつGit1件成功でもOverall updatedで1件と集計する",
			output: `Updated 0 packages.
Failed to update broken-crate.

Updated 1 git package.

Overall updated 1 package: agent-statusline.
`,
			expected: 1,
		},
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
		name          string
		mode          string
		cfg           config.ManagerConfig
		noUpdate      bool   // true: cargo-install-update バイナリを PATH に含めない
		noCargoBin    bool   // true: CARGO_HOME/bin にもバイナリを配置しない（フォールバック失敗のテスト用）
		v19           bool   // true: v19 スタイルの cargo-install-update を使用（バージョン互換テスト用）
		v19Version    string // v19 fake バイナリの --version 出力バージョン（空なら 16.0.0）
		expectArgs    string
		opts          UpdateOptions
		wantErr       bool
		errContains   string
		msgContains   string
		wantUpdated   int
		wantInstalled []PackageInfo
	}{
		{
			name:        "DryRunは更新せず計画表示（既定: Git パッケージ含む）",
			mode:        "updates",
			opts:        UpdateOptions{DryRun: true},
			wantErr:     false,
			msgContains: "Git パッケージ含む / DryRunモード",
			wantInstalled: []PackageInfo{
				{Name: "ripgrep", CurrentVersion: "14.0.0"},
				{Name: "bat", CurrentVersion: "0.24.0"},
			},
		},
		{
			name:        "DryRunはgit無効・locked有効をメッセージに反映する",
			mode:        "updates",
			cfg:         config.ManagerConfig{"git": false, "locked": true},
			opts:        UpdateOptions{DryRun: true},
			wantErr:     false,
			msgContains: "--locked / DryRunモード",
			wantInstalled: []PackageInfo{
				{Name: "ripgrep", CurrentVersion: "14.0.0"},
				{Name: "bat", CurrentVersion: "0.24.0"},
			},
		},
		{
			name:        "DryRunはgit有効・locked有効をメッセージに反映する",
			mode:        "updates",
			cfg:         config.ManagerConfig{"git": true, "locked": true},
			opts:        UpdateOptions{DryRun: true},
			wantErr:     false,
			msgContains: "Git パッケージ含む・--locked / DryRunモード",
			wantInstalled: []PackageInfo{
				{Name: "ripgrep", CurrentVersion: "14.0.0"},
				{Name: "bat", CurrentVersion: "0.24.0"},
			},
		},
		{
			name:        "DryRunはgit無効・locked無効で従来表示",
			mode:        "updates",
			cfg:         config.ManagerConfig{"git": false, "locked": false},
			opts:        UpdateOptions{DryRun: true},
			wantErr:     false,
			msgContains: "（DryRunモード）",
			wantInstalled: []PackageInfo{
				{Name: "ripgrep", CurrentVersion: "14.0.0"},
				{Name: "bat", CurrentVersion: "0.24.0"},
			},
		},
		{
			name:        "対象なし（DryRun）",
			mode:        "none",
			opts:        UpdateOptions{DryRun: true},
			wantErr:     false,
			msgContains: "パッケージがありません",
		},
		{
			name:        "更新成功（既定は -a -g を渡す）",
			mode:        "updates",
			expectArgs:  "install-update -a -g",
			opts:        UpdateOptions{},
			wantErr:     false,
			msgContains: "件のパッケージを更新しました",
			wantUpdated: 2,
		},
		{
			name:        "更新成功（git無効で -a のみを渡す）",
			mode:        "updates",
			cfg:         config.ManagerConfig{"git": false},
			expectArgs:  "install-update -a",
			opts:        UpdateOptions{},
			wantErr:     false,
			msgContains: "件のパッケージを更新しました",
			wantUpdated: 2,
		},
		{
			name:        "更新成功（locked有効で -a -g --locked を渡す）",
			mode:        "updates",
			cfg:         config.ManagerConfig{"locked": true},
			expectArgs:  "install-update -a -g --locked",
			opts:        UpdateOptions{},
			wantErr:     false,
			msgContains: "件のパッケージを更新しました",
			wantUpdated: 2,
		},
		{
			name:        "更新成功（git無効かつlocked有効で -a --locked を渡す）",
			mode:        "updates",
			cfg:         config.ManagerConfig{"git": false, "locked": true},
			expectArgs:  "install-update -a --locked",
			opts:        UpdateOptions{},
			wantErr:     false,
			msgContains: "件のパッケージを更新しました",
			wantUpdated: 2,
		},
		{
			name:        "更新失敗（実行した引数を含むエラー）",
			mode:        "update_error",
			expectArgs:  "install-update -a -g",
			opts:        UpdateOptions{},
			wantErr:     true,
			errContains: "cargo install-update -a -g に失敗",
		},
		{
			name:        "部分成功を伴う非ゼロ終了でもUpdatedCountを引き継ぐ",
			mode:        "partial_update_error",
			expectArgs:  "install-update -a -g",
			opts:        UpdateOptions{},
			wantErr:     true,
			errContains: "cargo install-update -a -g に失敗",
			wantUpdated: 1,
		},
		{
			name:        "cargo-update v14.0.0 で locked有効 は更新前にアップグレード案内エラーを返す",
			mode:        "updates",
			cfg:         config.ManagerConfig{"locked": true},
			v19:         true,
			v19Version:  "14.0.0",
			opts:        UpdateOptions{},
			wantErr:     true,
			errContains: "cargo-update v14.1.0 以上が必要です",
		},
		{
			name:        "cargo-update 未インストール → 自動インストール → 更新成功",
			mode:        "updates",
			noUpdate:    true,
			expectArgs:  "install-update -a -g",
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
			name:        "v19環境での更新成功（既定は -a -g）",
			mode:        "updates",
			v19:         true,
			expectArgs:  "-a -g",
			opts:        UpdateOptions{},
			wantErr:     false,
			msgContains: "件のパッケージを更新しました",
			wantUpdated: 2,
		},
		{
			name:        "v19環境での更新成功（git無効かつlocked有効で -a --locked）",
			mode:        "updates",
			cfg:         config.ManagerConfig{"git": false, "locked": true},
			v19:         true,
			expectArgs:  "-a --locked",
			opts:        UpdateOptions{},
			wantErr:     false,
			msgContains: "件のパッケージを更新しました",
			wantUpdated: 2,
		},
		{
			name:        "v19環境での更新なし",
			mode:        "none",
			v19:         true,
			expectArgs:  "-a -g",
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
				writeFakeCargoInstallUpdateV19Binary(t, fakeDir, tc.v19Version)
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
			t.Setenv("DSX_TEST_CARGO_EXPECT_ARGS", tc.expectArgs)

			c := &CargoUpdater{
				// テスト環境の実際のプロセスに左右されないよう空の検知関数を注入
				listRunning: func(string, []string) ([]string, error) { return nil, nil },
			}
			if tc.cfg != nil {
				require.NoError(t, c.Configure(tc.cfg))
			}

			got, err := c.Update(context.Background(), tc.opts)

			if tc.wantErr {
				assert.Error(t, err)

				if tc.errContains != "" && err != nil {
					assert.Contains(t, err.Error(), tc.errContains)
				}

				if tc.wantUpdated > 0 {
					require.NotNil(t, got)
					assert.Equal(t, tc.wantUpdated, got.UpdatedCount)
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

			assert.Empty(t, got.Packages)

			if tc.wantInstalled == nil {
				assert.Empty(t, got.InstalledPackages)
			} else {
				assert.Equal(t, tc.wantInstalled, got.InstalledPackages)
			}
		})
	}
}

func TestCargoUpdater_Update_FailureRunningProcessHint(t *testing.T) {
	tests := []struct {
		name           string
		binFiles       []string
		listRunning    func(dir string, names []string) ([]string, error)
		wantErrContain string
		wantNoHint     bool
	}{
		{
			name:     "更新失敗時にCARGO_HOME/bin配下の実行中プロセスがあればエラーへ補足する",
			binFiles: []string{"agent-statusline.exe", "README.txt"},
			listRunning: func(dir string, names []string) ([]string, error) {
				if len(names) != 1 || names[0] != "agent-statusline.exe" {
					return nil, errors.New("unexpected names: " + strings.Join(names, ","))
				}

				return []string{"agent-statusline.exe (PID 4242)"}, nil
			},
			wantErrContain: "実行中の cargo バイナリがあるため置き換えに失敗した可能性があります: agent-statusline.exe (PID 4242)",
		},
		{
			name:     "実行中プロセスが空なら補足なしで元のエラーを返す",
			binFiles: []string{"agent-statusline.exe"},
			listRunning: func(string, []string) ([]string, error) {
				return nil, nil
			},
			wantErrContain: "cargo install-update -a -g に失敗",
			wantNoHint:     true,
		},
		{
			name:     "プロセス一覧の取得に失敗しても元の更新エラーを隠さない",
			binFiles: []string{"agent-statusline.exe"},
			listRunning: func(string, []string) ([]string, error) {
				return nil, errors.New("snapshot error")
			},
			wantErrContain: "cargo install-update -a -g に失敗",
			wantNoHint:     true,
		},
		{
			name:     "CARGO_HOME/binに.exeが無い場合はプロセス一覧を呼ばず元のエラーを返す",
			binFiles: []string{"agent-statusline"},
			listRunning: func(string, []string) ([]string, error) {
				return []string{"should-not-be-called"}, nil
			},
			wantErrContain: "cargo install-update -a -g に失敗",
			wantNoHint:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeCargoCommandImpl(t, fakeDir, true)

			cargoHome := filepath.Join(fakeDir, "cargo_home")
			binDir := filepath.Join(cargoHome, "bin")
			require.NoError(t, os.MkdirAll(binDir, 0o755))

			for _, name := range tt.binFiles {
				require.NoError(t, os.WriteFile(filepath.Join(binDir, name), []byte("dummy"), 0o644))
			}

			t.Setenv("CARGO_HOME", cargoHome)
			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DSX_TEST_CARGO_MODE", "update_error")
			t.Setenv("DSX_TEST_CARGO_EXPECT_ARGS", "install-update -a -g")

			c := &CargoUpdater{listRunning: tt.listRunning}
			_, err := c.Update(context.Background(), UpdateOptions{})

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrContain)

			if tt.wantNoHint {
				assert.NotContains(t, err.Error(), "実行中の cargo バイナリ")
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

		writeFakeCIUBinary(t, dir)
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

		writeFakeCIUBinary(t, dir)
	}
}

// writeFakeCIUBinary は指定ディレクトリに v20+ スタイルの fake cargo-install-update バイナリを作成します。
// DSX_TEST_CARGO_EXPECT_ARGS が設定されている場合は渡された全引数との一致も検証します。
func writeFakeCIUBinary(t *testing.T, dir string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("dir creation failed: %v", err)
	}

	if runtime.GOOS == "windows" {
		content := "@echo off\r\n" +
			"set mode=%DSX_TEST_CARGO_MODE%\r\n" +
			"if \"%1\"==\"--version\" (\r\n" +
			"  echo cargo 22.1.5\r\n" +
			"  exit /b 0\r\n" +
			")\r\n" +
			"if not \"%DSX_TEST_CARGO_EXPECT_ARGS%\"==\"\" (\r\n" +
			"  if not \"%*\"==\"%DSX_TEST_CARGO_EXPECT_ARGS%\" (\r\n" +
			"    echo unexpected args: %* 1>&2\r\n" +
			"    exit /b 1\r\n" +
			"  )\r\n" +
			")\r\n" +
			"if not \"%1\"==\"install-update\" (\r\n" +
			"  echo invalid args 1>&2\r\n" +
			"  exit /b 1\r\n" +
			")\r\n" +
			"if not \"%2\"==\"-a\" (\r\n" +
			"  echo invalid args 1>&2\r\n" +
			"  exit /b 1\r\n" +
			")\r\n" +
			"if \"%mode%\"==\"update_error\" (\r\n" +
			"  echo cargo install-update failed 1>&2\r\n" +
			"  exit /b 1\r\n" +
			")\r\n" +
			"if \"%mode%\"==\"partial_update_error\" (\r\n" +
			"  echo Updated 0 packages.\r\n" +
			"  echo Updated 1 git package.\r\n" +
			"  echo Overall updated 1 package: agent-statusline.\r\n" +
			"  echo Failed to update broken-crate. 1>&2\r\n" +
			"  exit /b 1\r\n" +
			")\r\n" +
			"if \"%mode%\"==\"updates\" (\r\n" +
			"  echo Overall updated 2 packages.\r\n" +
			")\r\n" +
			"exit /b 0\r\n"
		path := filepath.Join(dir, "cargo-install-update.cmd")

		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("fake cargo-install-update.cmd write failed: %v", err)
		}
	} else {
		content := "#!/bin/sh\n" +
			"mode=\"${DSX_TEST_CARGO_MODE}\"\n" +
			"if [ \"$1\" = \"--version\" ]; then\n" +
			"  echo \"cargo 22.1.5\"\n" +
			"  exit 0\n" +
			"fi\n" +
			"if [ -n \"${DSX_TEST_CARGO_EXPECT_ARGS}\" ] && [ \"$*\" != \"${DSX_TEST_CARGO_EXPECT_ARGS}\" ]; then\n" +
			"  echo \"unexpected args: $*\" 1>&2\n" +
			"  exit 1\n" +
			"fi\n" +
			"if [ \"$1\" != \"install-update\" ] || [ \"$2\" != \"-a\" ]; then\n" +
			"  echo \"invalid args\" 1>&2\n" +
			"  exit 1\n" +
			"fi\n" +
			"if [ \"${mode}\" = \"update_error\" ]; then\n" +
			"  echo \"cargo install-update failed\" 1>&2\n" +
			"  exit 1\n" +
			"fi\n" +
			"if [ \"${mode}\" = \"partial_update_error\" ]; then\n" +
			"  echo \"Updated 0 packages.\"\n" +
			"  echo \"Updated 1 git package.\"\n" +
			"  echo \"Overall updated 1 package: agent-statusline.\"\n" +
			"  echo \"Failed to update broken-crate.\" 1>&2\n" +
			"  exit 1\n" +
			"fi\n" +
			"if [ \"${mode}\" = \"updates\" ]; then\n" +
			"  echo \"Overall updated 2 packages.\"\n" +
			"fi\n" +
			"exit 0\n"
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
// --version は "cargo-install-update <version>"（空なら 16.0.0）を返し、-a 以降を直接受け付けます。
func writeFakeCargoInstallUpdateV19Binary(t *testing.T, dir, ver string) {
	t.Helper()

	if ver == "" {
		ver = "16.0.0"
	}

	if runtime.GOOS == "windows" {
		content := "@echo off\r\n" +
			"set mode=%DSX_TEST_CARGO_MODE%\r\n" +
			"if \"%1\"==\"--version\" (\r\n" +
			"  echo cargo-install-update " + ver + "\r\n" +
			"  exit /b 0\r\n" +
			")\r\n" +
			"if not \"%DSX_TEST_CARGO_EXPECT_ARGS%\"==\"\" (\r\n" +
			"  if not \"%*\"==\"%DSX_TEST_CARGO_EXPECT_ARGS%\" (\r\n" +
			"    echo unexpected args: %* 1>&2\r\n" +
			"    exit /b 1\r\n" +
			"  )\r\n" +
			")\r\n" +
			"if not \"%1\"==\"-a\" (\r\n" +
			"  echo invalid args 1>&2\r\n" +
			"  exit /b 1\r\n" +
			")\r\n" +
			"if \"%mode%\"==\"update_error\" (\r\n" +
			"  echo cargo install-update failed 1>&2\r\n" +
			"  exit /b 1\r\n" +
			")\r\n" +
			"if \"%mode%\"==\"updates\" (\r\n" +
			"  echo Updated 2 packages.\r\n" +
			")\r\n" +
			"exit /b 0\r\n"
		path := filepath.Join(dir, "cargo-install-update.cmd")

		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("writeFakeCargoInstallUpdateV19Binary write failed: %v", err)
		}
	} else {
		content := "#!/bin/sh\n" +
			"mode=\"${DSX_TEST_CARGO_MODE}\"\n" +
			"if [ \"$1\" = \"--version\" ]; then\n" +
			"  echo \"cargo-install-update " + ver + "\"\n" +
			"  exit 0\n" +
			"fi\n" +
			"if [ -n \"${DSX_TEST_CARGO_EXPECT_ARGS}\" ] && [ \"$*\" != \"${DSX_TEST_CARGO_EXPECT_ARGS}\" ]; then\n" +
			"  echo \"unexpected args: $*\" 1>&2\n" +
			"  exit 1\n" +
			"fi\n" +
			"if [ \"$1\" != \"-a\" ]; then\n" +
			"  echo \"invalid args\" 1>&2\n" +
			"  exit 1\n" +
			"fi\n" +
			"if [ \"${mode}\" = \"update_error\" ]; then\n" +
			"  echo \"cargo install-update failed\" 1>&2\n" +
			"  exit 1\n" +
			"fi\n" +
			"if [ \"${mode}\" = \"updates\" ]; then\n" +
			"  echo \"Updated 2 packages.\"\n" +
			"fi\n" +
			"exit 0\n"
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
		name        string
		versionOut  string // --version の出力（空なら exit 1）
		opts        cargoUpdateOptions
		wantArgs    []string
		wantErr     bool
		errContains string
	}{
		{
			name:       "v20+/git無効・locked無効",
			versionOut: "cargo 20.0.0",
			opts:       cargoUpdateOptions{},
			wantArgs:   []string{"install-update", "-a"},
		},
		{
			name:       "v20+/git有効・locked無効",
			versionOut: "cargo 22.1.5",
			opts:       cargoUpdateOptions{includeGit: true},
			wantArgs:   []string{"install-update", "-a", "-g"},
		},
		{
			name:       "v20+/git無効・locked有効",
			versionOut: "cargo 22.1.5",
			opts:       cargoUpdateOptions{locked: true},
			wantArgs:   []string{"install-update", "-a", "--locked"},
		},
		{
			name:       "v20+/git有効・locked有効",
			versionOut: "cargo 22.1.5",
			opts:       cargoUpdateOptions{includeGit: true, locked: true},
			wantArgs:   []string{"install-update", "-a", "-g", "--locked"},
		},
		{
			name:       "v19-/git無効・locked無効",
			versionOut: "cargo-install-update 16.0.0",
			opts:       cargoUpdateOptions{},
			wantArgs:   []string{"-a"},
		},
		{
			name:       "v19-/git有効・locked無効",
			versionOut: "cargo-install-update 16.0.0",
			opts:       cargoUpdateOptions{includeGit: true},
			wantArgs:   []string{"-a", "-g"},
		},
		{
			name:       "v19-/git有効・locked有効（v16.0.0は対応版）",
			versionOut: "cargo-install-update 16.0.0",
			opts:       cargoUpdateOptions{includeGit: true, locked: true},
			wantArgs:   []string{"-a", "-g", "--locked"},
		},
		{
			name:       "v14.1.0は--locked対応の最低バージョン境界",
			versionOut: "cargo-install-update 14.1.0",
			opts:       cargoUpdateOptions{includeGit: true, locked: true},
			wantArgs:   []string{"-a", "-g", "--locked"},
		},
		{
			name:       "v14.0.0でもlocked無効なら正常に実行できる",
			versionOut: "cargo-install-update 14.0.0",
			opts:       cargoUpdateOptions{includeGit: true, locked: false},
			wantArgs:   []string{"-a", "-g"},
		},
		{
			name:        "v14.0.0でlocked有効は事前診断エラー",
			versionOut:  "cargo-install-update 14.0.0",
			opts:        cargoUpdateOptions{includeGit: true, locked: true},
			wantErr:     true,
			errContains: "cargo install cargo-update",
		},
		{
			name:        "v14.0.2でlocked有効は事前診断エラー",
			versionOut:  "cargo-install-update 14.0.2",
			opts:        cargoUpdateOptions{locked: true},
			wantErr:     true,
			errContains: "cargo-update v14.1.0 以上が必要です",
		},
		{
			name:       "--version 失敗時は v20+ 形式にフォールバック（オプション付与あり）",
			versionOut: "", // exit 1
			opts:       cargoUpdateOptions{includeGit: true, locked: true},
			wantArgs:   []string{"install-update", "-a", "-g", "--locked"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			binPath := writeFakeVersionBinary(t, dir, tt.versionOut)
			got, err := cargoUpdateArgs(binPath, tt.opts)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantArgs, got)
		})
	}
}

// writeFakeVersionBinary は --version に対して versionOut を返す fake バイナリを作成し、そのパスを返します。
// versionOut が空の場合は --version で exit 1 を返します。
func writeFakeVersionBinary(t *testing.T, dir, versionOut string) string {
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
