package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/scottlz0310/dsx/internal/config"
	"github.com/scottlz0310/dsx/internal/updater"
	"github.com/spf13/cobra"
)

var (
	sysDiscoverManager string
	sysDiscoverApply   bool
	sysDiscoverDryRun  bool
)

// supportedDiscoverManagers は discover コマンドで対応しているマネージャの一覧です。
var supportedDiscoverManagers = []string{"go"}

// sysDiscoverCmd はインストール済みツールを検出するコマンドです。
var sysDiscoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "インストール済みのツールを検出します",
	Long: `$GOBIN/$GOPATH/bin にインストールされた Go ツールをスキャンし、
go.targets に追加可能な候補を表示します。

例:
  dsx sys discover                    # 全対応マネージャをスキャン（表示のみ）
  dsx sys discover --manager go       # Go バイナリのみスキャン
  dsx sys discover --apply            # 検出結果を go.targets に書き込む
  dsx sys discover --apply --dry-run  # 書き込み内容をプレビュー表示（書き込みなし）`,
	RunE: runSysDiscover,
}

func init() {
	sysCmd.AddCommand(sysDiscoverCmd)
	sysDiscoverCmd.Flags().StringVar(&sysDiscoverManager, "manager", "", "スキャン対象のマネージャ（例: go）")
	sysDiscoverCmd.Flags().BoolVar(&sysDiscoverApply, "apply", false, "検出結果を go.targets に書き込む")
	sysDiscoverCmd.Flags().BoolVar(&sysDiscoverDryRun, "dry-run", false, "書き込みは行わず変更内容をプレビュー表示する（--apply と組み合わせて使用）")
}

func runSysDiscover(cmd *cobra.Command, args []string) error {
	if sysDiscoverDryRun && !sysDiscoverApply {
		return fmt.Errorf("--dry-run は --apply と組み合わせて使用してください")
	}

	managers, err := resolveDiscoverManagers(sysDiscoverManager)
	if err != nil {
		return err
	}

	baseCtx := cmd.Context()
	if baseCtx == nil {
		baseCtx = context.Background()
	}

	ctx, stop := signal.NotifyContext(baseCtx, os.Interrupt)
	defer stop()

	for _, m := range managers {
		if err := discoverManager(ctx, m, sysDiscoverApply, sysDiscoverDryRun); err != nil {
			return err
		}
	}

	return nil
}

// resolveDiscoverManagers は --manager フラグを解決し、対象マネージャ一覧を返します。
// 指定がなければ全対応マネージャを返します。未対応名を指定した場合はエラーを返します。
func resolveDiscoverManagers(manager string) ([]string, error) {
	manager = strings.TrimSpace(manager)
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
func discoverManager(ctx context.Context, manager string, apply, dryRun bool) error {
	switch manager {
	case "go":
		return discoverGoManager(ctx, apply, dryRun)
	default:
		return fmt.Errorf("未対応のマネージャ: %s", manager)
	}
}

// discoverGoManager は Go バイナリをスキャンして結果を表示します。
func discoverGoManager(ctx context.Context, apply, dryRun bool) error {
	result, err := updater.DiscoverGoBinaries(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return fmt.Errorf("処理が中断されました")
		}

		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("処理がタイムアウトしました")
		}

		return fmt.Errorf("go バイナリのスキャンに失敗: %w", err)
	}

	printGoDiscoverResult(result)

	if apply {
		return applyGoTargets(result, dryRun)
	}

	fmt.Println("💡 go.targets に追加するには --apply フラグを使用するか、設定ファイルを直接編集してください。")

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
			fmt.Printf("  %-20s 理由: %s\n", s.Name, s.Reason)
		}
	}

	fmt.Println()
}

// applyGoTargets は DiscoverResult を config.yaml の go.targets に反映します。
// dryRun が true の場合、書き込みは行わず変更内容をプレビュー表示します。
func applyGoTargets(result *updater.DiscoverResult, dryRun bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("go.targets 反映のための設定読み込みに失敗: %w", err)
	}

	existingTargets := extractGoTargets(cfg)
	toAdd, skippedCount := mergeGoTargets(existingTargets, result.Detected)

	configPath, err := config.ConfigPath()
	if err != nil {
		return fmt.Errorf("設定ファイルパスの取得に失敗: %w", err)
	}

	fileExists, _, err := config.ConfigFileExists()
	if err != nil {
		return fmt.Errorf("設定ファイルの確認に失敗: %w", err)
	}

	if dryRun {
		printGoApplyDryRun(toAdd, skippedCount, configPath, fileExists)
		return nil
	}

	return writeGoTargets(cfg, existingTargets, toAdd, skippedCount, configPath, fileExists)
}

