package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
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
	Long: `設定ファイルに基づいて、システムの更新とリポジトリの同期を一括で行います。
毎日の作業開始時に実行することを想定しています。

処理順序:
  1. システム更新
  2. リポジトリ同期

Bitwarden のアンロックと環境変数の読み込みは行いません（日常の更新にシークレットは不要です）。
シークレットが必要なときは dsx env を使います（例: dsx env run -- dsx run）。

フラグ（--dry-run, --tui/--no-tui, --jobs）は sys update / repo update に伝播されます。
--root は repo update のみに伝播され、指定時は設定ファイルの repo.root を上書きします。`,
	RunE: runDaily,
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().BoolVarP(&runDryRun, "dry-run", "n", false, "実際の更新は行わず、計画のみ表示（sys/repo に伝播）")
	runCmd.Flags().IntVarP(&runJobs, "jobs", "j", 0, "並列実行数（sys/repo に伝播、0 は設定値を使用）")
	runCmd.Flags().BoolVar(&runTUI, "tui", false, "Bubble Tea の進捗UIを表示（sys/repo に伝播）")
	runCmd.Flags().BoolVar(&runNoTUI, "no-tui", false, "TUI 進捗表示を無効化（sys/repo に伝播）")
	runCmd.Flags().StringVar(&runLogFile, "log-file", "", "ジョブ実行ログをファイルに保存（sys/repo に伝播）")
	// runRepoUpdate は渡された cmd の Changed("root") と repoRootOverride で root を解決するため、
	// repo update と同じ変数に束ねるだけで run からも伝播する。
	runCmd.Flags().StringVar(&repoRootOverride, "root", "", "リポジトリ更新対象のルートディレクトリ（指定時は設定を上書き、repo update のみに伝播）")
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
	defer printSelfUpdateNoticeAtEnd()

	fmt.Println("🚀 開発環境の同期を開始します...")
	fmt.Println()

	// run のフラグを子コマンドに伝播
	propagateRunFlags(cmd)

	// --tui と --no-tui の矛盾チェック（フェーズ実行前に検出）
	if cmd.Flags().Changed("tui") && runTUI && cmd.Flags().Changed("no-tui") && runNoTUI {
		return fmt.Errorf("--tui と --no-tui は同時指定できません")
	}

	var phaseErrors []phaseError

	// 1. システム更新
	fmt.Println("🛠  システムを更新中...")

	if err := runSysUpdateStep(cmd, nil); err != nil {
		phaseErrors = append(phaseErrors, phaseError{Name: "システム更新", Err: err})
		fmt.Fprintf(os.Stderr, "⚠️  システム更新でエラーが発生しましたが、続行します: %v\n", err)
	}

	fmt.Println()

	// 2. リポジトリ同期
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
