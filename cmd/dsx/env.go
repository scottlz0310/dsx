package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/scottlz0310/dsx/internal/secret"
	"github.com/spf13/cobra"
)

const envActionUnlock = "unlock"

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
  bash/zsh:    eval "$(dsx env export)"
  PowerShell:  dsx env export | Invoke-Expression

注意:
- 環境変数名は大文字とアンダースコアのみ（例: MY_VAR, API_KEY）
- 改行を含む値はサポートされません
- eval前提の出力のため、安全なクオート/エスケープを保証します`,
	RunE: runEnvExport,
}

var envUnlockCmd = &cobra.Command{
	Use:   envActionUnlock,
	Short: "Bitwarden をアンロックして BW_SESSION を設定",
	Long: `Bitwarden をアンロックし、BW_SESSION 設定コマンドを標準出力に出力します。
出力をシェルで評価することで、BW_SESSION が現在のシェルセッションに反映されます。

使用方法:
  bash/zsh:    eval "$(dsx env unlock)"
  PowerShell:  dsx env unlock | Invoke-Expression

--sync フラグを指定すると、アンロック後にサーバーと強制同期します。
トークンロール（シークレット更新）直後はこのフラグを使用してください。`,
	RunE: runEnvUnlock,
}

var envStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Bitwarden セッション状態を確認",
	Long: `Bitwarden CLI の現在のセッション状態を確認します。

--quiet を指定すると、アンロック済みの場合だけ終了コード 0 を返し、
それ以外は非ゼロで終了します。シェル連携スクリプトからの判定に使用します。`,
	RunE: runEnvStatus,
}

var envRunCmd = &cobra.Command{
	Use:   "run [--sync] [--detach] [--] <command> [args...]",
	Short: "環境変数を注入してコマンドを実行",
	Long: `Bitwardenから環境変数を取得し、それを注入した状態でコマンドを実行します。
eval を使わずに環境変数を利用する安全な方法です。

フラグ:
  --sync    Bitwarden サーバーと強制同期してから実行（トークンロール後など）
  --detach  プロセスをデタッチ起動する（GUIアプリ用。完了を待たない）

