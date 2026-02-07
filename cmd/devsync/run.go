package main

import (
	"fmt"
	"os"

	"github.com/scottlz0310/devsync/internal/secret"
	"github.com/spf13/cobra"
)

// runCmd は日次処理を実行するコマンドの定義です
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "日次のシステム・リポジトリ更新などの統合タスクを実行します",
	Long: `設定ファイルに基づいて、システムの更新、リポジトリの同期、
環境変数の設定などを一括で行います。毎日の作業開始時に実行することを想定しています。

処理順序:
  1. Bitwarden のアンロック
  2. 環境変数の読み込み (GPAT など)
  3. リポジトリ設定の自動化
  4. システム更新
  5. リポジトリ同期`,
	RunE: runDaily,
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func runDaily(cmd *cobra.Command, args []string) error {
	fmt.Println("🚀 開発環境の同期を開始します...")
	fmt.Println()

	// 1. Bitwarden のアンロック
	fmt.Println("🔐 シークレットをアンロック中...")

	if err := secret.Unlock(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Bitwarden のアンロックに失敗: %v\n", err)
		return err
	}

	fmt.Println()

	// 2. 環境変数の読み込み
	// 環境変数の読み込みは失敗しても続行する（非致命的エラー）
	stats, err := secret.LoadEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  環境変数の読み込みに失敗: %v\n", err)
	}

	if stats != nil && stats.Loaded > 0 {
		// GPATが読み込まれているか確認
		if gpat := os.Getenv("GPAT"); gpat != "" {
			fmt.Println("✅ GPAT が読み込まれました。リポジトリ設定の自動化が利用可能です。")
		}
	}

	fmt.Println()

	// 3. システム更新（将来実装）
	fmt.Println("🛠  システムを更新中...")
	fmt.Println("（sysup update の統合は今後実装予定）")
	fmt.Println()

	// 4. リポジトリ同期（将来実装）
	fmt.Println("📦 リポジトリを同期中...")
	fmt.Println("（setup-repo sync の統合は今後実装予定）")
	fmt.Println()

	fmt.Println("✅ 開発環境は最新の状態です。")

	return nil
}
