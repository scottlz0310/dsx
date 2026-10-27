package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/scottlz0310/dsx/internal/config"
	"github.com/scottlz0310/dsx/internal/snapshot"
	"github.com/scottlz0310/dsx/internal/updater"
	"github.com/spf13/cobra"
)

func newSysSnapshotCmd(managers func() []updater.Updater) *cobra.Command {
	var (
		output  string
		force   bool
		timeout time.Duration
	)

	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "インストール済みグローバルツールを YAML に保存します",
		Long: `sys.enable に関係なく、利用可能な対応マネージャのツール構成を保存します。
取得失敗時は保存しません。生成後に不要な項目を手動で削除して管理してください。

例:
  dsx sys snapshot --output dsx-tools.yaml
  dsx sys snapshot --output dsx-tools.yaml --force`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := snapshot.CheckOutput(output, force); err != nil {
				return err
			}

			duration, err := snapshotTimeout(cmd, timeout)
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()

			ctx, cancel := context.WithTimeout(ctx, duration)
			defer cancel()

			manifest, statuses, err := updater.CollectSnapshot(ctx, managers())
			for _, status := range statuses {
				cmd.Printf("[%s] %s\n", status.Manager, status.Message)
			}

			if err != nil {
				return err
			}

			if err = ctx.Err(); err != nil {
				return fmt.Errorf("スナップショットを保存できません: %w", err)
			}

			if err := snapshot.Save(output, manifest, force); err != nil {
				return err
			}

			cmd.Printf("スナップショットを保存しました: %s\n不要な項目を確認・編集して管理してください。\n", output)

			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "dsx-tools.yaml", "保存先の YAML ファイル")
	cmd.Flags().BoolVar(&force, "force", false, "既存ファイルの上書きを許可")
	cmd.Flags().DurationVarP(&timeout, "timeout", "t", 10*time.Minute, "全体のタイムアウト（未指定時は control.timeout）")

	return cmd
}

func snapshotTimeout(cmd *cobra.Command, timeout time.Duration) (time.Duration, error) {
	if !cmd.Flags().Changed("timeout") {
		cfg, err := config.Load()
		if err != nil {
			return 0, fmt.Errorf("設定の読み込みに失敗: %w", err)
		}

		if cfg.Control.Timeout != "" {
			timeout, err = time.ParseDuration(cfg.Control.Timeout)
			if err != nil {
				return 0, fmt.Errorf("control.timeout が不正です: %w", err)
			}
		}
	}

	if timeout <= 0 {
		return 0, fmt.Errorf("タイムアウトは正の時間を指定してください")
	}

	return timeout, nil
}

func init() {
	sysCmd.AddCommand(newSysSnapshotCmd(updater.All))
}
