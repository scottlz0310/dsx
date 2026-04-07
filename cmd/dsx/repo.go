package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/scottlz0310/dsx/internal/config"
	repomgr "github.com/scottlz0310/dsx/internal/repo"
	"github.com/scottlz0310/dsx/internal/runner"
	"github.com/spf13/cobra"
)

var (
	repoRootOverride      string
	repoUpdateJobs        int
	repoUpdateDryRun      bool
	repoUpdateSubmodules  bool
	repoUpdateNoSubmodule bool
	repoUpdateTUI         bool
	repoUpdateNoTUI       bool
	repoUpdateLogFile     string
)

var (
	repoListGitHubReposStep = listGitHubRepos
	repoCloneRepoStep       = cloneRepo
	repoLookPathStep        = exec.LookPath
	repoExecCommandStep     = exec.CommandContext
)

const (
	githubRepoListLimit        = 1000
	githubPullRequestListLimit = 200
)

type bootstrapResult struct {
	ReadyPaths  []string
	PlannedOnly int
}

type githubRepo struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	SSHURL     string `json:"sshUrl"`
	IsArchived bool   `json:"isArchived"`
}

type bootstrapRepoOutcome struct {
	ReadyPath string
	Planned   bool
}

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
	repoUpdateCmd.Flags().BoolVar(&repoUpdateTUI, "tui", false, "Bubble Tea の進捗UIを表示（既定値は config.yaml の ui.tui）")
	repoUpdateCmd.Flags().BoolVar(&repoUpdateNoTUI, "no-tui", false, "TUI 進捗表示を無効化（設定より優先）")
	repoUpdateCmd.Flags().StringVar(&repoUpdateLogFile, "log-file", "", "ジョブ実行ログをファイルに保存")
}

func runRepoList(cmd *cobra.Command, args []string) error {
	cfg, configExists, configPath := loadRepoConfig()

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
		return wrapRepoRootError(err, root, cmd.Flags().Changed("root"), configExists, configPath)
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
	cfg, configExists, configPath := loadRepoConfig()

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

	opts, err := buildRepoUpdateOptions(cmd, cfg)
	if err != nil {
		return err
	}

	tuiReq, err := resolveTUIRequest(cfg.UI.TUI, cmd.Flags().Changed("tui"), repoUpdateTUI, cmd.Flags().Changed("no-tui"), repoUpdateNoTUI)
	if err != nil {
		return err
	}

	useTUI, warning := resolveTUIEnabled(tuiReq)
	printTUIWarning(warning)

	bootstrap, bootstrapErr := bootstrapReposFromGitHub(ctx, root, cfg, opts.DryRun)
	if bootstrapErr != nil {
		return fmt.Errorf("GitHub リポジトリの取得に失敗しました: %w", bootstrapErr)
	}

	repoPaths = mergeRepoPaths(repoPaths, bootstrap.ReadyPaths)
	if len(repoPaths) == 0 {
		printNoTargetResult(root, bootstrap, tuiReq)
		return nil
	}

	jobs := resolveRepoJobs(cfg.Control.Concurrency, repoUpdateJobs)

	// TUI 使用時は開始メッセージを抑制（TUI が画面を制御するため）
	if !useTUI {
		fmt.Printf("🔄 リポジトリ更新を開始します (%d件, 並列=%d)\n", len(repoPaths), jobs)

		if opts.DryRun {
			fmt.Println("📋 DryRun モード: 実際の更新は行いません")
		}

		fmt.Println()
	}

	execJobs := buildRepoUpdateJobs(root, repoPaths, opts, useTUI)
	summary := runJobsWithOptionalTUI(ctx, "repo update 進捗", jobs, execJobs, useTUI, repoUpdateLogFile)

	// TUI 使用時は TUI 側で完了サマリーを表示済みのため、テキストサマリーは非 TUI 時のみ出力
	if !useTUI {
		printRepoUpdateSummary(summary)
	}

	// 失敗ジョブのエラー詳細を表示
	printFailedJobDetails(summary)

	if summary.Failed > 0 {
		return fmt.Errorf("%d 件のリポジトリ更新に失敗しました", summary.Failed)
	}

	if summary.Skipped > 0 {
		return fmt.Errorf("キャンセルまたはタイムアウトにより %d 件をスキップしました", summary.Skipped)
	}

	if !useTUI {
		fmt.Println("✅ リポジトリ更新が完了しました")
	}

	return nil
}

