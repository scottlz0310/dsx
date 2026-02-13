package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/scottlz0310/devsync/internal/config"
	repomgr "github.com/scottlz0310/devsync/internal/repo"
	"github.com/scottlz0310/devsync/internal/runner"
	"github.com/spf13/cobra"
)

var (
	repoCleanupJobs    int
	repoCleanupDryRun  bool
	repoCleanupTUI     bool
	repoCleanupNoTUI   bool
	repoCleanupLogFile string
)

var repoCleanupStep = repomgr.Cleanup

var repoCleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "マージ済みローカルブランチを整理します",
	Long: `設定された root 配下の Git リポジトリに対して
マージ済み（および設定により squashed 判定）ブランチを削除します。

注意:
  - cleanup はローカルブランチ削除を伴うため、未コミット変更/stash/detached HEAD を検出した場合は安全側にスキップします。
  - squashed 判定（PR は merged だが git 的には未マージなブランチの削除）は GitHub の PR 情報を利用します。`,
	RunE: runRepoCleanup,
}

func init() {
	repoCmd.AddCommand(repoCleanupCmd)

	repoCleanupCmd.Flags().StringVar(&repoRootOverride, "root", "", "cleanup 対象のルートディレクトリ（指定時は設定を上書き）")
	repoCleanupCmd.Flags().IntVarP(&repoCleanupJobs, "jobs", "j", 0, "並列実行数（0以下の場合は設定値または1を使用）")
	repoCleanupCmd.Flags().BoolVarP(&repoCleanupDryRun, "dry-run", "n", false, "実際の削除は行わず、計画のみ表示")
	repoCleanupCmd.Flags().BoolVar(&repoCleanupTUI, "tui", false, "Bubble Tea の進捗UIを表示（既定値は config.yaml の ui.tui）")
	repoCleanupCmd.Flags().BoolVar(&repoCleanupNoTUI, "no-tui", false, "TUI 進捗表示を無効化（設定より優先）")
	repoCleanupCmd.Flags().StringVar(&repoCleanupLogFile, "log-file", "", "ジョブ実行ログをファイルに保存")
}

func runRepoCleanup(cmd *cobra.Command, args []string) error {
	cfg, configExists, configPath := loadRepoConfig()

	if !cfg.Repo.Cleanup.Enabled {
		fmt.Println("📝 repo.cleanup.enabled=false のため repo cleanup は無効です")
		return nil
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
		return wrapRepoRootError(err, root, cmd.Flags().Changed("root"), configExists, configPath)
	}

	if len(repoPaths) == 0 {
		fmt.Printf("📝 cleanup 対象のリポジトリが見つかりませんでした: %s\n", root)
		return nil
	}

	opts := buildRepoCleanupOptions(cmd, cfg)

	tuiReq, err := resolveTUIRequest(cfg.UI.TUI, cmd.Flags().Changed("tui"), repoCleanupTUI, cmd.Flags().Changed("no-tui"), repoCleanupNoTUI)
	if err != nil {
		return err
	}

	useTUI, warning := resolveTUIEnabled(tuiReq)
	printTUIWarning(warning)

	jobs := resolveRepoJobs(cfg.Control.Concurrency, repoCleanupJobs)

	if useTUI {
		fmt.Println("🖥️  TUI 進捗表示を有効化しました")
	}

	fmt.Printf("🧹 repo cleanup を開始します (%d件, 並列=%d)\n", len(repoPaths), jobs)

	if opts.DryRun {
		fmt.Println("📋 DryRun モード: 実際の削除は行いません")
	}

	fmt.Println()

	execJobs := buildRepoCleanupJobs(root, repoPaths, opts, useTUI)
	summary := runJobsWithOptionalTUI(ctx, "repo cleanup 進捗", jobs, execJobs, useTUI, repoCleanupLogFile)

	printRepoCleanupSummary(summary)

	if summary.Failed > 0 {
		return fmt.Errorf("%d 件の repo cleanup に失敗しました", summary.Failed)
	}

	if summary.Skipped > 0 {
		return fmt.Errorf("キャンセルまたはタイムアウトにより %d 件をスキップしました", summary.Skipped)
	}

	fmt.Println("✅ repo cleanup が完了しました")

	return nil
}

