package config

// Config はアプリケーション全体の設定を保持する構造体です。
type Config struct {
	Version int           `mapstructure:"version" yaml:"version"`
	Control ControlConfig `mapstructure:"control" yaml:"control"`
	UI      UIConfig      `mapstructure:"ui" yaml:"ui"`
	Repo    RepoConfig    `mapstructure:"repo" yaml:"repo"`
	Sys     SysConfig     `mapstructure:"sys" yaml:"sys"`
	Secrets SecretsConfig `mapstructure:"secrets" yaml:"secrets"`

	Diagnostics DiagnosticsConfig `mapstructure:"diagnostics" yaml:"diagnostics"`
}

// DiagnosticsConfig は診断ログ（解析対象コマンドの生出力と環境情報のローカル保存）の設定です。
// 既定は無効で、有効にしたときだけ、利用者のローカルのディレクトリへ保存します（外部へは送信しません）。
type DiagnosticsConfig struct {
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`
	// Capture は保存の対象です。"anomaly"（判定不能の疑いまたはコマンド失敗のときだけ）または "all"（毎回）。
	Capture string `mapstructure:"capture" yaml:"capture"`
	// Dir は保存先ディレクトリ（絶対パス）です。空なら OS ごとの状態ディレクトリを使います。
	Dir string `mapstructure:"dir" yaml:"dir"`
	// MaxFiles は保持するログファイルの最大数です。超えた分は古いものから削除します。
	MaxFiles int `mapstructure:"max_files" yaml:"max_files"`
	// MaxTotalMB はログファイルの合計容量の上限（MiB）です。超えた分は古いものから削除します。
	MaxTotalMB int `mapstructure:"max_total_mb" yaml:"max_total_mb"`
}

// UIConfig はUI表示に関する設定です。
type UIConfig struct {
	// TUI は Bubble Tea の進捗UIを既定で有効化します。
	// 実行環境が対話端末でない場合は自動的に通常表示へフォールバックします。
	TUI bool `mapstructure:"tui" yaml:"tui"`
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
