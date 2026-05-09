package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scottlz0310/dsx/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSave(t *testing.T) {
	t.Run("正常系: デフォルトパスに保存", func(t *testing.T) {
		tmpDir := t.TempDir()
		testutil.SetTestHome(t, tmpDir)

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
		expectedPath := filepath.Join(tmpDir, ".config", "dsx", "config.yaml")
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
		tmpDir := t.TempDir()
		blocked := filepath.Join(tmpDir, "blocked")
		require.NoError(t, os.WriteFile(blocked, []byte("not a dir"), 0o644))

		cfg := Default()

		err := Save(cfg, filepath.Join(blocked, "config.yaml"))
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

func TestSaveAtomic(t *testing.T) {
	t.Run("正常系: 既存ファイルなしで新規作成", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "config.yaml")

		cfg := Default()
		backupPath, err := SaveAtomic(cfg, path)
		require.NoError(t, err)

		assert.Empty(t, backupPath, "既存ファイルがない場合バックアップパスは空のはず")

		_, err = os.Stat(path)
		assert.NoError(t, err, "設定ファイルが作成されていない")
	})

	t.Run("正常系: 既存ファイルありでバックアップが作成される", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "config.yaml")

		// 初回保存
		cfg := Default()
		_, err := SaveAtomic(cfg, path)
		require.NoError(t, err)

		// 2回目保存 → バックアップが作成される
		backupPath, err := SaveAtomic(cfg, path)
		require.NoError(t, err)

		assert.NotEmpty(t, backupPath, "バックアップパスが返されていない")

		_, err = os.Stat(backupPath)
		assert.NoError(t, err, "バックアップファイルが存在しない")

		assert.Contains(t, backupPath, ".bak.", "バックアップパスに .bak. が含まれていない")

		// ナノ秒精度のタイムスタンプが含まれることを確認（衝突回避）
		assert.Contains(t, backupPath, ".", "バックアップパスにナノ秒区切りの . が含まれていない")
	})

	t.Run("正常系: 同一秒内の2回呼び出しでバックアップ名が衝突しない", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "config.yaml")

		cfg := Default()
		_, err := SaveAtomic(cfg, path)
		require.NoError(t, err)

		backup1, err := SaveAtomic(cfg, path)
		require.NoError(t, err)

		backup2, err := SaveAtomic(cfg, path)
		require.NoError(t, err)

		assert.NotEqual(t, backup1, backup2, "連続呼び出しでバックアップ名が衝突している")
	})

	t.Run("正常系: 保存後にデータが正しく読み込める", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "config.yaml")

		cfg := &Config{
			Version: 1,
			Sys: SysConfig{
				Enable: []string{"go"},
				Managers: map[string]ManagerConfig{
					"go": {"targets": []string{"golang.org/x/tools/gopls@latest"}},
				},
			},
		}

		_, err := SaveAtomic(cfg, path)
		require.NoError(t, err)

		data, err := os.ReadFile(path)
		require.NoError(t, err)

		var loaded Config
		require.NoError(t, yaml.Unmarshal(data, &loaded))

		assert.Equal(t, 1, loaded.Version)
		assert.Equal(t, []string{"go"}, loaded.Sys.Enable)
	})

	t.Run("正常系: .tmp ファイルが残っていない", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "config.yaml")

		cfg := Default()
		_, err := SaveAtomic(cfg, path)
		require.NoError(t, err)

		_, err = os.Stat(path + ".tmp")
		assert.True(t, os.IsNotExist(err), ".tmp ファイルが残っている")
	})
}