// extractGoTargets は設定から go.targets を文字列スライスで取得します。
func extractGoTargets(cfg *config.Config) []string {
	if cfg.Sys.Managers == nil {
		return nil
	}

	goManager, ok := cfg.Sys.Managers["go"]
	if !ok {
		return nil
	}

	targets, ok := goManager["targets"]
	if !ok {
		return nil
	}

	switch v := targets.(type) {
	case []interface{}:
		result := make([]string, 0, len(v))

		for _, t := range v {
			if s, ok := t.(string); ok {
				result = append(result, s)
			}
		}

		return result
	case []string:
		return v
	}

	return nil
}

// mergeGoTargets は既存の go.targets と検出バイナリをマージし、追加対象と重複スキップ数を返します。
// 重複判定はパッケージパス（@ より後のバージョン部分を除いた部分）で行います。
// 既存エントリの固定バージョンは変更しません。
func mergeGoTargets(existing []string, detected []updater.GoBinaryInfo) (toAdd []string, skippedCount int) {
	existingPaths := make(map[string]struct{}, len(existing))
	for _, e := range existing {
		existingPaths[packagePathFrom(e)] = struct{}{}
	}

	for _, info := range detected {
		target := info.UpdateTarget()
		if target == "" {
			continue
		}

		pkgPath := packagePathFrom(target)
		if _, ok := existingPaths[pkgPath]; ok {
			skippedCount++
		} else {
			toAdd = append(toAdd, target)
			existingPaths[pkgPath] = struct{}{} // detected 側の重複も除外
		}
	}

	return toAdd, skippedCount
}

// packagePathFrom はターゲット文字列から @ 以降のバージョン部分を除いたパスを返します。
// 例: "golang.org/x/tools/gopls@latest" -> "golang.org/x/tools/gopls"
func packagePathFrom(target string) string {
	if i := strings.LastIndex(target, "@"); i >= 0 {
		return target[:i]
	}

	return target
}

// printGoApplyDryRun は dry-run モードでの変更プレビューを表示します。
func printGoApplyDryRun(toAdd []string, skippedCount int, configPath string, fileExists bool) {
	fmt.Println("[dry-run] go.targets への変更プレビュー（書き込みは行いません）:")

	if !fileExists && len(toAdd) > 0 {
		fmt.Printf("  ⚠️  config.yaml が存在しないため、新規作成されます: %s\n", configPath)
	}

	if len(toAdd) == 0 {
		fmt.Println("  追加対象なし（すべて既存エントリと重複）")
	} else {
		for _, t := range toAdd {
			fmt.Printf("  + %s\n", t)
		}
	}

	fmt.Printf("  スキップ: %d 件（既存エントリと重複）\n", skippedCount)

	if fileExists {
		fmt.Println()
		fmt.Println("  ⚠️  注意: 既存 config.yaml 内のコメントは書き込み時に保持されません。")
	}

	fmt.Println()
	fmt.Println("実際に反映するには --dry-run を外して実行してください: dsx sys discover --apply")
}

// writeGoTargets は go.targets を設定ファイルにアトミックに書き込みます。
func writeGoTargets(cfg *config.Config, existing, toAdd []string, skippedCount int, configPath string, fileExists bool) error {
	if len(toAdd) == 0 {
		fmt.Printf("✅ go.targets に追加する新しいエントリはありませんでした（%d 件スキップ）\n", skippedCount)
		return nil
	}

	if !fileExists {
		fmt.Printf("config.yaml が存在しないため新規作成します: %s\n", configPath)
	}

	merged := append(append([]string(nil), existing...), toAdd...)

	if cfg.Sys.Managers == nil {
		cfg.Sys.Managers = make(map[string]config.ManagerConfig)
	}

	if _, ok := cfg.Sys.Managers["go"]; !ok {
		cfg.Sys.Managers["go"] = make(config.ManagerConfig)
	}

	cfg.Sys.Managers["go"]["targets"] = merged

	backupPath, err := config.SaveAtomic(cfg, "")
	if err != nil {
		return fmt.Errorf("設定ファイルの書き込みに失敗: %w", err)
	}

	fmt.Printf("✅ go.targets を更新しました（%d 件追加、%d 件スキップ）\n", len(toAdd), skippedCount)
	fmt.Printf("   設定ファイル: %s\n", configPath)

	if backupPath != "" {
		fmt.Printf("   バックアップ:  %s\n", backupPath)
	}

	return nil
}
