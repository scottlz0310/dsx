package config

const (
	// RepoGitHubProtocolHTTPS は GitHub リポジトリの HTTPS プロトコルです。
	RepoGitHubProtocolHTTPS = "https"
	// RepoCleanupTargetMerged は通常マージ済みブランチを表すクリーンアップターゲットです。
	RepoCleanupTargetMerged = "merged"
	// RepoCleanupTargetSquashed はスカッシュマージ済みブランチを表すクリーンアップターゲットです。
	RepoCleanupTargetSquashed = "squashed"
	secretsProviderBitwarden  = "bitwarden"
	fieldControlTimeout       = "control.timeout"
	fieldRepoRoot             = "repo.root"
	fieldSecretsProvider      = "secrets.provider"
	fieldSysEnable            = "sys.enable"
)
