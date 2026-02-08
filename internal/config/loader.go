package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

var (
	// currentConfig は現在ロードされている設定を保持します。
	currentConfig *Config
)

// Load は設定ファイルを読み込み、設定構造体を返します。
// 設定ファイルが見つからない場合はデフォルト値を返します。
func Load() (*Config, error) {
	v := viper.New()

	// デフォルト値の設定
	setDefaults(v)

	// 環境変数の設定
	// 環境変数は DEVSYNC_ で始まり、ドットはアンダースコアに置換される
	// 例: DEVSYNC_CONTROL_DRY_RUN -> control.dry_run
	v.SetEnvPrefix("devsync")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 設定ファイルの探索パス
	home, err := os.UserHomeDir()
	if err == nil {
		configDir := filepath.Join(home, ".config", "devsync")
		v.AddConfigPath(configDir)
		v.SetConfigName("config")
		v.SetConfigType("yaml")
	}

	// 設定ファイルの読み込み（ファイルがない場合はデフォルト値のみで続行）
	if err := v.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			// 設定ファイルが存在するが読み込めない場合はエラー
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	currentConfig = &cfg

	return currentConfig, nil
}

// Get は現在ロードされている設定を返します。
// Load が呼ばれていない場合は nil を返す可能性があります。
func Get() *Config {
	if currentConfig == nil {
		// 未ロードの場合はデフォルトロードを試みる
		// エラーは意図的に無視（設定ファイルがなくても動作させるため）
		//nolint:errcheck // 設定ファイル未存在は許容
		Load()
	}

	return currentConfig
}

// Default はデフォルト設定を返します。
// 設定ファイルが存在しない場合などに使用します。
func Default() *Config {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	defaultRoot := filepath.Join(home, "src")
	if home == "" {
		defaultRoot = "./src"
	}

	return &Config{
		Version: 1,
		Control: ControlConfig{
			Concurrency: 8,
			Timeout:     "10m",
			DryRun:      false,
		},
		Repo: RepoConfig{
			Root: defaultRoot,
			GitHub: GitHubConfig{
				Protocol: "https",
			},
			Sync: RepoSyncConfig{
				AutoStash:       true,
				Prune:           true,
				SubmoduleUpdate: true,
			},
			Cleanup: RepoCleanupConfig{
				Enabled:         true,
				Target:          []string{"merged", "squashed"},
				ExcludeBranches: []string{"main", "master", "develop"},
			},
		},
		Sys: SysConfig{
			Enable:   []string{},
			Managers: map[string]ManagerConfig{},
		},
		Secrets: SecretsConfig{
			Enabled:  false,
			Provider: "bitwarden",
		},
	}
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("version", 1)

	// Control
	v.SetDefault("control.concurrency", 8)
	v.SetDefault("control.timeout", "10m")
	v.SetDefault("control.dry_run", false)

	// Repo
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	defaultRoot := filepath.Join(home, "src")
	if home == "" {
		defaultRoot = "./src"
	}

	v.SetDefault("repo.root", defaultRoot)
	v.SetDefault("repo.github.owner", "")
	v.SetDefault("repo.github.protocol", "https")
	v.SetDefault("repo.sync.auto_stash", true)
	v.SetDefault("repo.sync.prune", true)
	v.SetDefault("repo.sync.submodule_update", true)
	v.SetDefault("repo.cleanup.enabled", true)
	v.SetDefault("repo.cleanup.target", []string{"merged", "squashed"})
	v.SetDefault("repo.cleanup.exclude_branches", []string{"main", "master", "develop"})

	// Sys defaults (managers are enabled per environment usually, but defaults can be empty)
	v.SetDefault("sys.enable", []string{})
	v.SetDefault("sys.managers", map[string]interface{}{})

	// Secrets
	v.SetDefault("secrets.enabled", false)
	v.SetDefault("secrets.provider", "bitwarden")
	v.SetDefault("secrets.items", []string{})
}

// ConfigPath はデフォルトの設定ファイルパスを返します。
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("ホームディレクトリの取得に失敗: %w", err)
	}

	return filepath.Join(home, ".config", "devsync", "config.yaml"), nil
}

// ConfigFileExists は設定ファイルの存在有無を返します。
// path には判定対象の設定ファイルパスが入ります。
func ConfigFileExists() (exists bool, path string, err error) {
	path, err = ConfigPath()
	if err != nil {
		return false, "", err
	}

	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return false, path, fmt.Errorf("設定ファイルのパスがディレクトリです: %s", path)
		}

		return true, path, nil
	}

	if errors.Is(err, os.ErrNotExist) {
		return false, path, nil
	}

	return false, path, fmt.Errorf("設定ファイルの状態確認に失敗: %w", err)
}