func buildRepoCleanupOptions(cmd *cobra.Command, cfg *config.Config) repomgr.CleanupOptions {
	opts := repomgr.CleanupOptions{
		Prune:           cfg.Repo.Sync.Prune,
		DryRun:          cfg.Control.DryRun,
		Targets:         cfg.Repo.Cleanup.Target,
		ExcludeBranches: cfg.Repo.Cleanup.ExcludeBranches,
	}

	if cmd.Flags().Changed("dry-run") {
		opts.DryRun = repoCleanupDryRun
	}

	return opts
}

func buildRepoCleanupJobs(root string, repoPaths []string, opts repomgr.CleanupOptions, useTUI bool) []runner.Job {
	var outputMu sync.Mutex

	nameCounts := make(map[string]int, len(repoPaths))
	for _, path := range repoPaths {
		displayName := buildRepoJobDisplayName(root, path)
		nameCounts[displayName]++
	}

	execJobs := make([]runner.Job, 0, len(repoPaths))
	for _, path := range repoPaths {
		repoPath := path

		repoName := buildRepoJobDisplayName(root, repoPath)
		if nameCounts[repoName] > 1 {
			repoName = filepath.Clean(repoPath)
		}

		execJobs = append(execJobs, runner.Job{
			Name: repoName,
			Run: func(jobCtx context.Context) error {
				cleanupResult, cleanupErr := runRepoCleanupJob(jobCtx, repoPath, opts)

				if !useTUI {
					outputMu.Lock()
					printRepoCleanupResult(repoName, cleanupResult, cleanupErr)
					outputMu.Unlock()
				}

				return cleanupErr
			},
		})
	}

	return execJobs
}

func runRepoCleanupJob(ctx context.Context, repoPath string, opts repomgr.CleanupOptions) (*repomgr.CleanupResult, error) {
	cleanupOpts, warnings := prepareRepoCleanupOptions(ctx, repoPath, opts)

	cleanupResult, cleanupErr := repoCleanupStep(ctx, repoPath, cleanupOpts)

	if cleanupResult != nil && len(warnings) > 0 {
		cleanupResult.SkippedMessages = append(cleanupResult.SkippedMessages, warnings...)
	}

	return cleanupResult, cleanupErr
}

func prepareRepoCleanupOptions(ctx context.Context, repoPath string, opts repomgr.CleanupOptions) (prepared repomgr.CleanupOptions, warnings []string) {
	if !wantsCleanupTarget(opts.Targets, "squashed") {
		return opts, nil
	}

	defaultInfo, err := repomgr.DetectDefaultBranch(ctx, repoPath)
	if err != nil {
		return opts, []string{fmt.Sprintf("squashed 判定の準備に失敗したためスキップしました: %v", err)}
	}

	if strings.TrimSpace(defaultInfo.Branch) == "" {
		return opts, []string{"squashed 判定の準備に失敗したためスキップしました: デフォルトブランチ名が空です"}
	}

	heads, err := listMergedPRHeads(ctx, repoPath, defaultInfo.Branch)
	if err != nil {
		return opts, []string{fmt.Sprintf("squashed 判定をスキップしました: %v", err)}
	}

	opts.SquashedPRHeadByBranch = heads.Heads

	warnings = make([]string, 0, 1)
	if heads.Warning != "" {
		warnings = append(warnings, heads.Warning)
	}

	return opts, warnings
}

func wantsCleanupTarget(targets []string, want string) bool {
	for _, t := range targets {
		if strings.EqualFold(strings.TrimSpace(t), want) {
			return true
		}
	}

	return false
}

type mergedPR struct {
	HeadRefName string `json:"headRefName"`
	HeadRefOID  string `json:"headRefOid"`
	MergedAt    string `json:"mergedAt"`
}

