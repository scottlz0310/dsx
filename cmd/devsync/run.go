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

// runCmd は日次処理を実行するコマンドの定義です
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "日次のシステム・リポジトリ更新などの統合タスクを実行します",
	Long: `設定ファイルに基づいて、システムの更新、リポジトリの同期、
環境変数の設定などを一括で行います。毎日の作業開始時に実行することを想定しています。

処理順序:
  1. Bitwarden のアンロック（secrets.enabled=true かつ未アンロック時のみ）
  2. 環境変数の読み込み（secrets.enabled=true かつ未読み込み時のみ）
  3. システム更新
  4. リポジトリ同期`,
	RunE: runDaily,
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func runDaily(cmd *cobra.Command, args []string) error {
	fmt.Println("🚀 開発環境の同期を開始します...")
	fmt.Println()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  設定ファイルの読み込みに失敗（デフォルト設定を使用）: %v\n", err)

		cfg = config.Default()
	}

	// 1 & 2. Bitwarden のアンロック + 環境変数読み込み
	runSecretsPhase(cfg)

	// 3. システム更新
	fmt.Println("🛠  システムを更新中...")

	if err := runSysUpdateStep(cmd, nil); err != nil {
		return fmt.Errorf("システム更新に失敗しました: %w", err)
	}

	fmt.Println()

	// 4. リポジトリ同期
	fmt.Println("📦 リポジトリを同期中...")

	if err := runRepoUpdateStep(cmd, nil); err != nil {
		return fmt.Errorf("リポジトリ同期に失敗しました: %w", err)
	}

	fmt.Println()

	fmt.Println("✅ 開発環境は最新の状態です。")

	return nil
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
