package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/scottlz0310/devsync/internal/config"
	repomgr "github.com/scottlz0310/devsync/internal/repo"
	"github.com/scottlz0310/devsync/internal/runner"
	"github.com/spf13/cobra"
)

var (
	repoRootOverride      string
	repoUpdateJobs        int
	repoUpdateDryRun      bool
	repoUpdateSubmodules  bool
	repoUpdateNoSubmodule bool
)

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "リポジトリ管理",
	Long:  `管理対象リポジトリの検出・状態確認・更新を行います。`,
}

var repoListCmd = &cobra.Command{
	Use:   "list",
	Short: "管理下リポジトリの一覧を表示します",
	Long: `設定された root 配下の Git リポジトリを検出し、
状態（クリーン/ダーティ/未プッシュ/追跡なし）を表示します。`,
	RunE: runRepoList,
}

var repoUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "管理下リポジトリを更新します",
	Long: `設定された root 配下の Git リポジトリに対して
fetch/pull/submodule update を実行します。`,
	RunE: runRepoUpdate,
}

func init() {
	rootCmd.AddCommand(repoCmd)
	repoCmd.AddCommand(repoListCmd)
	repoCmd.AddCommand(repoUpdateCmd)

	repoListCmd.Flags().StringVar(&repoRootOverride, "root", "", "スキャン対象のルートディレクトリ（指定時は設定を上書き）")
	repoUpdateCmd.Flags().StringVar(&repoRootOverride, "root", "", "更新対象のルートディレクトリ（指定時は設定を上書き）")
	repoUpdateCmd.Flags().IntVarP(&repoUpdateJobs, "jobs", "j", 0, "並列実行数（0以下の場合は設定値または1を使用）")
	repoUpdateCmd.Flags().BoolVarP(&repoUpdateDryRun, "dry-run", "n", false, "実際の更新は行わず、計画のみ表示")
	repoUpdateCmd.Flags().BoolVar(&repoUpdateSubmodules, "submodule", false, "submodule update を有効化する（設定値を上書き）")
	repoUpdateCmd.Flags().BoolVar(&repoUpdateNoSubmodule, "no-submodule", false, "submodule update を無効化する（設定値を上書き）")
}

func runRepoList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  設定ファイルの読み込みに失敗（デフォルト設定を使用）: %v\n", err)

		cfg = config.Default()
	}

	root := cfg.Repo.Root
	if cmd.Flags().Changed("root") {
		root = repoRootOverride
	}

	timeout := 10 * time.Minute
	if parsed, parseErr := time.ParseDuration(cfg.Control.Timeout); parseErr == nil {
		timeout = parsed
	}

	baseCtx := cmd.Context()
	if baseCtx == nil {
		baseCtx = context.Background()
	}

	ctx, cancel := context.WithTimeout(baseCtx, timeout)
	defer cancel()

	repos, err := repomgr.List(ctx, root)
	if err != nil {
		return err
	}

	if len(repos) == 0 {
		fmt.Printf("📝 リポジトリが見つかりませんでした: %s\n", root)
		return nil
	}

	fmt.Printf("📦 管理下リポジトリ一覧 (%d件)\n\n", len(repos))

	if err := printRepoTable(repos); err != nil {
		return fmt.Errorf("一覧表示に失敗: %w", err)
	}

	return nil
}

