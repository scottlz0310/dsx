package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/scottlz0310/dsx/internal/updater"
	"github.com/spf13/cobra"
)

var sysDiscoverManager string

// supportedDiscoverManagers は discover コマンドで対応しているマネージャの一覧です。
var supportedDiscoverManagers = []string{"go"}

// sysDiscoverCmd はインストール済みツールを検出するコマンドです。
var sysDiscoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "インストール済みのツールを検出します",
	Long: `$GOBIN/$GOPATH/bin にインストールされた Go ツールをスキャンし、
go.targets に追加可能な候補を表示します。

例:
  dsx sys discover              # 全対応マネージャをスキャン
  dsx sys discover --manager go # Go バイナリのみスキャン`,
	RunE: runSysDiscover,
}

func init() {
	sysCmd.AddCommand(sysDiscoverCmd)
	sysDiscoverCmd.Flags().StringVar(&sysDiscoverManager, "manager", "", "スキャン対象のマネージャ（例: go）")
}

func runSysDiscover(cmd *cobra.Command, args []string) error {
	managers, err := resolveDiscoverManagers(sysDiscoverManager)
	if err != nil {
		return err
	}

	baseCtx := cmd.Context()
	if baseCtx == nil {
		baseCtx = context.Background()
	}

	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		<-sigCh
		fmt.Println("\n⚠️  中断シグナルを受信しました。処理を終了します...")
		cancel()
	}()

	for _, m := range managers {
		if err := discoverManager(ctx, m); err != nil {
			return err
		}
	}

	return nil
}

// resolveDiscoverManagers は --manager フラグを解決し、対象マネージャ一覧を返します。
// 指定がなければ全対応マネージャを返します。未対応名を指定した場合はエラーを返します。
func resolveDiscoverManagers(manager string) ([]string, error) {
	if manager == "" {
		return supportedDiscoverManagers, nil
	}

	if !isSupportedDiscoverManager(manager) {
		return nil, fmt.Errorf("未対応のマネージャ: %s", manager)
	}

	return []string{manager}, nil
}

// isSupportedDiscoverManager は指定されたマネージャ名が discover で対応しているかを返します。
func isSupportedDiscoverManager(name string) bool {
	for _, m := range supportedDiscoverManagers {
		if m == name {
			return true
		}
	}

	return false
}

// discoverManager は指定マネージャのスキャンを実行します。
func discoverManager(ctx context.Context, manager string) error {
	switch manager {
	case "go":
		return discoverGoManager(ctx)
	default:
		return fmt.Errorf("未対応のマネージャ: %s", manager)
	}
}

// discoverGoManager は Go バイナリをスキャンして結果を表示します。
func discoverGoManager(ctx context.Context) error {
	result, err := updater.DiscoverGoBinaries(ctx)
	if err != nil {
		return fmt.Errorf("go バイナリのスキャンに失敗: %w", err)
	}

	printGoDiscoverResult(result)

	return nil
}

// printGoDiscoverResult は DiscoverResult を指定フォーマットで標準出力に表示します。
func printGoDiscoverResult(result *updater.DiscoverResult) {
	if len(result.Detected) == 0 {
		fmt.Println("[go] 検出されたバイナリはありませんでした。")
	} else {
		fmt.Println("[go] 検出されたバイナリ:")

		for _, info := range result.Detected {
			target := info.UpdateTarget()
			if info.InstalledVersion != "" {
				fmt.Printf("  %-20s %s (インストール済み: %s)\n", info.BinaryName, target, info.InstalledVersion)
			} else {
				fmt.Printf("  %-20s %s\n", info.BinaryName, target)
			}
		}
	}

	if len(result.Skipped) > 0 {
		fmt.Println()
		fmt.Println("[go] スキップ:")

		for _, s := range result.Skipped {
			fmt.Printf("  %-20s %s\n", s.Name, s.Reason)
		}
	}

	fmt.Println()
	fmt.Println("💡 go.targets に追加するには設定ファイルを編集してください。")
}