使用例:
  dsx env run npm run build
  dsx env run --sync go test ./...
  dsx env run --detach -- "C:\Program Files\Claude\Claude.exe"
  dsx env run --sync --detach -- "C:\Program Files\Claude\Claude.exe"

  ショートカットのターゲット例（Claude Desktop用）:
    pwsh.exe -WindowStyle Hidden -Command "dsx env run --detach -- 'C:\...\Claude.exe'"`,
	RunE:               runEnvRun,
	DisableFlagParsing: true, // サブコマンドのフラグと混在しないよう手動パース
}

// envUnlockSync は --sync フラグの値を保持します。
var envUnlockSync bool
var envStatusQuiet bool

var (
	getEnvVarsFunc                = secret.GetEnvVars
	getBitwardenSessionStatusFunc = secret.GetBitwardenSessionStatus
	exportFormatFunc              = secret.ExportFormat
	formatForShellFunc            = secret.FormatForShell
	detectShellFunc               = secret.DetectShell
)

func init() {
	rootCmd.AddCommand(envCmd)
	envCmd.AddCommand(envExportCmd)
	envCmd.AddCommand(envUnlockCmd)
	envCmd.AddCommand(envStatusCmd)
	envCmd.AddCommand(envRunCmd)

	envUnlockCmd.Flags().BoolVar(&envUnlockSync, "sync", false, "アンロック後に Bitwarden サーバーと強制同期する")
	envStatusCmd.Flags().BoolVar(&envStatusQuiet, "quiet", false, "アンロック済みかを終了コードのみで返す")
}

func runEnvExport(cmd *cobra.Command, args []string) error {
	// GetEnvVars が内部で EnsureBitwardenSession を呼び出し、
	// 必要に応じて対話的アンロックを行い BW_SESSION を更新する
	envVars, err := getEnvVarsFunc()
	if err != nil {
		return fmt.Errorf("環境変数の取得に失敗しました: %w", err)
	}

	// EnsureBitwardenSession による BW_SESSION 更新後の最新値を取得
	sessionToken := os.Getenv("BW_SESSION")

	// シェル用の形式でフォーマット
	output, err := exportFormatFunc(envVars)
	if err != nil {
		return fmt.Errorf("エクスポート形式の生成に失敗しました: %w", err)
	}

	sessionOutput, formatErr := formatForShellFunc(
		map[string]string{"BW_SESSION": sessionToken},
		detectShellFunc(),
	)
	if formatErr != nil {
		return fmt.Errorf("BW_SESSION のエクスポート形式の生成に失敗しました: %w", formatErr)
	}

	output = sessionOutput + "\n" + output

	// 標準出力に出力（evalで使用される）
	fmt.Println(output)

	// 統計情報を stderr に出力（stdout は eval/Invoke-Expression 用なので汚さない）
	fmt.Fprintf(os.Stderr, "✅ %d 個の環境変数を読み込みました。\n", len(envVars))

	return nil
}

func runEnvStatus(cmd *cobra.Command, args []string) error {
	status, err := getBitwardenSessionStatusFunc()
	if err != nil {
		if envStatusQuiet {
			return err
		}

		return fmt.Errorf("bitwarden セッション状態の確認に失敗しました: %w", err)
	}

	if envStatusQuiet {
		if status == "unlocked" {
			return nil
		}

		return fmt.Errorf("bitwarden はアンロックされていません")
	}

	switch status {
	case "unlocked":
		fmt.Println("Bitwarden はアンロック済みです。")
	case "missing":
		fmt.Println("BW_SESSION が設定されていません。")
	case "locked":
		fmt.Println("Bitwarden はロックされています。")
	default:
		fmt.Printf("Bitwarden の状態: %s\n", status)
	}

	return nil
}

func runEnvUnlock(cmd *cobra.Command, args []string) error {
	// アンロックしてトークンを取得
	token, err := secret.UnlockGetToken()
	if err != nil {
		return err
	}

	// BW_SESSION をシェル設定コマンドとして stdout に出力
	// eval "$(dsx env unlock)" / dsx env unlock | Invoke-Expression で親シェルに反映される
	output, err := secret.FormatForShell(
		map[string]string{"BW_SESSION": token},
		secret.DetectShell(),
	)
	if err != nil {
		return fmt.Errorf("シェルコマンドの生成に失敗しました: %w", err)
	}

	fmt.Println(output)

	// --sync 指定時はアンロック後にサーバーと同期
	if envUnlockSync {
		// 子プロセス（bw sync）に BW_SESSION を引き継ぐため現プロセスにも設定
		if setErr := os.Setenv("BW_SESSION", token); setErr != nil {
			fmt.Fprintf(os.Stderr, "⚠️  BW_SESSION の設定に失敗しました: %v\n", setErr)
		}

		if err := secret.Sync(); err != nil {
			// 同期失敗はエラーとして返す（unlock 自体は成功している）
			return fmt.Errorf("同期に失敗しました: %w", err)
		}
	}

	return nil
}

func runEnvRun(cmd *cobra.Command, args []string) error {
	// DisableFlagParsing: true のため --sync / --detach を手動パース
	// これにより実行コマンド側のフラグ（例: npm --save）と競合しない
	withSync := false
	detach := false
	cmdArgs := make([]string, 0, len(args))
	passThroughStarted := false

	for _, arg := range args {
		if passThroughStarted {
			cmdArgs = append(cmdArgs, arg)
			continue
		}

		switch arg {
		case "--sync":
			withSync = true
		case "--detach":
			detach = true
		case "--":
			// 以降はすべてコマンド引数（フラグ解釈しない）
			passThroughStarted = true
		default:
			// 最初の非フラグ引数以降はすべてコマンドとして扱う
			cmdArgs = append(cmdArgs, arg)
			passThroughStarted = true
		}
	}

	if len(cmdArgs) == 0 {
		return fmt.Errorf("実行するコマンドを指定してください")
	}

	// Bitwarden から環境変数を取得（--sync 指定時は強制同期）
	getFunc := secret.GetEnvVars
	if withSync {
		getFunc = secret.GetEnvVarsWithSync
	}

	envVars, err := getFunc()
	if err != nil {
		return fmt.Errorf("環境変数の取得に失敗しました: %w", err)
	}

	// デタッチ起動（GUIアプリ用）
	if detach {
		return secret.RunWithEnvDetach(cmdArgs, envVars)
	}

	// 通常実行（プロセス完了まで待機）
	if err := secret.RunWithEnv(cmdArgs, envVars); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}

		return err
	}

	return nil
}
