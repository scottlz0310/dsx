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

func TestBrewUpdater_Name(t *testing.T) {
	brew := &BrewUpdater{}
	assert.Equal(t, "brew", brew.Name())
}

func TestBrewUpdater_DisplayName(t *testing.T) {
	brew := &BrewUpdater{}
	assert.Equal(t, "Homebrew", brew.DisplayName())
}

func TestBrewUpdater_Configure(t *testing.T) {
	tests := []struct {
		name          string
		cfg           config.ManagerConfig
		expectCleanup bool
		expectGreedy  bool
		description   string
	}{
		{
			name:          "nilの設定",
			cfg:           nil,
			expectCleanup: false, // デフォルト値
			expectGreedy:  false,
			description:   "nil設定の場合はデフォルトのまま",
		},
		{
			name:          "空の設定",
			cfg:           config.ManagerConfig{},
			expectCleanup: false,
			expectGreedy:  false,
			description:   "空の設定の場合はデフォルトのまま",
		},
		{
			name:          "cleanup=true",
			cfg:           config.ManagerConfig{"cleanup": true},
			expectCleanup: true,
			expectGreedy:  false,
			description:   "cleanupがtrueの場合はクリーンアップを有効化",
		},
		{
			name:          "greedy=true",
			cfg:           config.ManagerConfig{"greedy": true},
			expectCleanup: false,
			expectGreedy:  true,
			description:   "greedyがtrueの場合はauto_updates caskも更新対象",
		},
		{
			name:          "両方設定",
			cfg:           config.ManagerConfig{"cleanup": true, "greedy": true},
			expectCleanup: true,
			expectGreedy:  true,
			description:   "複数の設定を同時に適用",
		},
		{
			name:          "不正な型の値",
			cfg:           config.ManagerConfig{"cleanup": "true", "greedy": 1},
			expectCleanup: false,
			expectGreedy:  false,
			description:   "不正な型の値は無視される",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			brew := &BrewUpdater{cleanup: false, greedy: false}
			err := brew.Configure(tt.cfg)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectCleanup, brew.cleanup, tt.description+" (cleanup)")
			assert.Equal(t, tt.expectGreedy, brew.greedy, tt.description+" (greedy)")
		})
	}
}

func TestBrewUpdater_parseOutdatedList(t *testing.T) {
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
			name:     "空白行のみ",
			output:   "   \n  \n   ",
			expected: nil,
		},
		{
			name:   "単一のパッケージ（シンプル形式）",
			output: `node (18.17.0) < 20.5.0`,
			expected: []PackageInfo{
				{Name: "node", CurrentVersion: "18.17.0", NewVersion: "20.5.0"},
			},
		},
		{
			name: "複数のパッケージ",
			output: `node (18.17.0) < 20.5.0
python@3.11 (3.11.4) < 3.11.5
git (2.40.0) < 2.41.0`,
			expected: []PackageInfo{
				{Name: "node", CurrentVersion: "18.17.0", NewVersion: "20.5.0"},
				{Name: "python@3.11", CurrentVersion: "3.11.4", NewVersion: "3.11.5"},
				{Name: "git", CurrentVersion: "2.40.0", NewVersion: "2.41.0"},
			},
		},
		{
			name: "!=形式のバージョン",
			// NOTE: パーサーはIndexAny("<!=")を使うため、!=の場合は=が含まれる（既存の挙動）
			output: `vim (8.2.3500) != 9.0.1000`,
			expected: []PackageInfo{
				{Name: "vim", CurrentVersion: "8.2.3500", NewVersion: "= 9.0.1000"},
			},
		},
		{
			name:   "バージョン情報のみ（新バージョンなし）",
			output: `mypackage (1.0.0)`,
			expected: []PackageInfo{
				{Name: "mypackage", CurrentVersion: "1.0.0", NewVersion: ""},
			},
		},
		{
			name:   "パッケージ名のみ",
			output: `simplepackage`,
			expected: []PackageInfo{
				{Name: "simplepackage", CurrentVersion: "", NewVersion: ""},
			},
		},
		{
			name: "空行を含む",
			output: `node (18.17.0) < 20.5.0

python@3.11 (3.11.4) < 3.11.5
`,
			expected: []PackageInfo{
				{Name: "node", CurrentVersion: "18.17.0", NewVersion: "20.5.0"},
				{Name: "python@3.11", CurrentVersion: "3.11.4", NewVersion: "3.11.5"},
			},
		},
		{
			name: "Cask形式のパッケージ（<形式）",
			output: `google-chrome (114.0.5735.198) < 115.0.5790.102
visual-studio-code (1.79.2) < 1.80.0`,
			expected: []PackageInfo{
				{Name: "google-chrome", CurrentVersion: "114.0.5735.198", NewVersion: "115.0.5790.102"},
				{Name: "visual-studio-code", CurrentVersion: "1.79.2", NewVersion: "1.80.0"},
			},
		},
		{
			name: "複雑なバージョン番号（<形式）",
			output: `openssl@3 (3.1.1_1) < 3.1.2
python@3.11 (3.11.4_1) < 3.11.5`,
			expected: []PackageInfo{
				{Name: "openssl@3", CurrentVersion: "3.1.1_1", NewVersion: "3.1.2"},
				{Name: "python@3.11", CurrentVersion: "3.11.4_1", NewVersion: "3.11.5"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			brew := &BrewUpdater{}
			result := brew.parseOutdatedList(tt.output)

			if tt.expected == nil {
				assert.Empty(t, result)
				return
			}

			assert.Len(t, result, len(tt.expected))

			for i, expected := range tt.expected {
				assert.Equal(t, expected.Name, result[i].Name, "Package name mismatch at index %d", i)
				assert.Equal(t, expected.CurrentVersion, result[i].CurrentVersion, "Current version mismatch at index %d", i)
				assert.Equal(t, expected.NewVersion, result[i].NewVersion, "New version mismatch at index %d", i)
			}
		})
	}
}

