package updater

import (
	"testing"

	"github.com/scottlz0310/devsync/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestAptUpdater_Name(t *testing.T) {
	apt := &AptUpdater{}
	assert.Equal(t, "apt", apt.Name())
}

func TestAptUpdater_DisplayName(t *testing.T) {
	apt := &AptUpdater{}
	assert.Equal(t, "APT (Debian/Ubuntu)", apt.DisplayName())
}

func TestAptUpdater_Configure(t *testing.T) {
	tests := []struct {
		name        string
		cfg         config.ManagerConfig
		expectSudo  bool
		description string
	}{
		{
			name:        "nilの設定",
			cfg:         nil,
			expectSudo:  false, // デフォルト値
			description: "nil設定の場合はデフォルトのまま",
		},
		{
			name:        "空の設定",
			cfg:         config.ManagerConfig{},
			expectSudo:  false,
			description: "空の設定の場合はデフォルトのまま",
		},
		{
			name:        "use_sudo=true",
			cfg:         config.ManagerConfig{"use_sudo": true},
			expectSudo:  true,
			description: "use_sudoがtrueの場合はsudoを使用",
		},
		{
			name:        "use_sudo=false",
			cfg:         config.ManagerConfig{"use_sudo": false},
			expectSudo:  false,
			description: "use_sudoがfalseの場合はsudoを使用しない",
		},
		{
			name:        "不正な型の値",
			cfg:         config.ManagerConfig{"use_sudo": "true"}, // stringは無視される
			expectSudo:  false,
			description: "不正な型の値は無視される",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apt := &AptUpdater{useSudo: false}
			err := apt.Configure(tt.cfg)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectSudo, apt.useSudo, tt.description)
		})
	}
}

func TestAptUpdater_parseUpgradableList(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected []PackageInfo
	}{
		{
			name:     "空の出力",
			output:   "",
			expected: []PackageInfo{},
		},
		{
			name:     "Listingヘッダーのみ",
			output:   "Listing... Done",
			expected: []PackageInfo{},
		},
		{
			name: "単一のパッケージ",
			output: `Listing... Done
vim/jammy-updates 2:8.2.3995-1ubuntu2.11 amd64 [upgradable from: 2:8.2.3995-1ubuntu2.10]`,
			expected: []PackageInfo{
				{Name: "vim", NewVersion: "2:8.2.3995-1ubuntu2.11", CurrentVersion: "2:8.2.3995-1ubuntu2.10"},
			},
		},
		{
			name: "複数のパッケージ",
			output: `Listing... Done
vim/jammy-updates 2:8.2.3995-1ubuntu2.11 amd64 [upgradable from: 2:8.2.3995-1ubuntu2.10]
curl/jammy-security 7.81.0-1ubuntu1.14 amd64 [upgradable from: 7.81.0-1ubuntu1.13]
git/jammy-updates 1:2.34.1-1ubuntu1.10 amd64 [upgradable from: 1:2.34.1-1ubuntu1.9]`,
			expected: []PackageInfo{
				{Name: "vim", NewVersion: "2:8.2.3995-1ubuntu2.11", CurrentVersion: "2:8.2.3995-1ubuntu2.10"},
				{Name: "curl", NewVersion: "7.81.0-1ubuntu1.14", CurrentVersion: "7.81.0-1ubuntu1.13"},
				{Name: "git", NewVersion: "1:2.34.1-1ubuntu1.10", CurrentVersion: "1:2.34.1-1ubuntu1.9"},
			},
		},
		{
			name: "空行を含む",
			output: `Listing... Done

vim/jammy-updates 2:8.2.3995-1ubuntu2.11 amd64 [upgradable from: 2:8.2.3995-1ubuntu2.10]

curl/jammy-security 7.81.0-1ubuntu1.14 amd64 [upgradable from: 7.81.0-1ubuntu1.13]
`,
			expected: []PackageInfo{
				{Name: "vim", NewVersion: "2:8.2.3995-1ubuntu2.11", CurrentVersion: "2:8.2.3995-1ubuntu2.10"},
				{Name: "curl", NewVersion: "7.81.0-1ubuntu1.14", CurrentVersion: "7.81.0-1ubuntu1.13"},
			},
		},
		{
			name: "古いバージョン情報がない場合",
			output: `Listing... Done
vim/jammy-updates 2:8.2.3995-1ubuntu2.11 amd64`,
			expected: []PackageInfo{
				{Name: "vim", NewVersion: "2:8.2.3995-1ubuntu2.11", CurrentVersion: ""},
			},
		},
		{
			name: "パッケージ名にスラッシュがない場合（不正な形式）",
			output: `Listing... Done
invalid-line-without-space`,
			expected: []PackageInfo{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apt := &AptUpdater{}
			result := apt.parseUpgradableList(tt.output)

			assert.Len(t, result, len(tt.expected))

			for i, expected := range tt.expected {
				assert.Equal(t, expected.Name, result[i].Name, "Package name mismatch at index %d", i)
				assert.Equal(t, expected.NewVersion, result[i].NewVersion, "New version mismatch at index %d", i)
				assert.Equal(t, expected.CurrentVersion, result[i].CurrentVersion, "Current version mismatch at index %d", i)
			}
		})
	}
}