func loadRepoConfig() (cfg *config.Config, configExists bool, configPath string) {
	configExists, configPath, stateErr := config.ConfigFileExists()
	if stateErr != nil {
		fmt.Fprintf(os.Stderr, "⚠️  設定ファイル状態の確認に失敗: %v\n", stateErr)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  設定ファイルの読み込みに失敗（デフォルト設定を使用）: %v\n", err)

		cfg = config.Default()
	}

	return cfg, configExists, configPath
}

func wrapRepoRootError(err error, root string, rootOverridden, configExists bool, configPath string) error {
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if rootOverridden || configExists {
		return err
	}

	pathNote := ""
	if configPath != "" {
		pathNote = fmt.Sprintf("（設定ファイル: %s）", configPath)
	}

	return fmt.Errorf(
		"repo.root (%s) が見つかりません。設定ファイルが未初期化の可能性があります%s。まず `dsx config init` を実行してください: %w",
		root,
		pathNote,
		err,
	)
}

func buildRepoUpdateOptions(cmd *cobra.Command, cfg *config.Config) (repomgr.UpdateOptions, error) {
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
		return repomgr.UpdateOptions{}, err
	}

	opts.SubmoduleUpdate = submoduleUpdate

	return opts, nil
}

func buildRepoUpdateJobs(root string, repoPaths []string, opts repomgr.UpdateOptions, useTUI bool) []runner.Job {
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
			// 同名衝突時はフルパスで表示して一意性を担保する。
			repoName = filepath.Clean(repoPath)
		}

		execJobs = append(execJobs, runner.Job{
			Name: repoName,
			Run: func(jobCtx context.Context) error {
				updateResult, updateErr := repomgr.Update(jobCtx, repoPath, opts)
				if !useTUI {
					outputMu.Lock()
					printRepoUpdateResult(repoName, updateResult, updateErr)
					outputMu.Unlock()
				}

				return updateErr
			},
		})
	}

	return execJobs
}

func buildRepoJobDisplayName(root, repoPath string) string {
	cleanRepoPath := filepath.Clean(repoPath)

	rel, err := filepath.Rel(root, cleanRepoPath)
	if err != nil {
		return filepath.Base(cleanRepoPath)
	}

	cleanRel := filepath.Clean(rel)
	if cleanRel == "." {
		return cleanRel
	}

	if cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
		return filepath.Base(cleanRepoPath)
	}

	if cleanRel == "" {
		return filepath.Base(cleanRepoPath)
	}

	// Windows でも表示名は GitHub/パス表記に合わせて "/" 区切りに統一する。
	return filepath.ToSlash(cleanRel)
}

func printRepoTable(repos []repomgr.Info) error {
	return writeRepoTable(os.Stdout, repos)
}

