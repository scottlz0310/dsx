package updater

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScoopParseStatusOutput(t *testing.T) {
	s := &ScoopUpdater{}

	tests := []struct {
		name     string
		input    string
		expected []PackageInfo
	}{
		{
			name: "複数パッケージの更新あり",
			input: `Scoop is up to date.
Name              Installed Version   Latest Version   Missing Dependencies   Info
----              -----------------   --------------   --------------------   ----
git               2.34.1              2.38.0
nodejs            16.13.0             18.9.0
python            3.9.7               3.10.4
`,
			expected: []PackageInfo{
				{Name: "git", CurrentVersion: "2.34.1", NewVersion: "2.38.0"},
				{Name: "nodejs", CurrentVersion: "16.13.0", NewVersion: "18.9.0"},
				{Name: "python", CurrentVersion: "3.9.7", NewVersion: "3.10.4"},
			},
		},
		{
			name: "更新なし",
			input: `Scoop is up to date.
Everything is ok!
`,
			expected: nil,
		},
		{
			name:     "空出力",
			input:    "",
			expected: nil,
		},
		{
			name: "1件のみ",
			input: `Name              Installed Version   Latest Version
----              -----------------   --------------
git               2.34.1              2.38.0
`,
			expected: []PackageInfo{
				{Name: "git", CurrentVersion: "2.34.1", NewVersion: "2.38.0"},
			},
		},
		{
			name: "Missing Dependencies と Info カラムあり",
			input: `Name              Installed Version   Latest Version   Missing Dependencies   Info
----              -----------------   --------------   --------------------   ----
7zip              21.07               22.01                                   Version changed
git               2.34.1              2.38.0
`,
			expected: []PackageInfo{
				{Name: "7zip", CurrentVersion: "21.07", NewVersion: "22.01"},
				{Name: "git", CurrentVersion: "2.34.1", NewVersion: "2.38.0"},
			},
		},
		{
			name: "WARN メッセージを含む出力",
			input: `WARN  Scoop bucket(s) out of date. Run 'scoop update' to get the latest changes.
Name              Installed Version   Latest Version
----              -----------------   --------------
git               2.34.1              2.38.0
`,
			expected: []PackageInfo{
				{Name: "git", CurrentVersion: "2.34.1", NewVersion: "2.38.0"},
			},
		},
		{
			name: "ヘッダーのみ・データなし",
			input: `Name              Installed Version   Latest Version
----              -----------------   --------------
`,
			expected: []PackageInfo{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.parseStatusOutput(tt.input)

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

func TestIsScoopSeparator(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"ダッシュのみ", "----------", true},
		{"ダッシュとスペース", "----  -----  ----", true},
		{"文字を含む", "---a---", false},
		{"スペースのみ", "     ", false},
		{"空文字列", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isScoopSeparator(tt.input)
			if got != tt.expected {
				t.Errorf("isScoopSeparator(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestDetectScoopColumnPositions(t *testing.T) {
	tests := []struct {
		name        string
		header      string
		minExpected int
	}{
		{
			name:        "通常のヘッダー",
			header:      "Name              Installed Version   Latest Version",
			minExpected: 3,
		},
		{
			name:        "5カラム",
			header:      "Name              Installed Version   Latest Version   Missing Dependencies   Info",
			minExpected: 5,
		},
		{
			name:        "1カラム",
			header:      "Name",
			minExpected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectScoopColumnPositions(tt.header)
			if len(got) < tt.minExpected {
				t.Errorf("カラム位置数: got %d, want >= %d (positions: %v)", len(got), tt.minExpected, got)
			}
		})
	}
}

func TestScoopUpdater_Check(t *testing.T) {
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
			name:        "scoop update 失敗",
			mode:        "update_bucket_error",
			wantUpdates: 0,
			wantErr:     true,
			errContains: "scoop update の実行に失敗",
		},
		{
			name:        "scoop status 失敗（出力なし）",
			mode:        "status_error",
			wantUpdates: 0,
			wantErr:     true,
			errContains: "scoop status の実行に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeScoopCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_SCOOP_MODE", tc.mode)

			s := &ScoopUpdater{}
			got, err := s.Check(context.Background())

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

func TestScoopUpdater_Update(t *testing.T) {
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
			msgContains: "すべての Scoop パッケージは最新です",
		},
		{
			name:        "更新成功",
			mode:        "updates",
			opts:        UpdateOptions{},
			wantUpdated: 2,
			wantErr:     false,
			msgContains: "2 件の Scoop パッケージを更新しました",
		},
		{
			name:        "更新失敗",
			mode:        "update_all_error",
			opts:        UpdateOptions{},
			wantUpdated: 0,
			wantErr:     true,
			errContains: "scoop update --all に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			writeFakeScoopCommand(t, fakeDir)

			t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("DEVSYNC_TEST_SCOOP_MODE", tc.mode)

			s := &ScoopUpdater{}
			got, err := s.Update(context.Background(), tc.opts)

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

func writeFakeScoopCommand(t *testing.T, dir string) {
	t.Helper()

	var (
		fileName string
		content  string
	)

	if runtime.GOOS == "windows" {
		fileName = "scoop.cmd"
		content = `@echo off
set mode=%DEVSYNC_TEST_SCOOP_MODE%
if "%1"=="status" goto dostatus
if not "%1"=="update" (
  echo invalid args 1>&2
  exit /b 1
)
if "%2"=="--all" goto doall
if "%mode%"=="update_bucket_error" (
  echo scoop update failed 1>&2
  exit /b 1
)
echo Scoop was updated successfully!
exit /b 0
:doall
if "%mode%"=="update_all_error" (
  echo scoop update --all failed 1>&2
  exit /b 1
)
exit /b 0
:dostatus
if "%mode%"=="status_error" (
  exit /b 1
)
if "%mode%"=="none" (
  echo Everything is ok!
  exit /b 0
)
echo Name              Installed Version   Latest Version
echo ----              -----------------   --------------
echo git               2.34.1              2.38.0
echo nodejs            16.13.0             18.9.0
exit /b 0
`
	} else {
		fileName = "scoop"
		content = `#!/bin/sh
mode="${DEVSYNC_TEST_SCOOP_MODE}"
if [ "$1" = "update" ]; then
  # --all 付きなら実際の更新
  has_all=false
  for arg in "$@"; do
    if [ "$arg" = "--all" ]; then
      has_all=true
    fi
  done
  if [ "${has_all}" = "true" ]; then
    if [ "${mode}" = "update_all_error" ]; then
      echo "scoop update --all failed" 1>&2
      exit 1
    fi
    exit 0
  fi
  # バケット更新
  if [ "${mode}" = "update_bucket_error" ]; then
    echo "scoop update failed" 1>&2
    exit 1
  fi
  echo "Scoop was updated successfully!"
  exit 0
fi
if [ "$1" = "status" ]; then
  if [ "${mode}" = "status_error" ]; then
    exit 1
  fi
  if [ "${mode}" = "none" ]; then
    echo "Everything is ok!"
    exit 0
  fi
  echo "Name              Installed Version   Latest Version"
  echo "----              -----------------   --------------"
  echo "git               2.34.1              2.38.0"
  echo "nodejs            16.13.0             18.9.0"
  exit 0
fi
echo "invalid args" 1>&2
exit 1
`
	}

	fullPath := filepath.Join(dir, fileName)
	if err := os.WriteFile(fullPath, []byte(content), 0o755); err != nil {
		t.Fatalf("fake scoop command write failed: %v", err)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(fullPath, 0o755); err != nil {
			t.Fatalf("fake scoop command chmod failed: %v", err)
		}
	}
}
