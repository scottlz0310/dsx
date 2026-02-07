package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefault(t *testing.T) {
	t.Run("デフォルト設定の構造を検証", func(t *testing.T) {
		cfg := Default()

		// Version
		assert.Equal(t, 1, cfg.Version)

		// Control defaults
		assert.Equal(t, 8, cfg.Control.Concurrency)
		assert.Equal(t, "10m", cfg.Control.Timeout)
		assert.False(t, cfg.Control.DryRun)

		// Repo defaults
		home, err := os.UserHomeDir()
		require.NoError(t, err)

		expectedRoot := filepath.Join(home, "src")
		if home == "" {
			expectedRoot = "./src"
		}

		assert.Equal(t, expectedRoot, cfg.Repo.Root)
		assert.Equal(t, "https", cfg.Repo.GitHub.Protocol)
		assert.True(t, cfg.Repo.Sync.AutoStash)
		assert.True(t, cfg.Repo.Sync.Prune)
		assert.True(t, cfg.Repo.Sync.SubmoduleUpdate)
		assert.True(t, cfg.Repo.Cleanup.Enabled)
		assert.Contains(t, cfg.Repo.Cleanup.Target, "merged")
		assert.Contains(t, cfg.Repo.Cleanup.ExcludeBranches, "main")

		// Sys defaults
		assert.Empty(t, cfg.Sys.Enable)
		assert.NotNil(t, cfg.Sys.Managers)

		// Secrets defaults
		assert.False(t, cfg.Secrets.Enabled)
		assert.Equal(t, "bitwarden", cfg.Secrets.Provider)
	})
}

func TestLoad(t *testing.T) {
	t.Run("設定ファイルがない場合はデフォルト値で成功", func(t *testing.T) {
		// 一時ディレクトリを作成して HOME を変更
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		// currentConfigをリセット
		currentConfig = nil

		cfg, err := Load()
		require.NoError(t, err)
		require.NotNil(t, cfg)

		// デフォルト値が設定されていることを確認
		assert.Equal(t, 1, cfg.Version)
		assert.Equal(t, 8, cfg.Control.Concurrency)
	})

	t.Run("有効なYAML設定ファイルの読み込み", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		// 設定ファイルを作成
		configDir := filepath.Join(tmpDir, ".config", "devsync")
		err := os.MkdirAll(configDir, 0o755)
		require.NoError(t, err)

		configContent := `
version: 2
control:
  concurrency: 16
  timeout: "30m"
  dry_run: true
repo:
  root: /custom/path
  github:
    protocol: ssh
  sync:
    auto_stash: false
sys:
  enable:
    - apt
    - brew
secrets:
  enabled: true
  provider: bitwarden
`
		err = os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configContent), 0o644)
		require.NoError(t, err)

		// currentConfigをリセット
		currentConfig = nil

		cfg, err := Load()
		require.NoError(t, err)
		require.NotNil(t, cfg)

		// カスタム値が読み込まれていることを確認
		assert.Equal(t, 2, cfg.Version)
		assert.Equal(t, 16, cfg.Control.Concurrency)
		assert.Equal(t, "30m", cfg.Control.Timeout)
		assert.True(t, cfg.Control.DryRun)
		assert.Equal(t, "/custom/path", cfg.Repo.Root)
		assert.Equal(t, "ssh", cfg.Repo.GitHub.Protocol)
		assert.False(t, cfg.Repo.Sync.AutoStash)
		assert.True(t, cfg.Repo.Sync.SubmoduleUpdate)
		assert.Contains(t, cfg.Sys.Enable, "apt")
		assert.Contains(t, cfg.Sys.Enable, "brew")
		assert.True(t, cfg.Secrets.Enabled)
	})

	t.Run("不正なYAMLの場合はエラー", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		// 不正なYAMLファイルを作成
		configDir := filepath.Join(tmpDir, ".config", "devsync")
		err := os.MkdirAll(configDir, 0o755)
		require.NoError(t, err)

		invalidContent := `
version: [invalid
control:
  - this is wrong
`
		err = os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(invalidContent), 0o644)
		require.NoError(t, err)

		// currentConfigをリセット
		currentConfig = nil

		_, err = Load()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read config file")
	})

	t.Run("環境変数による設定の上書き", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		// 環境変数で設定を上書き
		t.Setenv("DEVSYNC_CONTROL_CONCURRENCY", "32")
		t.Setenv("DEVSYNC_CONTROL_DRY_RUN", "true")

		// currentConfigをリセット
		currentConfig = nil

		cfg, err := Load()
		require.NoError(t, err)

		assert.Equal(t, 32, cfg.Control.Concurrency)
		assert.True(t, cfg.Control.DryRun)
	})
}

func TestGet(t *testing.T) {
	t.Run("Loadが呼ばれていない場合は自動ロード", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		// currentConfigをリセット
		currentConfig = nil

		cfg := Get()
		require.NotNil(t, cfg)
		// デフォルト値が設定されていることを確認
		assert.Equal(t, 1, cfg.Version)
	})

	t.Run("Loadが呼ばれた後はキャッシュされた設定を返す", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		// currentConfigをリセット
		currentConfig = nil

		cfg1, err := Load()
		require.NoError(t, err)

		cfg2 := Get()

		// 同じポインタを返すことを確認
		assert.Same(t, cfg1, cfg2)
	})
}