func runRepoUpdate(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  設定ファイルの読み込みに失敗（デフォルト設定を使用）: %v\n", err)

		cfg = config.Default()
	}

	root := cfg.Repo.Root
	if cmd.Flags().Changed("root") {
		root = repoRootOverride
	}

	timeout := 10 * time.Minute
	if parsed, parseErr := time.ParseDuration(cfg.Control.Timeout); parseErr == nil {
		timeout = parsed
	}

	baseCtx := cmd.Context()
	if baseCtx == nil {
		baseCtx = context.Background()
	}

	ctx, cancel := context.WithTimeout(baseCtx, timeout)
	defer cancel()

	repoPaths, err := repomgr.Discover(root)
	if err != nil {
		return err
	}

	if len(repoPaths) == 0 {
		fmt.Printf("📝 更新対象のリポジトリが見つかりませんでした: %s\n", root)
		return nil
	}

	opts := repomgr.UpdateOptions{
		Prune:           cfg.Repo.Sync.Prune,
		AutoStash:       cfg.Repo.Sync.AutoStash,
		SubmoduleUpdate: cfg.Repo.Sync.SubmoduleUpdate,
		DryRun:          cfg.Control.DryRun,
	}

	if cmd.Flags().Changed("dry-run") {
		opts.DryRun = repoUpdateDryRun
	}

	enableSubmodule := cmd.Flags().Changed("submodule") && repoUpdateSubmodules
	disableSubmodule := cmd.Flags().Changed("no-submodule") && repoUpdateNoSubmodule

	submoduleUpdate, err := resolveRepoSubmoduleUpdate(opts.SubmoduleUpdate, enableSubmodule, disableSubmodule)
	if err != nil {
		return err
	}

	opts.SubmoduleUpdate = submoduleUpdate

	jobs := resolveRepoJobs(cfg.Control.Concurrency, repoUpdateJobs)

	fmt.Printf("🔄 リポジトリ更新を開始します (%d件, 並列=%d)\n", len(repoPaths), jobs)

	if opts.DryRun {
		fmt.Println("📋 DryRun モード: 実際の更新は行いません")
	}

	fmt.Println()

	var (
		outputMu sync.Mutex
		execJobs = make([]runner.Job, 0, len(repoPaths))
	)

	for _, path := range repoPaths {
		repoPath := path
		repoName := filepath.Base(repoPath)

		execJobs = append(execJobs, runner.Job{
			Name: repoName,
			Run: func(jobCtx context.Context) error {
				updateResult, updateErr := repomgr.Update(jobCtx, repoPath, opts)

				outputMu.Lock()
				printRepoUpdateResult(repoName, updateResult, updateErr)
				outputMu.Unlock()

				return updateErr
			},
		})
	}

	summary := runner.Execute(ctx, jobs, execJobs)
	printRepoUpdateSummary(summary)

	if summary.Failed > 0 {
		return fmt.Errorf("%d 件のリポジトリ更新に失敗しました", summary.Failed)
	}

	if summary.Skipped > 0 {
		return fmt.Errorf("キャンセルまたはタイムアウトにより %d 件をスキップしました", summary.Skipped)
	}

	fmt.Println("✅ リポジトリ更新が完了しました")

	return nil
}

func printRepoTable(repos []repomgr.Info) error {
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)

	if _, err := fmt.Fprintln(writer, "名前\t状態\tAhead\tパス"); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(writer, "----\t----\t-----\t----"); err != nil {
		return err
	}

	for _, repo := range repos {
		ahead := "-"
		if repo.HasUpstream {
			ahead = strconv.Itoa(repo.Ahead)
		}

		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", repo.Name, repomgr.StatusLabel(repo.Status), ahead, repo.Path); err != nil {
			return err
		}
	}

	return writer.Flush()
}

func printRepoUpdateResult(name string, result *repomgr.UpdateResult, updateErr error) {
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("📁 %s\n", name)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	if result != nil {
		for _, command := range result.Commands {
			fmt.Printf("  $ %s\n", command)
		}

		for _, message := range result.SkippedMessages {
			fmt.Printf("  ⚪ %s\n", message)
		}
	}

	if updateErr == nil {
		fmt.Println("  ✅ 成功")
		fmt.Println()
		return
	}

	if isContextCancellation(updateErr) {
		fmt.Printf("  ⚪ スキップ: %v\n\n", updateErr)
		return
	}

	fmt.Printf("  ❌ 失敗: %v\n\n", updateErr)
}

func printRepoUpdateSummary(summary runner.Summary) {
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("📊 repo update サマリー")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  対象: %d 件\n", summary.Total)
	fmt.Printf("  成功: %d 件\n", summary.Success)
	fmt.Printf("  失敗: %d 件\n", summary.Failed)
	fmt.Printf("  スキップ: %d 件\n", summary.Skipped)
	fmt.Println()
}

func resolveRepoJobs(configJobs, flagJobs int) int {
	if flagJobs > 0 {
		return flagJobs
	}

	if configJobs > 0 {
		return configJobs
	}

	return 1
}

func resolveRepoSubmoduleUpdate(configValue, enableOverride, disableOverride bool) (bool, error) {
	if enableOverride && disableOverride {
		return false, fmt.Errorf("--submodule と --no-submodule は同時指定できません")
	}

	if enableOverride {
		return true, nil
	}

	if disableOverride {
		return false, nil
	}

	return configValue, nil
}
