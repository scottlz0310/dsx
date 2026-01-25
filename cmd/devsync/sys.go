package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/scottlz0310/devsync/internal/config"
	"github.com/scottlz0310/devsync/internal/updater"

	// 各マネージャを init() で登録させるためにインポート
	_ "github.com/scottlz0310/devsync/internal/updater"
	"github.com/spf13/cobra"
)

var (
	sysDryRun  bool
	sysVerbose bool
	sysTimeout string
)

// sysCmd はシステム関連コマンドのルートです
var sysCmd = &cobra.Command{
	Use:   "sys",
	Short: "システムパッケージの管理",
	Long:  `システムパッケージの更新・管理を行うサブコマンド群です。`,
}

// sysUpdateCmd はシステムパッケージを更新するコマンドです
var sysUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "システムパッケージを更新します",
	Long: `設定ファイルで有効化されたパッケージマネージャを使用して、
システムパッケージを最新版に更新します。

対応マネージャ:
  - apt    (Debian/Ubuntu)
  - brew   (macOS/Linux Homebrew)
  - go     (Go ツール)

例:
  devsync sys update           # 設定に基づいて更新
  devsync sys update --dry-run # 更新計画のみ表示
  devsync sys update -v        # 詳細ログを表示`,
	RunE: runSysUpdate,
}

// sysListCmd は利用可能なマネージャを一覧表示します
var sysListCmd = &cobra.Command{
	Use:   "list",
	Short: "利用可能なパッケージマネージャを一覧表示します",
	RunE:  runSysList,
}

func init() {
	rootCmd.AddCommand(sysCmd)
	sysCmd.AddCommand(sysUpdateCmd)
	sysCmd.AddCommand(sysListCmd)

	// フラグの定義
	sysUpdateCmd.Flags().BoolVarP(&sysDryRun, "dry-run", "n", false, "実際の更新は行わず、計画のみ表示")
	sysUpdateCmd.Flags().BoolVarP(&sysVerbose, "verbose", "v", false, "詳細なログを出力")
	sysUpdateCmd.Flags().StringVarP(&sysTimeout, "timeout", "t", "10m", "全体のタイムアウト時間")
}

func runSysUpdate(cmd *cobra.Command, args []string) error {
	fmt.Println("🔄 システムパッケージの更新を開始します...")
	fmt.Println()

	// 設定の読み込み
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  設定ファイルの読み込みに失敗（デフォルト設定を使用）: %v\n", err)
		cfg = config.Default()
	}

	// DryRun フラグの適用（コマンドラインが優先）
	if cmd.Flags().Changed("dry-run") {
		cfg.Control.DryRun = sysDryRun
	}

	// タイムアウトの設定
	timeout, err := time.ParseDuration(sysTimeout)
	if err != nil {
		return fmt.Errorf("タイムアウト値が不正です: %w", err)
	}

	// コンテキストの作成（タイムアウト + キャンセル対応）
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Ctrl+C でキャンセル可能に
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		fmt.Println("\n⚠️  中断シグナルを受信しました。処理を終了します...")
		cancel()
	}()

	// 有効なマネージャを取得
	enabledUpdaters, err := updater.GetEnabled(&cfg.Sys)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  %v\n", err)
	}

	// 有効なマネージャがない場合は利用可能なものを表示
	if len(enabledUpdaters) == 0 {
		fmt.Println("📝 有効化されたマネージャがありません。")
		fmt.Println()
		fmt.Println("利用可能なマネージャ:")
		for _, u := range updater.Available() {
			fmt.Printf("  - %s (%s)\n", u.Name(), u.DisplayName())
		}
		fmt.Println()
		fmt.Println("💡 config.yaml の sys.enable で使用するマネージャを指定してください。")
		fmt.Println("   例: enable: [\"apt\", \"go\"]")
		return nil
	}

	// 更新オプション
	opts := updater.UpdateOptions{
		DryRun:  cfg.Control.DryRun,
		Verbose: sysVerbose,
	}

	if opts.DryRun {
		fmt.Println("📋 DryRun モード: 実際の更新は行いません")
		fmt.Println()
	}

	// 各マネージャで更新を実行
	var totalUpdated, totalFailed int
	var allErrors []error

	for _, u := range enabledUpdaters {
		select {
		case <-ctx.Done():
			return fmt.Errorf("タイムアウトまたはキャンセルされました")
		default:
		}

		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		fmt.Printf("📦 %s (%s)\n", u.DisplayName(), u.Name())
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

		result, err := u.Update(ctx, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ エラー: %v\n", err)
			allErrors = append(allErrors, fmt.Errorf("%s: %w", u.Name(), err))
			totalFailed++
			continue
		}

		if result.Message != "" {
			fmt.Printf("✅ %s\n", result.Message)
		}

		if sysVerbose && len(result.Packages) > 0 {
			fmt.Println("  更新パッケージ:")
			for _, pkg := range result.Packages {
				if pkg.CurrentVersion != "" {
					fmt.Printf("    - %s: %s → %s\n", pkg.Name, pkg.CurrentVersion, pkg.NewVersion)
				} else {
					fmt.Printf("    - %s %s\n", pkg.Name, pkg.NewVersion)
				}
			}
		}

		if len(result.Errors) > 0 {
			for _, e := range result.Errors {
				fmt.Fprintf(os.Stderr, "  ⚠️  %v\n", e)
			}
			allErrors = append(allErrors, result.Errors...)
		}

		totalUpdated += result.UpdatedCount
		totalFailed += result.FailedCount
		fmt.Println()
	}

	// サマリー
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("📊 更新サマリー")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  更新成功: %d 件\n", totalUpdated)
	if totalFailed > 0 {
		fmt.Printf("  失敗: %d 件\n", totalFailed)
	}
	if len(allErrors) > 0 {
		fmt.Printf("  エラー数: %d\n", len(allErrors))
	}

	if len(allErrors) > 0 {
		return fmt.Errorf("%d 件のエラーが発生しました", len(allErrors))
	}

	fmt.Println()
	fmt.Println("✅ システムパッケージの更新が完了しました")
	return nil
}

func runSysList(cmd *cobra.Command, args []string) error {
	fmt.Println("📋 パッケージマネージャ一覧")
	fmt.Println()

	// 設定の読み込み
	cfg, err := config.Load()
	if err != nil {
		cfg = config.Default()
	}

	enabledSet := make(map[string]bool)
	for _, name := range cfg.Sys.Enable {
		enabledSet[name] = true
	}

	// 登録されている全マネージャを表示
	allUpdaters := updater.All()
	if len(allUpdaters) == 0 {
		fmt.Println("  (登録されているマネージャがありません)")
		return nil
	}

	fmt.Println("名前       | 表示名                    | 利用可能 | 有効")
	fmt.Println("-----------|---------------------------|----------|------")
	for _, u := range allUpdaters {
		available := "❌"
		if u.IsAvailable() {
			available = "✅"
		}
		enabled := "  "
		if enabledSet[u.Name()] {
			enabled = "✅"
		}
		fmt.Printf("%-10s | %-25s | %s       | %s\n",
			u.Name(), u.DisplayName(), available, enabled)
	}

	fmt.Println()
	fmt.Println("💡 マネージャを有効化するには config.yaml の sys.enable を編集してください。")

	return nil
}
