package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "devsync",
	Short: "DevSync: 開発環境運用ツール",
	Long: `DevSync は開発環境の運用作業（リポジトリ管理、システム更新など）を統合する CLI ツールです。
日本語環境での利用を前提に設計されています。`,
	// Runを指定しない場合、サブコマンドがない時はヘルプが表示されます
}

// Execute はコマンド実行のエントリーポイントです
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// 将来的にグローバルなフラグ（--configなど）はここで定義します
}
