package config

const (
	// RepoGitHubProtocolHTTPS は GitHub リポジトリの HTTPS プロトコルです。
	RepoGitHubProtocolHTTPS = "https"
	// RepoCleanupTargetMerged は通常マージ済みブランチを表すクリーンアップターゲットです。
	RepoCleanupTargetMerged = "merged"
	// RepoCleanupTargetSquashed はスカッシュマージ済みブランチを表すクリーンアップターゲットです。
	RepoCleanupTargetSquashed = "squashed"
	// DiagnosticsCaptureAnomaly は判定不能の疑いまたはコマンド失敗のときだけ診断ログを保存する指定です。
	DiagnosticsCaptureAnomaly = "anomaly"
	// DiagnosticsCaptureAll は解析対象コマンドの出力を毎回診断ログへ保存する指定です。
	DiagnosticsCaptureAll = "all"
	// DiagnosticsDefaultMaxFiles は診断ログファイルの既定の保持数です。
	DiagnosticsDefaultMaxFiles = 20
	// DiagnosticsDefaultMaxTotalMB は診断ログの既定の合計容量の上限（MiB）です。
	DiagnosticsDefaultMaxTotalMB = 20
	secretsProviderBitwarden     = "bitwarden"
	fieldControlTimeout          = "control.timeout"
	fieldRepoRoot                = "repo.root"
	fieldSecretsProvider         = "secrets.provider"
	fieldSysEnable               = "sys.enable"
)
