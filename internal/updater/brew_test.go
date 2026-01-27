package updater

import (
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
