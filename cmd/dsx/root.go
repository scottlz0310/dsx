package main

import (
	"fmt"
	"os"

	"github.com/scottlz0310/dsx/internal/config"
	"github.com/spf13/cobra"
)

// version はビルド時に -ldflags で注入されます。
// 例: go build -ldflags "-X main.version=v1.0.0"
// GoReleaser がタグから自動設定します。
var version = "dev"

var rootCmd = &cobra.Command{
	Use:           "dsx",
	Short:         "dsx: 開発環境運用ツール",
	Version:       version,
	SilenceErrors: true,
	SilenceUsage:  true,
	Long: `dsx は開発環境の運用作業を統合する CLI ツールです。

日次運用:
  dsx run           Bitwarden解錠→環境変数読込→システム更新を実行

システム更新:
  dsx sys update    パッケージマネージャで一括更新
  dsx sys list      利用可能なマネージャを一覧表示

リポジトリ管理:
  dsx repo update   管理下のリポジトリを更新
  dsx repo list     管理下のリポジトリ一覧と状態を表示
  dsx repo cleanup  マージ済みローカルブランチを整理

環境変数:
  dsx env export    Bitwardenから環境変数をシェル形式で出力
  dsx env run       環境変数を注入してコマンドを実行

設定管理:
  dsx config init       対話形式で設定ファイルを生成
  dsx config show       設定を表示
  dsx config validate   設定を検証
  dsx config uninstall  シェル設定からdsxを削除
  dsx doctor            依存ツールと環境の診断
  dsx self-update       dsx 本体を更新

使用例:
  eval "$(dsx env export)"    # シェルに環境変数を読み込み
  dsx env run npm run build   # 環境変数を注入してビルド
  dsx sys update -n           # ドライラン（計画のみ表示）
  dsx sys update --jobs 4     # 4並列で更新
  dsx repo update --jobs 4    # リポジトリを4並列で更新
  dsx repo cleanup -n         # 削除対象ブランチの計画を表示（DryRun）
  dsx self-update --check     # 新バージョンの有無だけ確認`,
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