type mergedPRHeadsResult struct {
	Heads   map[string]string
	Warning string
}

func listMergedPRHeads(ctx context.Context, repoPath, baseBranch string) (mergedPRHeadsResult, error) {
	if _, err := repoLookPathStep("gh"); err != nil {
		return mergedPRHeadsResult{}, fmt.Errorf("gh コマンドが見つかりません: %w", err)
	}

	output, stderr, err := runGhOutputWithRetry(
		ctx,
		repoPath,
		"pr",
		"list",
		"--state",
		"merged",
		"--base",
		baseBranch,
		"--limit",
		strconv.Itoa(githubPullRequestListLimit),
		"--json",
		"headRefName,headRefOid,mergedAt",
	)
	if err != nil {
		msg := strings.TrimSpace(stderr)
		if msg != "" {
			return mergedPRHeadsResult{}, fmt.Errorf("gh pr list の実行に失敗しました: %w: %s", err, msg)
		}

		return mergedPRHeadsResult{}, fmt.Errorf("gh pr list の実行に失敗しました: %w", err)
	}

	var prs []mergedPR
	if err := json.Unmarshal(output, &prs); err != nil {
		return mergedPRHeadsResult{}, fmt.Errorf("PR 一覧の解析に失敗: %w", err)
	}

	latest := make(map[string]mergedPR, len(prs))
	for _, pr := range prs {
		head := strings.TrimSpace(pr.HeadRefName)

		oid := strings.TrimSpace(pr.HeadRefOID)
		if head == "" || oid == "" {
			continue
		}

		prev, ok := latest[head]
		if !ok || pr.MergedAt > prev.MergedAt {
			latest[head] = pr
		}
	}

	result := make(map[string]string, len(latest))
	for branch, pr := range latest {
		result[branch] = strings.TrimSpace(pr.HeadRefOID)
	}

	if len(prs) == githubPullRequestListLimit {
		return mergedPRHeadsResult{
			Heads:   result,
			Warning: fmt.Sprintf("⚠️  gh pr list の取得件数が上限 (%d件) に達しました。squashed 判定が一部欠ける可能性があります。", githubPullRequestListLimit),
		}, nil
	}

	return mergedPRHeadsResult{
		Heads: result,
	}, nil
}

func printRepoCleanupResult(name string, result *repomgr.CleanupResult, cleanupErr error) {
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("📁 %s\n", name)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	if result != nil {
		for _, command := range result.Commands {
			fmt.Printf("  $ %s\n", command)
		}

		for _, plan := range result.PlannedDeletes {
			suffix := plan.Target
			if plan.Force {
				suffix += ", 強制"
			}

			fmt.Printf("  📝 削除予定: %s (%s)\n", plan.Branch, suffix)
		}

		for _, deleted := range result.DeletedBranches {
			suffix := deleted.Target
			if deleted.Force {
				suffix += ", 強制"
			}

			fmt.Printf("  🗑️  削除: %s (%s)\n", deleted.Branch, suffix)
		}

		for _, msg := range result.SkippedMessages {
			fmt.Printf("  ⚪ %s\n", msg)
		}

		for _, err := range result.Errors {
			fmt.Printf("  ❌ %v\n", err)
		}
	}

	if cleanupErr == nil {
		fmt.Println("  ✅ 成功")
		fmt.Println()
		return
	}

	if isContextCancellation(cleanupErr) {
		fmt.Printf("  ⚪ スキップ: %v\n\n", cleanupErr)
		return
	}

	fmt.Printf("  ❌ 失敗: %v\n\n", cleanupErr)
}

func printRepoCleanupSummary(summary runner.Summary) {
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("📊 repo cleanup サマリー")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  対象: %d 件\n", summary.Total)
	fmt.Printf("  成功: %d 件\n", summary.Success)
	fmt.Printf("  失敗: %d 件\n", summary.Failed)
	fmt.Printf("  スキップ: %d 件\n", summary.Skipped)
	fmt.Println()
}
