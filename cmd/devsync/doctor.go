package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// doctorCmd は doctor コマンドの定義です
var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "依存ツールと環境の診断を行います",
	Long: `開発に必要なツール (git, git-lfs, bw など) がインストールされているか確認し、
設定ファイルの状態を診断します。`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("🏥 DevSync Doctor: 環境診断を開始します...")
		// TODO: 実際の実装はここに行います
		fmt.Println("現在はプロトタイプ実装のため、チェック項目はありません。")
		fmt.Println("✅ 診断完了 (ダミー)")
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