func TestBrewUpdater_Check(t *testing.T) {
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
			name:        "brew update 失敗",
			mode:        "update_error",
			wantUpdates: 0,
			wantErr:     true,
			errContains: "brew update に失敗",
		},
		{
			name:        "brew outdated 失敗",
			mode:        "outdated_error",
			wantUpdates: 0,
			wantErr:     true,
			errContains: "brew outdated に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeBrewCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_BREW_MODE", tc.mode)

			b := &BrewUpdater{}
			got, err := b.Check(context.Background())

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

func TestBrewUpdater_Update(t *testing.T) {
	testCases := []struct {
		name        string
		mode        string
		opts        UpdateOptions
		wantUpdated int
		wantErr     bool
		errContains string
		msgContains string
		wantErrors  int // result.Errors の件数
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
			msgContains: "すべてのパッケージは最新です",
		},
		{
			name:        "更新成功",
			mode:        "updates",
			opts:        UpdateOptions{},
			wantUpdated: 2,
			wantErr:     false,
			msgContains: "2 件のパッケージを更新しました",
		},
		{
			name:        "更新失敗",
			mode:        "upgrade_error",
			opts:        UpdateOptions{},
			wantUpdated: 2, // Check は成功するため UpdatedCount は設定される
			wantErr:     false,
			wantErrors:  2, // brew upgrade と brew upgrade --cask の両方が失敗
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeBrewCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_BREW_MODE", tc.mode)

			b := &BrewUpdater{}
			got, err := b.Update(context.Background(), tc.opts)

			if tc.wantErr {
				assert.Error(t, err)

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

			if tc.wantErrors > 0 {
				assert.Len(t, got.Errors, tc.wantErrors)
			}
		})
	}
}

func writeFakeBrewCommand(t *testing.T, dir string) {
	t.Helper()

	var (
		fileName string
		content  string
	)

	if runtime.GOOS == "windows" {
		fileName = "brew.cmd"
		content = `@echo off
set mode=%DEVSYNC_TEST_BREW_MODE%
if "%1"=="update" goto doupdate
if "%1"=="outdated" goto dooutdated
if "%1"=="upgrade" goto doupgrade
if "%1"=="cleanup" goto docleanup
echo invalid args 1>&2
exit /b 1
:doupdate
if "%mode%"=="update_error" (
  echo brew update failed 1>&2
  exit /b 1
)
exit /b 0
:dooutdated
if "%mode%"=="outdated_error" (
  echo brew outdated failed 1>&2
  exit /b 1
)
if "%mode%"=="none" (
  exit /b 0
)
echo vim (8.2) ^< 9.0
echo curl (7.88) ^< 8.5
exit /b 0
:doupgrade
if "%mode%"=="upgrade_error" (
  echo brew upgrade failed 1>&2
  exit /b 1
)
exit /b 0
:docleanup
exit /b 0
`
	} else {
		fileName = "brew"
		content = `#!/bin/sh
mode="${DEVSYNC_TEST_BREW_MODE}"
case "$1" in
  update)
    if [ "${mode}" = "update_error" ]; then
      echo "brew update failed" 1>&2; exit 1
    fi
    exit 0 ;;
  outdated)
    if [ "${mode}" = "outdated_error" ]; then
      echo "brew outdated failed" 1>&2; exit 1
    fi
    if [ "${mode}" = "none" ]; then exit 0; fi
    echo "vim (8.2) < 9.0"
    echo "curl (7.88) < 8.5"
    exit 0 ;;
  upgrade)
    if [ "${mode}" = "upgrade_error" ]; then
      echo "brew upgrade failed" 1>&2; exit 1
    fi
    exit 0 ;;
  cleanup)
    exit 0 ;;
  *)
    echo "invalid args" 1>&2; exit 1 ;;
esac
`
	}

	fullPath := filepath.Join(dir, fileName)
	if err := os.WriteFile(fullPath, []byte(content), 0o755); err != nil {
		t.Fatalf("fake brew command write failed: %v", err)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(fullPath, 0o755); err != nil {
			t.Fatalf("fake brew command chmod failed: %v", err)
		}
	}
}
