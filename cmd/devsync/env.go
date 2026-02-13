package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/scottlz0310/devsync/internal/secret"
	"github.com/spf13/cobra"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "環境変数の管理",
	Long:  `Bitwardenから環境変数を取得・エクスポートします。`,
}

var envExportCmd = &cobra.Command{
	Use:   "export",
	Short: "環境変数をシェル用の形式でエクスポート",
	Long: `Bitwardenから環境変数を取得し、現在のシェルにエクスポートできる形式で出力します。

使用方法:
  bash/zsh:    eval "$(devsync env export)"
  PowerShell:  & devsync env export | Invoke-Expression

注意:
- 環境変数名は大文字とアンダースコアのみ（例: MY_VAR, API_KEY）
- 改行を含む値はサポートされません
- eval前提の出力のため、安全なクオート/エスケープを保証します`,
	RunE: runEnvExport,
}

var envRunCmd = &cobra.Command{
	Use:   "run [command...]",
	Short: "環境変数を注入してコマンドを実行",
	Long: `Bitwardenから環境変数を取得し、それを注入した状態でコマンドを実行します。

これは eval を使わずに環境変数を利用する安全な方法です。

使用例:
  devsync env run npm run build
  devsync env run go test ./...`,
	RunE:               runEnvRun,
	DisableFlagParsing: true, // コマンド引数をそのまま渡す
}

func init() {
	rootCmd.AddCommand(envCmd)
	envCmd.AddCommand(envExportCmd)
	envCmd.AddCommand(envRunCmd)
}

func runEnvExport(cmd *cobra.Command, args []string) error {
	// Bitwardenから環境変数を取得
	envVars, err := secret.GetEnvVars()
	if err != nil {
		return fmt.Errorf("環境変数の取得に失敗しました: %w", err)
	}

	// シェル用の形式でフォーマット
	output, err := secret.ExportFormat(envVars)
	if err != nil {
		return fmt.Errorf("エクスポート形式の生成に失敗しました: %w", err)
	}

	// 標準出力に出力（evalで使用される）
	fmt.Println(output)

	// 統計情報を stderr に出力（stdout は eval/Invoke-Expression 用なので汚さない）
	fmt.Fprintf(os.Stderr, "✅ %d 個の環境変数を読み込みました。\n", len(envVars))

	return nil
}

func runEnvRun(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("実行するコマンドを指定してください")
	}

	// Bitwardenから環境変数を取得
	envVars, err := secret.GetEnvVars()
	if err != nil {
		return fmt.Errorf("環境変数の取得に失敗しました: %w", err)
	}

	// 環境変数を注入してコマンドを実行
	if err := secret.RunWithEnv(args, envVars); err != nil {
		// コマンドの終了コードを取得して終了
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}

		return err
	}

	return nil
}
