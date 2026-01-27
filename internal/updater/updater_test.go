package updater

import (
	"context"
	"testing"

	"github.com/scottlz0310/devsync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockUpdater はテスト用のモックUpdaterです
type mockUpdater struct {
	name        string
	displayName string
	available   bool
	configErr   error
}

func (m *mockUpdater) Name() string        { return m.name }
func (m *mockUpdater) DisplayName() string { return m.displayName }
func (m *mockUpdater) IsAvailable() bool   { return m.available }

func (m *mockUpdater) Check(_ context.Context) (*CheckResult, error) {
	return &CheckResult{AvailableUpdates: 0}, nil
}

func (m *mockUpdater) Update(_ context.Context, _ UpdateOptions) (*UpdateResult, error) {
	return &UpdateResult{UpdatedCount: 0}, nil
}

func (m *mockUpdater) Configure(_ config.ManagerConfig) error {
	return m.configErr
}

// テスト前にレジストリをクリアするヘルパー
func clearRegistry() {
	globalRegistry.mu.Lock()
	globalRegistry.updaters = make(map[string]Updater)
	globalRegistry.mu.Unlock()
}

func TestRegisterAndGet(t *testing.T) {
	t.Run("正常系: Updaterを登録して取得", func(t *testing.T) {
		clearRegistry()

		mock := &mockUpdater{name: "test", displayName: "Test Manager", available: true}
		Register(mock)

		retrieved, ok := Get("test")
		assert.True(t, ok)
		assert.Equal(t, mock, retrieved)
	})

	t.Run("存在しないUpdaterの取得", func(t *testing.T) {
		clearRegistry()

		_, ok := Get("nonexistent")
		assert.False(t, ok)
	})

	t.Run("同名のUpdaterを登録すると上書き", func(t *testing.T) {
		clearRegistry()

		mock1 := &mockUpdater{name: "test", displayName: "First"}
		mock2 := &mockUpdater{name: "test", displayName: "Second"}

		Register(mock1)
		Register(mock2)

		retrieved, ok := Get("test")
		assert.True(t, ok)
		assert.Equal(t, "Second", retrieved.DisplayName())
	})
}

func TestAll(t *testing.T) {
	t.Run("空のレジストリ", func(t *testing.T) {
		clearRegistry()

		all := All()
		assert.Empty(t, all)
	})

	t.Run("複数のUpdaterを取得", func(t *testing.T) {
		clearRegistry()

		mock1 := &mockUpdater{name: "apt", displayName: "APT"}
		mock2 := &mockUpdater{name: "brew", displayName: "Homebrew"}
		mock3 := &mockUpdater{name: "npm", displayName: "NPM"}

		Register(mock1)
		Register(mock2)
		Register(mock3)

		all := All()
		assert.Len(t, all, 3)

		// 名前のセットを確認
		names := make(map[string]bool)

		for _, u := range all {
			names[u.Name()] = true
		}

		assert.True(t, names["apt"])
		assert.True(t, names["brew"])
		assert.True(t, names["npm"])
	})
}

func TestAvailable(t *testing.T) {
	t.Run("利用可能なUpdaterのみ返す", func(t *testing.T) {
		clearRegistry()

		available1 := &mockUpdater{name: "available1", available: true}
		available2 := &mockUpdater{name: "available2", available: true}
		unavailable := &mockUpdater{name: "unavailable", available: false}

		Register(available1)
		Register(available2)
		Register(unavailable)

		avail := Available()
		assert.Len(t, avail, 2)

		names := make(map[string]bool)

		for _, u := range avail {
			names[u.Name()] = true
		}

		assert.True(t, names["available1"])
		assert.True(t, names["available2"])
		assert.False(t, names["unavailable"])
	})

	t.Run("すべて利用不可の場合は空のスライス", func(t *testing.T) {
		clearRegistry()

		unavailable := &mockUpdater{name: "unavailable", available: false}
		Register(unavailable)

		avail := Available()
		assert.Empty(t, avail)
	})
}

func TestGetEnabled(t *testing.T) {
	t.Run("nilの設定", func(t *testing.T) {
		clearRegistry()

		result, err := GetEnabled(nil)
		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("空のenableリスト", func(t *testing.T) {
		clearRegistry()

		cfg := &config.SysConfig{
			Enable: []string{},
		}

		result, err := GetEnabled(cfg)
		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("有効なUpdaterのみ返す", func(t *testing.T) {
		clearRegistry()

		available := &mockUpdater{name: "apt", available: true}
		unavailable := &mockUpdater{name: "brew", available: false}

		Register(available)
		Register(unavailable)

		cfg := &config.SysConfig{
			Enable:   []string{"apt", "brew"},
			Managers: make(map[string]config.ManagerConfig),
		}

		result, err := GetEnabled(cfg)
		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "apt", result[0].Name())
	})

	t.Run("未知のマネージャが指定された場合はエラー", func(t *testing.T) {
		clearRegistry()

		available := &mockUpdater{name: "apt", available: true}
		Register(available)

		cfg := &config.SysConfig{
			Enable:   []string{"apt", "unknown1", "unknown2"},
			Managers: make(map[string]config.ManagerConfig),
		}

		result, err := GetEnabled(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "未知のマネージャが指定されています")
		assert.Contains(t, err.Error(), "unknown1")
		assert.Contains(t, err.Error(), "unknown2")
		// aptは正常に返される
		assert.Len(t, result, 1)
		assert.Equal(t, "apt", result[0].Name())
	})

	t.Run("設定の適用", func(t *testing.T) {
		clearRegistry()

		mock := &mockUpdater{name: "apt", available: true}
		Register(mock)

		cfg := &config.SysConfig{
			Enable: []string{"apt"},
			Managers: map[string]config.ManagerConfig{
				"apt": {"use_sudo": true},
			},
		}

		result, err := GetEnabled(cfg)
		require.NoError(t, err)
		assert.Len(t, result, 1)
	})

	t.Run("設定適用時のエラー", func(t *testing.T) {
		clearRegistry()

		mock := &mockUpdater{
			name:      "apt",
			available: true,
			configErr: assert.AnError,
		}
		Register(mock)

		cfg := &config.SysConfig{
			Enable: []string{"apt"},
			Managers: map[string]config.ManagerConfig{
				"apt": {"invalid": "config"},
			},
		}

		_, err := GetEnabled(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "apt の設定適用に失敗")
	})
}
