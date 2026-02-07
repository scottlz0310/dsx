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
	Long: `DevSync は開発環境の運用作業を統合する CLI ツールです。

日次運用:
  devsync run           Bitwarden解錠→環境変数読込→システム更新を実行

システム更新:
  devsync sys update    パッケージマネージャで一括更新
  devsync sys list      利用可能なマネージャを一覧表示

環境変数:
  devsync env export    Bitwardenから環境変数をシェル形式で出力
  devsync env run       環境変数を注入してコマンドを実行

設定管理:
  devsync config init   対話形式で設定ファイルを生成
  devsync doctor        依存ツールと環境の診断

使用例:
  eval "$(devsync env export)"    # シェルに環境変数を読み込み
  devsync env run npm run build   # 環境変数を注入してビルド
  devsync sys update -n           # ドライラン（計画のみ表示）
  devsync sys update --jobs 4     # 4並列で更新`,
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
	// 設定ファイルが存在しない場合（初回実行時など）はエラーを無視して続行
	//nolint:errcheck // 初回実行時に設定ファイルがない場合のエラーは意図的に無視する
	config.Load()
}
