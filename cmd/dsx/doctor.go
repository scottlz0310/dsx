package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/fatih/color"
	"github.com/scottlz0310/dsx/internal/config"
	"github.com/spf13/cobra"
)

// doctorCmd は doctor コマンドの定義です
var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "依存ツールと環境の診断を行います",
	Long: `開発に必要なツール (git, git-lfs, bw など) がインストールされているか確認し、
設定ファイルの状態を診断します。`,
	Run: func(cmd *cobra.Command, args []string) {
		runDoctor()
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor() {
	fmt.Println("🏥 dsx Doctor: 環境診断を開始します...")
	fmt.Println()

	allPassed := true

	// 1. 設定ファイルのチェック
	fmt.Println("📋 設定ファイル:")

	configExists, configPath, configStateErr := config.ConfigFileExists()
	if configStateErr != nil {
		printResult(false, fmt.Sprintf("設定ファイル状態の確認に失敗: %v", configStateErr))

		allPassed = false
	}

	cfg, loadErr := config.Load()
	switch {
	case loadErr != nil:
		printResult(false, fmt.Sprintf("設定ファイルの読み込みに失敗: %v", loadErr))

		allPassed = false
	case configStateErr == nil:
		printResult(true, buildDoctorConfigStatusMessage(configExists, configPath))
	default:
		printResult(true, "設定値の読み込みは成功しました（設定ファイル状態は要確認）")
	}

	if cfg == nil {
		fmt.Println("\n❌ 重大なエラー: 設定がロードできないため、以降のチェックを中断します")
		os.Exit(1)

		return
	}

	fmt.Println("\n🛠️  基本ツール:")
	// Git
	if err := checkCommand("git"); err != nil {
		printResult(false, "git が見つかりません")

		allPassed = false
	} else {
		printResult(true, "git")
	}

	// GitHub CLI
	// 必須ではないので見つからなくてもFail扱いにしない
	if err := checkCommand("gh"); err != nil {
		printResult(false, "gh (GitHub CLI) が見つかりません（推奨）")
	} else {
		printResult(true, "gh (GitHub CLI)")
	}

	fmt.Println("\n🔐 シークレット管理 (Bitwarden):")

	if cfg.Secrets.Enabled && cfg.Secrets.Provider == "bitwarden" {
		// bw コマンド
		if err := checkCommand("bw"); err != nil {
			printResult(false, "bw (Bitwarden CLI) が見つかりません")

			allPassed = false
		} else {
			printResult(true, "bw (Bitwarden CLI)")
		}

		// BW_SESSION
		if os.Getenv("BW_SESSION") == "" {
			printResult(false, "環境変数 BW_SESSION が設定されていません (ロック解除が必要です)")

			allPassed = false
		} else {
			printResult(true, "環境変数 BW_SESSION が設定されています")
		}
	} else {
		fmt.Println("   ⚪ スキップ (設定で無効化されています)")
	}

	fmt.Println()

	if allPassed {
		color.Green("✅ すべての診断項目をパスしました！準備完了です。")
	} else {
		color.Red("❌ 一部の項目で問題が見つかりました。ログを確認してください。")
		os.Exit(1)
	}
}

func checkCommand(name string) error {
	_, err := exec.LookPath(name)
	return err
}

func printResult(ok bool, message string) {
	if ok {
		color.Green("  ✅ %s", message)
	} else {
		color.Red("  ❌ %s", message)
	}
}

func buildDoctorConfigStatusMessage(configExists bool, configPath string) string {
	if configExists {
		return fmt.Sprintf("設定ファイルを読み込みました: %s", configPath)
	}

	return fmt.Sprintf("設定ファイルは未作成です（デフォルト値で実行中）: %s", configPath)
}
