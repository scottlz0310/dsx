package main

import (
	"fmt"
	"os"

	"github.com/scottlz0310/devsync/internal/config"
	"github.com/scottlz0310/devsync/internal/secret"
	"github.com/spf13/cobra"
)

var (
	runUnlockStep     = secret.Unlock
	runLoadEnvStep    = secret.LoadEnv
	runSysUpdateStep  = runSysUpdate
	runRepoUpdateStep = runRepoUpdate
)

var (
	runDryRun  bool
	runJobs    int
	runTUI     bool
	runNoTUI   bool
	runLogFile string
)

// runCmd は日次処理を実行するコマンドの定義です
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "日次のシステム・リポジトリ更新などの統合タスクを実行します",
	Long: `設定ファイルに基づいて、システムの更新、リポジトリの同期、
環境変数の設定などを一括で行います。毎日の作業開始時に実行することを想定しています。

処理順序:
  1. Bitwarden のアンロック（secrets.enabled=true かつ未アンロック時のみ）
  2. Bitwarden データの同期（bw sync でキャッシュを最新化）
  3. 環境変数の読み込み（secrets.enabled=true かつ未読み込み時のみ）
  4. システム更新
  5. リポジトリ同期

フラグ（--dry-run, --tui/--no-tui, --jobs）は sys update / repo update に伝播されます。`,
	RunE: runDaily,
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().BoolVarP(&runDryRun, "dry-run", "n", false, "実際の更新は行わず、計画のみ表示（sys/repo に伝播）")
	runCmd.Flags().IntVarP(&runJobs, "jobs", "j", 0, "並列実行数（sys/repo に伝播、0 は設定値を使用）")
	runCmd.Flags().BoolVar(&runTUI, "tui", false, "Bubble Tea の進捗UIを表示（sys/repo に伝播）")
	runCmd.Flags().BoolVar(&runNoTUI, "no-tui", false, "TUI 進捗表示を無効化（sys/repo に伝播）")
	runCmd.Flags().StringVar(&runLogFile, "log-file", "", "ジョブ実行ログをファイルに保存（sys/repo に伝播）")
}

// propagateRunFlags は run コマンドのフラグを sys/repo のグローバルフラグ変数に伝播します。
func propagateRunFlags(cmd *cobra.Command) {
	if cmd.Flags().Changed("dry-run") {
		sysDryRun = runDryRun
		repoUpdateDryRun = runDryRun
	}

	if cmd.Flags().Changed("jobs") {
		sysJobs = runJobs
		repoUpdateJobs = runJobs
	}

	if cmd.Flags().Changed("tui") {
		sysTUI = runTUI
		repoUpdateTUI = runTUI
	}

	if cmd.Flags().Changed("no-tui") {
		sysNoTUI = runNoTUI
		repoUpdateNoTUI = runNoTUI
	}

	if cmd.Flags().Changed("log-file") {
		sysLogFile = runLogFile
		repoUpdateLogFile = runLogFile
	}
}

func runDaily(cmd *cobra.Command, args []string) error {
	fmt.Println("🚀 開発環境の同期を開始します...")
	fmt.Println()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  設定ファイルの読み込みに失敗（デフォルト設定を使用）: %v\n", err)

		cfg = config.Default()
	}

	// run のフラグを子コマンドに伝播
	propagateRunFlags(cmd)

	// --tui と --no-tui の矛盾チェック（フェーズ実行前に検出）
	if cmd.Flags().Changed("tui") && runTUI && cmd.Flags().Changed("no-tui") && runNoTUI {
		return fmt.Errorf("--tui と --no-tui は同時指定できません")
	}

	// 1 & 2. Bitwarden のアンロック + 環境変数読み込み
	runSecretsPhase(cfg)

	var phaseErrors []phaseError

	// 3. システム更新
	fmt.Println("🛠  システムを更新中...")

	if err := runSysUpdateStep(cmd, nil); err != nil {
		phaseErrors = append(phaseErrors, phaseError{Name: "システム更新", Err: err})
		fmt.Fprintf(os.Stderr, "⚠️  システム更新でエラーが発生しましたが、続行します: %v\n", err)
	}

	fmt.Println()

	// 4. リポジトリ同期
	fmt.Println("📦 リポジトリを同期中...")

	if err := runRepoUpdateStep(cmd, nil); err != nil {
		phaseErrors = append(phaseErrors, phaseError{Name: "リポジトリ同期", Err: err})
	}

	fmt.Println()

	// 統合サマリー
	if len(phaseErrors) > 0 {
		printPhaseErrors(phaseErrors)

		return fmt.Errorf("%d 件のフェーズでエラーが発生しました", len(phaseErrors))
	}

	fmt.Println("✅ 開発環境は最新の状態です。")

	return nil
}

// phaseError は run コマンド内の各フェーズで発生したエラーを保持します。
type phaseError struct {
	Name string
	Err  error
}

// printPhaseErrors は各フェーズのエラーをまとめて表示します。
func printPhaseErrors(phaseErrors []phaseError) {
	if len(phaseErrors) == 0 {
		return
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "❌ 以下のフェーズでエラーが発生しました:")

	for i, pe := range phaseErrors {
		prefix := "  ├──"
		if i == len(phaseErrors)-1 {
			prefix = "  └──"
		}

		fmt.Fprintf(os.Stderr, "%s %s: %v\n", prefix, pe.Name, pe.Err)
	}
}

// runSecretsPhase は secrets 設定に応じて Bitwarden のアンロックと環境変数読み込みを実行します。
// dev-sync シェル関数経由で既にアンロック済みの場合、重複する bw 呼び出しをスキップします。
func runSecretsPhase(cfg *config.Config) {
	if !cfg.Secrets.Enabled {
		fmt.Println("ℹ️  シークレット管理は無効です（secrets.enabled=false）")
		fmt.Println()

		return
	}

	// シェル関数側（devsync-unlock）で BW_SESSION が既に設定済みの場合、
	// Unlock 内部で「既にアンロック済み」と判定して bw unlock をスキップする。
	fmt.Println("🔐 シークレットをアンロック中...")

	if err := runUnlockStep(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Bitwarden のアンロックに失敗: %v\n", err)
		fmt.Fprintf(os.Stderr, "⚠️  シークレット読み込みをスキップして続行します\n")
		fmt.Println()

		return
	}

	fmt.Println()

	// シェル関数側（devsync-load-env）で環境変数が既に設定済みかを判定し、
	// 設定済みなら bw list items の再実行をスキップする。
	if isEnvAlreadyLoaded() {
		fmt.Println("ℹ️  環境変数はシェル側で読み込み済みです（bw 再取得をスキップ）")
		fmt.Println()

		return
	}

	// 環境変数の読み込みは失敗しても続行する（非致命的エラー）
	stats, err := runLoadEnvStep()
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  環境変数の読み込みに失敗: %v\n", err)
	}

	if stats != nil && stats.Loaded > 0 {
		if gpat := os.Getenv("GPAT"); gpat != "" {
			fmt.Println("✅ GPAT が読み込まれました。リポジトリ設定の自動化が利用可能です。")
		}
	}

	fmt.Println()
}

// isEnvAlreadyLoaded はシェル関数（devsync-load-env）により設定されるマーカー環境変数
// DEVSYNC_ENV_LOADED が "1" の場合、bw list items の再実行をスキップします。
func isEnvAlreadyLoaded() bool {
	return os.Getenv("DEVSYNC_ENV_LOADED") == "1"
}
