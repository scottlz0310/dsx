package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/scottlz0310/dsx/internal/restore"
	"github.com/scottlz0310/dsx/internal/snapshot"
	"github.com/scottlz0310/dsx/internal/updater"
	"github.com/spf13/cobra"
)

func newSysRestoreCmd(managers func() map[string]restore.Manager, run restore.Runner) *cobra.Command {
	var (
		file    string
		dryRun  bool
		timeout time.Duration
	)

	cmd := &cobra.Command{
		Use:   "restore",
		Short: "保存したツール構成を復元します（--dry-run で計画のみ表示）",
		Long: `manifest と現在の構成を比較し、提供版・source を照会して復元計画を作成します。
--dry-run を指定すると計画の表示だけを行い、インストールは実行しません。
--dry-run を省略すると、「追加候補」の項目だけをインストールします。
既存ツールの削除・ダウングレードは行いません。解決不能な項目が1件でもある場合は、何も実行しません。
計画の作成には公開レジストリへの読み取り専用の通信とマネージャの一覧・照会コマンドを使用します。

例:
  dsx sys restore --file dsx-tools.yaml --dry-run
  dsx sys restore --file dsx-tools.yaml`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			manifest, err := loadRestoreManifest(file)
			if err != nil {
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

			plan, planErr := restore.Build(ctx, manifest, managers(), runtime.GOOS)
			if plan != nil {
				printRestorePlan(cmd, plan, dryRun)
			}

			if planErr != nil {
				return fmt.Errorf("復元計画の作成に失敗: %w", planErr)
			}

			if count := plan.UnresolvedCount(); count > 0 {
				return fmt.Errorf("復元計画に解決不能な項目が %d 件あります", count)
			}

			if dryRun {
				return nil
			}

			return executeRestore(ctx, cmd, plan, run)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "dsx-tools.yaml", "入力の YAML manifest")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "復元計画のみ表示（インストールしない）")
	cmd.Flags().DurationVarP(&timeout, "timeout", "t", 10*time.Minute, "全体のタイムアウト（未指定時は control.timeout）")

	return cmd
}

func loadRestoreManifest(path string) (*snapshot.Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("manifest の読み込みに失敗: %w", err)
	}

	manifest, decodeErr := snapshot.Decode(file)
	if err := errors.Join(decodeErr, file.Close()); err != nil {
		return nil, fmt.Errorf("manifest の検証に失敗: %w", err)
	}

	return manifest, nil
}

func printRestorePlan(cmd *cobra.Command, plan *restore.Plan, dryRun bool) {
	if dryRun {
		cmd.Println("復元計画（インストールは実行しません）")
	} else {
		cmd.Println("復元計画")
	}

	for i := range plan.Entries {
		entry := &plan.Entries[i]

		requestedVersion := entry.Tool.Version
		if requestedVersion == "" {
			requestedVersion = "未固定"
		}

		cmd.Printf("[%s] %s %s@%s: %s\n", entry.Manager, entry.State, entry.Tool.Package, requestedVersion, entry.Message)

		if entry.Tool.Source != "" {
			cmd.Printf("  source: %s\n", entry.Tool.Source)
		}

		if len(entry.Command.Args) > 0 {
			// 引数境界を保持し、シェル用の文字列として誤解されない形式にします。
			cmd.Printf("  予定コマンド（実行ファイル・引数の配列）: %q\n", entry.Command.Args)

			if entry.Command.Version != "" {
				cmd.Printf("  解決した版: %s\n", entry.Command.Version)
			}
		}
	}

	cmd.Printf("計 %d 件、解決不能 %d 件\n", len(plan.Entries), plan.UnresolvedCount())
}

// executeRestore は追加候補をインストールし、失敗した項目をマネージャ・パッケージ付きで集約して報告します。
func executeRestore(ctx context.Context, cmd *cobra.Command, plan *restore.Plan, run restore.Runner) error {
	results, err := restore.Execute(ctx, plan, run)
	if err != nil {
		return fmt.Errorf("復元の実行を中断しました: %w", err)
	}

	var failures []error

	for i := range results {
		entry := &results[i].Entry
		if results[i].Err == nil {
			cmd.Printf("[%s] %s のインストールが完了しました\n", entry.Manager, entry.Tool.Package)
			continue
		}

		cmd.Printf("[%s] %s のインストールに失敗しました: %v\n", entry.Manager, entry.Tool.Package, results[i].Err)
		failures = append(failures, fmt.Errorf("%s %s: %w", entry.Manager, entry.Tool.Package, results[i].Err))
	}

	cmd.Printf("インストール %d 件中、成功 %d 件、失敗 %d 件\n", len(results), len(results)-len(failures), len(failures))

	if len(failures) > 0 {
		return fmt.Errorf("復元に失敗した項目が %d 件あります: %w", len(failures), errors.Join(failures...))
	}

	return nil
}

func init() {
	sysCmd.AddCommand(newSysRestoreCmd(func() map[string]restore.Manager {
		return updater.RestoreManagers(updater.All())
	}, updater.RunRestoreCommand))
}
