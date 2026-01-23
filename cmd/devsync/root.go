package main

import (
	"fmt"
	"os"

	"github.com/scottlz0310/devsync/internal/config"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "devsync",
	Short: "DevSync: 開発環境運用ツール",
	Long: `DevSync は開発環境の運用作業（リポジトリ管理、システム更新など）を統合する CLI ツールです。
日本語環境での利用を前提に設計されています。`,
}

// Execute はコマンド実行のエントリーポイントです
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
}

func initConfig() {
	if _, err := config.Load(); err != nil {
		// 設定ファイルが存在しない場合などはエラーを無視
		// (初回実行時など)
	}
}