func writeRepoTable(output io.Writer, repos []repomgr.Info) error {
	writer := tabwriter.NewWriter(output, 0, 8, 2, ' ', 0)

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
		if result != nil && len(result.SkippedMessages) > 0 {
			fmt.Println("  ⚪ スキップ（pull を実行しませんでした）")
		} else {
			fmt.Println("  ✅ 成功")
		}
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

func printNoTargetResult(root string, bootstrap bootstrapResult, tuiReq tuiRequest) {
	printNoTargetTUIMessage(tuiReq, "repo update")

	if bootstrap.PlannedOnly > 0 {
		fmt.Printf("📝 DryRun のため clone 計画のみ表示しました（%d件）\n", bootstrap.PlannedOnly)
		return
	}

	fmt.Printf("📝 更新対象のリポジトリが見つかりませんでした: %s\n", root)
}

func bootstrapReposFromGitHub(ctx context.Context, root string, cfg *config.Config, dryRun bool) (bootstrapResult, error) {
	owner := strings.TrimSpace(cfg.Repo.GitHub.Owner)
	if owner == "" {
		return bootstrapResult{}, nil
	}

	fmt.Printf("🌐 GitHub からリポジトリ一覧を取得します（owner: %s）\n", owner)

	repos, err := repoListGitHubReposStep(ctx, owner)
	if err != nil {
		if isGitHubRateLimitError(err) {
			fmt.Fprintf(os.Stderr, "⚠️  GitHub のレート制限によりリポジトリ一覧の取得をスキップします: %v\n", err)
			fmt.Fprintln(os.Stderr, "📝 GitHub からの補完は行わず、ローカルに存在するリポジトリのみ更新を継続します。")
			fmt.Println()
			return bootstrapResult{}, nil
		}

		return bootstrapResult{}, err
	}

	if len(repos) == 0 {
		fmt.Printf("📝 GitHub で対象リポジトリが見つかりませんでした: %s\n", owner)
		return bootstrapResult{}, nil
	}

	protocol := strings.TrimSpace(cfg.Repo.GitHub.Protocol)
	result := bootstrapResult{
		ReadyPaths: make([]string, 0, len(repos)),
	}

	for _, repo := range repos {
		outcome, outcomeErr := prepareBootstrapRepo(ctx, root, protocol, repo, dryRun)
		if outcomeErr != nil {
			return bootstrapResult{}, outcomeErr
		}

		accumulateBootstrapResult(&result, outcome)
	}

	result.ReadyPaths = uniqueSortedPaths(result.ReadyPaths)
	if !dryRun && len(result.ReadyPaths) > 0 {
		fmt.Printf("✅ GitHub から %d 件のリポジトリを同期対象に追加しました\n\n", len(result.ReadyPaths))
	}

	return result, nil
}

func prepareBootstrapRepo(ctx context.Context, root, protocol string, repo githubRepo, dryRun bool) (bootstrapRepoOutcome, error) {
	if repo.IsArchived {
		return bootstrapRepoOutcome{}, nil
	}

	targetPath := filepath.Join(root, repo.Name)

	pathStatus, statusErr := inspectRepoPath(targetPath)
	if statusErr != nil {
		return bootstrapRepoOutcome{}, statusErr
	}

	if pathStatus.exists && pathStatus.isGitRepo {
		return bootstrapRepoOutcome{ReadyPath: targetPath}, nil
	}

	if pathStatus.exists {
		return bootstrapRepoOutcome{}, fmt.Errorf("既存パスがGitリポジトリではありません: %s", targetPath)
	}

	cloneURL := selectRepoCloneURL(protocol, repo)
	if cloneURL == "" {
		fmt.Printf("⚠️  clone URL を解決できないためスキップ: %s\n", repo.Name)
		return bootstrapRepoOutcome{}, nil
	}

	fmt.Printf("📥 取得: %s\n", repo.Name)
	fmt.Printf("  $ git clone %s %s\n", cloneURL, targetPath)

	if dryRun {
		return bootstrapRepoOutcome{Planned: true}, nil
	}

	if cloneErr := repoCloneRepoStep(ctx, cloneURL, targetPath); cloneErr != nil {
		return bootstrapRepoOutcome{}, cloneErr
	}

	return bootstrapRepoOutcome{ReadyPath: targetPath}, nil
}

func accumulateBootstrapResult(result *bootstrapResult, outcome bootstrapRepoOutcome) {
	if outcome.ReadyPath != "" {
		result.ReadyPaths = append(result.ReadyPaths, outcome.ReadyPath)
	}

	if outcome.Planned {
		result.PlannedOnly++
	}
}

func listGitHubRepos(ctx context.Context, owner string) ([]githubRepo, error) {
	if _, err := repoLookPathStep("gh"); err != nil {
		return nil, fmt.Errorf("gh コマンドが見つかりません: %w", err)
	}

	output, stderr, err := runGhOutputWithRetry(
		ctx,
		"",
		"repo",
		"list",
		owner,
		"--limit",
		strconv.Itoa(githubRepoListLimit),
		"--json",
		"name,url,sshUrl,isArchived",
	)
	if err != nil {
		if strings.TrimSpace(stderr) != "" {
			return nil, fmt.Errorf("gh repo list の実行に失敗しました (owner=%s): %w: %s", owner, err, strings.TrimSpace(stderr))
		}

		return nil, fmt.Errorf("gh repo list の実行に失敗しました (owner=%s): %w", owner, err)
	}

	repos := []githubRepo{}
	if err := json.Unmarshal(output, &repos); err != nil {
		return nil, fmt.Errorf("GitHub リポジトリ一覧の解析に失敗: %w", err)
	}

	if len(repos) == githubRepoListLimit {
		fmt.Fprintf(
			os.Stderr,
			"⚠️  GitHub 取得件数が上限 (%d件) に達しました。owner=%s の一部リポジトリが同期対象に含まれていない可能性があります。\n",
			githubRepoListLimit,
			owner,
		)
	}

	return repos, nil
}

func cloneRepo(ctx context.Context, cloneURL, targetPath string) error {
	cmd := exec.CommandContext(ctx, "git", "clone", cloneURL, targetPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone に失敗 (%s): %w", cloneURL, err)
	}

	return nil
}

func selectRepoCloneURL(protocol string, repo githubRepo) string {
	if protocol == "ssh" {
		if strings.TrimSpace(repo.SSHURL) != "" {
			return strings.TrimSpace(repo.SSHURL)
		}

		return strings.TrimSpace(repo.URL)
	}

	if strings.TrimSpace(repo.URL) != "" {
		return strings.TrimSpace(repo.URL)
	}

	return strings.TrimSpace(repo.SSHURL)
}

type repoPathStatus struct {
	exists    bool
	isGitRepo bool
}

func inspectRepoPath(path string) (status repoPathStatus, err error) {
	info, statErr := os.Stat(path)
	if statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return repoPathStatus{}, nil
		}

		return repoPathStatus{}, statErr
	}

	if !info.IsDir() {
		return repoPathStatus{exists: true, isGitRepo: false}, nil
	}

	gitPath := filepath.Join(path, ".git")

	gitInfo, gitErr := os.Stat(gitPath)
	if gitErr != nil {
		if errors.Is(gitErr, os.ErrNotExist) {
			return repoPathStatus{exists: true, isGitRepo: false}, nil
		}

		return repoPathStatus{}, gitErr
	}

	if gitInfo.IsDir() {
		return repoPathStatus{exists: true, isGitRepo: true}, nil
	}

	if !gitInfo.Mode().IsRegular() {
		return repoPathStatus{exists: true, isGitRepo: false}, nil
	}

	content, readErr := os.ReadFile(gitPath)
	if readErr != nil {
		return repoPathStatus{}, readErr
	}

	line := strings.TrimSpace(string(content))
	if !strings.HasPrefix(line, "gitdir:") {
		return repoPathStatus{exists: true, isGitRepo: false}, nil
	}

	isGitRepo := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:")) != ""

	return repoPathStatus{exists: true, isGitRepo: isGitRepo}, nil
}

func uniqueSortedPaths(paths []string) []string {
	uniq := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))

	for _, path := range paths {
		clean := filepath.Clean(path)
		if _, exists := uniq[clean]; exists {
			continue
		}

		uniq[clean] = struct{}{}
		out = append(out, clean)
	}

	sort.Strings(out)

	return out
}

func mergeRepoPaths(discoveredPaths, bootstrappedPaths []string) []string {
	merged := append([]string{}, discoveredPaths...)
	merged = append(merged, bootstrappedPaths...)

	return uniqueSortedPaths(merged)
}
