package main

import (
	"fmt"
	"os"

	"github.com/scottlz0310/devsync/internal/config"
	"github.com/scottlz0310/devsync/internal/secret"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "devsync",
	Short: "DevSync: 開発環境運用ツール",
	Long: `DevSync は開発環境の運用作業（リポジトリ管理、システム更新など）を統合する CLI ツールです。
日本語環境での利用を前提に設計されています。`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// 設定のロード（initで呼ばれるが、ここで確実に取得する）
		cfg := config.Get()
		if cfg == nil {
			// ロードされていない場合は再試行
			var err error
			cfg, err = config.Load()
			if err != nil {
				// 設定ファイルがない場合でも、initコマンドなどは動くべきなので
				// ここではWarn程度にするか、コマンドによっては無視する
				// ただしBitwarden設定が読めないと困るので、Debugログを出したいところ
			}
		}

		// シークレット注入処理
		if cfg != nil && cfg.Secrets.Enabled {
			// Bitwardenの場合
			if cfg.Secrets.Provider == "bitwarden" {
				injector := secret.NewInjector(cfg.Secrets.Items)
				if err := injector.Inject(); err != nil {
					return fmt.Errorf("シークレットの注入に失敗しました: %w", err)
				}
			}
		}
		return nil
	},
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
