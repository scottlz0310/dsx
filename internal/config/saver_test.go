package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSave(t *testing.T) {
	t.Run("正常系: デフォルトパスに保存", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpDir)

		cfg := &Config{
			Version: 1,
			Control: ControlConfig{
				Concurrency: 8,
				Timeout:     "10m",
				DryRun:      false,
			},
		}

		err := Save(cfg, "")
		require.NoError(t, err)

		// ファイルが作成されたことを確認
		expectedPath := filepath.Join(tmpDir, ".config", "devsync", "config.yaml")
		_, err = os.Stat(expectedPath)
		assert.NoError(t, err)

		// 内容を検証
		data, err := os.ReadFile(expectedPath)
		require.NoError(t, err)

		var loaded Config
		err = yaml.Unmarshal(data, &loaded)
		require.NoError(t, err)

		assert.Equal(t, cfg.Version, loaded.Version)
		assert.Equal(t, cfg.Control.Concurrency, loaded.Control.Concurrency)
	})

	t.Run("正常系: 指定パスに保存", func(t *testing.T) {
		tmpDir := t.TempDir()
		customPath := filepath.Join(tmpDir, "custom", "config.yaml")

		cfg := &Config{
			Version: 2,
			Control: ControlConfig{
				Concurrency: 16,
				DryRun:      true,
			},
			Repo: RepoConfig{
				Root: "/custom/root",
				GitHub: GitHubConfig{
					Owner:    "testowner",
					Protocol: "ssh",
				},
			},
		}

		err := Save(cfg, customPath)
		require.NoError(t, err)

		// ファイルが作成されたことを確認
		_, err = os.Stat(customPath)
		assert.NoError(t, err)

		// 内容を検証
		data, err := os.ReadFile(customPath)
		require.NoError(t, err)

		var loaded Config
		err = yaml.Unmarshal(data, &loaded)
		require.NoError(t, err)

		assert.Equal(t, 2, loaded.Version)
		assert.Equal(t, 16, loaded.Control.Concurrency)
		assert.True(t, loaded.Control.DryRun)
		assert.Equal(t, "/custom/root", loaded.Repo.Root)
		assert.Equal(t, "ssh", loaded.Repo.GitHub.Protocol)
	})

	t.Run("正常系: ディレクトリが存在しない場合は作成される", func(t *testing.T) {
		tmpDir := t.TempDir()
		deepPath := filepath.Join(tmpDir, "deep", "nested", "dir", "config.yaml")

		cfg := Default()

		err := Save(cfg, deepPath)
		require.NoError(t, err)

		// ファイルが作成されたことを確認
		_, err = os.Stat(deepPath)
		assert.NoError(t, err)
	})

	t.Run("エラー系: 書き込み不可能なディレクトリ", func(t *testing.T) {
		// /proc は書き込み不可能なため、エラーが期待される
		cfg := Default()

		err := Save(cfg, "/proc/invalid/config.yaml")
		assert.Error(t, err)
	})

	t.Run("正常系: 複雑な設定の保存と読み込み", func(t *testing.T) {
		tmpDir := t.TempDir()
		customPath := filepath.Join(tmpDir, "config.yaml")

		cfg := &Config{
			Version: 1,
			Control: ControlConfig{
				Concurrency: 4,
				Timeout:     "5m",
				DryRun:      true,
			},
			Repo: RepoConfig{
				Root: "/home/user/repos",
				GitHub: GitHubConfig{
					Owner:    "myorg",
					Protocol: "ssh",
				},
				Sync: RepoSyncConfig{
					AutoStash: true,
					Prune:     false,
				},
				Cleanup: RepoCleanupConfig{
					Enabled:         true,
					Target:          []string{"merged", "squashed"},
					ExcludeBranches: []string{"main", "develop", "staging"},
				},
			},
			Sys: SysConfig{
				Enable: []string{"apt", "brew", "go"},
				Managers: map[string]ManagerConfig{
					"apt": {
						"use_sudo": true,
					},
					"brew": {
						"cleanup": true,
						"greedy":  false,
					},
				},
			},
			Secrets: SecretsConfig{
				Enabled:  true,
				Provider: "bitwarden",
			},
		}

		err := Save(cfg, customPath)
		require.NoError(t, err)

		// 読み込んで検証
		data, err := os.ReadFile(customPath)
		require.NoError(t, err)

		var loaded Config
		err = yaml.Unmarshal(data, &loaded)
		require.NoError(t, err)

		// 全ての値が正しく保存されていることを確認
		assert.Equal(t, cfg.Version, loaded.Version)
		assert.Equal(t, cfg.Control.Concurrency, loaded.Control.Concurrency)
		assert.Equal(t, cfg.Control.Timeout, loaded.Control.Timeout)
		assert.Equal(t, cfg.Control.DryRun, loaded.Control.DryRun)
		assert.Equal(t, cfg.Repo.Root, loaded.Repo.Root)
		assert.Equal(t, cfg.Repo.GitHub.Owner, loaded.Repo.GitHub.Owner)
		assert.Equal(t, cfg.Repo.GitHub.Protocol, loaded.Repo.GitHub.Protocol)
		assert.Equal(t, cfg.Repo.Cleanup.ExcludeBranches, loaded.Repo.Cleanup.ExcludeBranches)
		assert.Equal(t, cfg.Sys.Enable, loaded.Sys.Enable)
		assert.Equal(t, cfg.Secrets.Enabled, loaded.Secrets.Enabled)
	})
}
