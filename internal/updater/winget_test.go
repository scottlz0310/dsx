package updater

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWingetParseUpgradeOutput(t *testing.T) {
	w := &WingetUpdater{}

	tests := []struct {
		name     string
		input    string
		expected []PackageInfo
	}{
		{
			name: "英語出力・複数パッケージ",
			input: `Name                                   ID                               Version              Available            Source
-----------------------------------------------------------------------------------------------------------------------
Docker Desktop                         Docker.DockerDesktop             4.59.0               4.60.0               winget
GitHub CLI                             GitHub.cli                       2.83.2               2.86.0               winget
Go Programming Language amd64 go1.25.6 GoLang.Go                        1.25.6               1.26.0               winget
3 upgrades available.
`,
			expected: []PackageInfo{
				{Name: "Docker Desktop", CurrentVersion: "4.59.0", NewVersion: "4.60.0"},
				{Name: "GitHub CLI", CurrentVersion: "2.83.2", NewVersion: "2.86.0"},
				{Name: "Go Programming Language amd64 go1.25.6", CurrentVersion: "1.25.6", NewVersion: "1.26.0"},
			},
		},
		{
			name: "日本語ヘッダー出力",
			input: `名前                                   ID                               バージョン           利用可能            ソース
-----------------------------------------------------------------------------------------------------------------------
Docker Desktop                         Docker.DockerDesktop             4.59.0               4.60.0              winget
9 アップグレードを利用できます。
`,
			expected: []PackageInfo{
				{Name: "Docker Desktop", CurrentVersion: "4.59.0", NewVersion: "4.60.0"},
			},
		},
		{
			name:     "更新なし",
			input:    "No applicable upgrade found.\n",
			expected: nil,
		},
		{
			name:     "空出力",
			input:    "",
			expected: nil,
		},
		{
			name:     "ヘッダーのみ・データ行なし",
			input:    "Name   ID   Version   Available   Source\n-----------------------------------------\n",
			expected: []PackageInfo{},
		},
		{
			name: "1件のみ",
			input: `Name          ID               Version   Available   Source
-------------------------------------------------------------
Bitwarden CLI Bitwarden.CLI    2025.12.0 2026.1.0    winget
1 upgrades available.
`,
			expected: []PackageInfo{
				{Name: "Bitwarden CLI", CurrentVersion: "2025.12.0", NewVersion: "2026.1.0"},
			},
		},
		{
			name: "バージョンにプレフィックスを含む",
			input: `Name                                   ID                               Version              Available            Source
-----------------------------------------------------------------------------------------------------------------------
Microsoft Teams                        Microsoft.Teams                  25332.1210.4188.1171 26005.204.4249.1621  winget
1 upgrades available.
`,
			expected: []PackageInfo{
				{Name: "Microsoft Teams", CurrentVersion: "25332.1210.4188.1171", NewVersion: "26005.204.4249.1621"},
			},
		},
		{
			name: "プログレスバーを含む出力",
			input: `█████████████████████████████████████
Name   ID          Version Available Source
-------------------------------------------
App1   App.One     1.0.0   2.0.0     winget
1 upgrades available.
`,
			expected: []PackageInfo{
				{Name: "App1", CurrentVersion: "1.0.0", NewVersion: "2.0.0"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := w.parseUpgradeOutput(tt.input)

			if len(got) != len(tt.expected) {
				t.Fatalf("パッケージ数が不一致: got %d, want %d\ngot: %+v", len(got), len(tt.expected), got)
			}

			for i, pkg := range got {
				exp := tt.expected[i]
				if pkg.Name != exp.Name {
					t.Errorf("[%d] Name: got %q, want %q", i, pkg.Name, exp.Name)
				}

				if pkg.CurrentVersion != exp.CurrentVersion {
					t.Errorf("[%d] CurrentVersion: got %q, want %q", i, pkg.CurrentVersion, exp.CurrentVersion)
				}

				if pkg.NewVersion != exp.NewVersion {
					t.Errorf("[%d] NewVersion: got %q, want %q", i, pkg.NewVersion, exp.NewVersion)
				}
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "通常の行",
			input:    "line1\nline2\nline3",
			expected: 3,
		},
		{
			name:     "プログレスバーを含む行は除去される",
			input:    "line1\n██████████\nline2",
			expected: 2,
		},
		{
			name:     "空入力",
			input:    "",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLines(tt.input)
			if len(got) != tt.expected {
				t.Errorf("行数: got %d, want %d", len(got), tt.expected)
			}
		})
	}
}

func TestIsSummaryLine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"英語サマリー", "3 upgrades available.", true},
		{"日本語サマリー", "9 アップグレードを利用できます。", true},
		{"No applicable", "No applicable upgrade found.", true},
		{"通常のデータ行", "Docker Desktop Docker.DockerDesktop 4.59.0 4.60.0 winget", false},
		{"空行", "", false},
		{"数字始まりだが関係ない", "3 packages installed", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSummaryLine(tt.input)
			if got != tt.expected {
				t.Errorf("isSummaryLine(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestDetectColumnPositions(t *testing.T) {
	tests := []struct {
		name        string
		header      string
		minExpected int
	}{
		{
			name:        "英語ヘッダー",
			header:      "Name                                   ID                               Version              Available            Source",
			minExpected: 5,
		},
		{
			name:        "短いヘッダー",
			header:      "Name ID Version Available Source",
			minExpected: 5,
		},
		{
			name:        "カラム1つ",
			header:      "Name",
			minExpected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectColumnPositions(tt.header)
			if len(got) < tt.minExpected {
				t.Errorf("カラム位置数: got %d, want >= %d (positions: %v)", len(got), tt.minExpected, got)
			}
		})
	}
}

func TestIsAllDashes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"ダッシュのみ", "----------", true},
		{"ダッシュとスペース", "--- ---", false},
		{"空文字列", "", true},
		{"混在", "--a--", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAllDashes(tt.input)
			if got != tt.expected {
				t.Errorf("isAllDashes(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSafeSubstring(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		start    int
		end      int
		expected string
	}{
		{"通常", "hello world", 0, 5, "hello"},
		{"start超過", "hello", 10, 15, ""},
		{"end超過", "hello", 0, 20, "hello"},
		{"空文字列", "", 0, 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := safeSubstring(tt.s, tt.start, tt.end)
			if got != tt.expected {
				t.Errorf("safeSubstring(%q, %d, %d) = %q, want %q", tt.s, tt.start, tt.end, got, tt.expected)
			}
		})
	}
}

func TestContainsProgressChars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"ブロック文字を含む", "Installing █████████", true},
		{"通常テキスト", "Normal text", false},
		{"空文字列", "", false},
		{"━を含む", "Progress ━━━━━", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsProgressChars(tt.input)
			if got != tt.expected {
				t.Errorf("containsProgressChars(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestWingetUpdater_Check(t *testing.T) {
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
			errContains: "winget upgrade の実行に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeWingetCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_WINGET_MODE", tc.mode)

			w := &WingetUpdater{}
			got, err := w.Check(context.Background())

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

func TestWingetUpdater_Update(t *testing.T) {
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
			msgContains: "すべての winget パッケージは最新です",
		},
		{
			name:        "更新成功",
			mode:        "updates",
			opts:        UpdateOptions{},
			wantUpdated: 2,
			wantErr:     false,
			msgContains: "2 件の winget パッケージを更新しました",
		},
		{
			name:        "更新失敗",
			mode:        "update_error",
			opts:        UpdateOptions{},
			wantUpdated: 0,
			wantErr:     true,
			errContains: "winget upgrade --all に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeWingetCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_WINGET_MODE", tc.mode)

			w := &WingetUpdater{}
			got, err := w.Update(context.Background(), tc.opts)

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

func writeFakeWingetCommand(t *testing.T, dir string) {
	t.Helper()

	var (
		fileName string
		content  string
	)

	if runtime.GOOS == "windows" {
		fileName = "winget.cmd"
		content = `@echo off
set mode=%DEVSYNC_TEST_WINGET_MODE%
if not "%1"=="upgrade" (
  echo invalid args 1>&2
  exit /b 1
)
if "%2"=="--all" goto doall
if "%mode%"=="check_error" (
  echo winget upgrade failed 1>&2
  exit /b 1
)
if "%mode%"=="none" (
  echo No installed package found matching input criteria.
  exit /b 0
)
echo Name                ID                  Version   Available  Source
echo ------------------------------------------------------------------
echo Docker Desktop      Docker.DockerDesktop 4.59.0    4.60.0     winget
echo GitHub CLI          GitHub.cli           2.83.2    2.86.0     winget
echo 2 upgrades available.
exit /b 0
:doall
if "%mode%"=="update_error" (
  echo winget upgrade --all failed 1>&2
  exit /b 1
)
exit /b 0
`
	} else {
		fileName = "winget"
		content = `#!/bin/sh
mode="${DEVSYNC_TEST_WINGET_MODE}"
if [ "$1" != "upgrade" ]; then
  echo "invalid args" 1>&2
  exit 1
fi
# Check モード（--all なし）と Update モード（--all あり）を区別
has_all=false
for arg in "$@"; do
  if [ "$arg" = "--all" ]; then
    has_all=true
  fi
done
if [ "${has_all}" = "true" ]; then
  if [ "${mode}" = "update_error" ]; then
    echo "winget upgrade --all failed" 1>&2
    exit 1
  fi
  exit 0
fi
# Check モード
if [ "${mode}" = "check_error" ]; then
  echo "winget upgrade failed" 1>&2
  exit 1
fi
if [ "${mode}" = "none" ]; then
  echo "No installed package found matching input criteria."
  exit 0
fi
echo "Name                ID                  Version   Available  Source"
echo "------------------------------------------------------------------"
echo "Docker Desktop      Docker.DockerDesktop 4.59.0    4.60.0     winget"
echo "GitHub CLI          GitHub.cli           2.83.2    2.86.0     winget"
echo "2 upgrades available."
exit 0
`
	}

	fullPath := filepath.Join(dir, fileName)
	if err := os.WriteFile(fullPath, []byte(content), 0o755); err != nil {
		t.Fatalf("fake winget command write failed: %v", err)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(fullPath, 0o755); err != nil {
			t.Fatalf("fake winget command chmod failed: %v", err)
		}
	}
}
