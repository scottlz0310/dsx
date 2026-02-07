package config

// Config はアプリケーション全体の設定を保持する構造体です。
type Config struct {
	Version int           `mapstructure:"version" yaml:"version"`
	Control ControlConfig `mapstructure:"control" yaml:"control"`
	Repo    RepoConfig    `mapstructure:"repo" yaml:"repo"`
	Sys     SysConfig     `mapstructure:"sys" yaml:"sys"`
	Secrets SecretsConfig `mapstructure:"secrets" yaml:"secrets"`
}

// SecretsConfig はシークレット管理に関する設定です。
// 環境変数は Bitwarden の "env:" プレフィックス付き項目から自動的に読み込まれます。
type SecretsConfig struct {
	Enabled  bool   `mapstructure:"enabled" yaml:"enabled"`
	Provider string `mapstructure:"provider" yaml:"provider"` // "bitwarden"
}

// ControlConfig は実行制御に関する設定です。
type ControlConfig struct {
	Concurrency int    `mapstructure:"concurrency" yaml:"concurrency"`
	Timeout     string `mapstructure:"timeout" yaml:"timeout"` // 例: "10m"
	DryRun      bool   `mapstructure:"dry_run" yaml:"dry_run"`
}

// RepoConfig はリポジトリ管理機能に関する設定です。
type RepoConfig struct {
	Root    string            `mapstructure:"root" yaml:"root"`
	GitHub  GitHubConfig      `mapstructure:"github" yaml:"github"`
	Sync    RepoSyncConfig    `mapstructure:"sync" yaml:"sync"`
	Cleanup RepoCleanupConfig `mapstructure:"cleanup" yaml:"cleanup"`
}

type GitHubConfig struct {
	Owner    string `mapstructure:"owner" yaml:"owner"`
	Protocol string `mapstructure:"protocol" yaml:"protocol"` // "https" or "ssh"
}

type RepoSyncConfig struct {
	AutoStash       bool `mapstructure:"auto_stash" yaml:"auto_stash"`
	Prune           bool `mapstructure:"prune" yaml:"prune"`
	SubmoduleUpdate bool `mapstructure:"submodule_update" yaml:"submodule_update"`
}

type RepoCleanupConfig struct {
	Enabled         bool     `mapstructure:"enabled" yaml:"enabled"`
	Target          []string `mapstructure:"target" yaml:"target"`                     // ["merged", "squashed"]
	ExcludeBranches []string `mapstructure:"exclude_branches" yaml:"exclude_branches"` // ["main", "master", "develop"]
}

// SysConfig はシステム更新機能に関する設定です。
type SysConfig struct {
	Enable   []string                 `mapstructure:"enable" yaml:"enable"`     // 有効化するマネージャ名のリスト
	Managers map[string]ManagerConfig `mapstructure:"managers" yaml:"managers"` // マネージャごとの個別設定
}

// ManagerConfig は各パッケージマネージャの汎用的な設定マップです。
// Go側で map[string]interface{} として受け取り、必要に応じてキャストします。
type ManagerConfig map[string]interface{}
