package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	survey "github.com/AlecAivazis/survey/v2"
	repomgr "github.com/scottlz0310/dsx/internal/repo"
	"github.com/spf13/cobra"
)

var (
	repoBranchCleanDryRun  bool
	repoBranchCleanYes     bool
	repoBranchCleanNoFetch bool
	repoBranchCleanExclude []string
)

var repoBranchCleanCmd = &cobra.Command{
	Use:   "branch-clean",
	Short: "不要なブランチを検出してクリーンアップします",
	Long: `全リポジトリのブランチを4つのカテゴリで検出してクリーンアップします。

カテゴリ:
  [MERGED]      デフォルトブランチにマージ済みのローカルブランチ（安全削除）
  [UNMERGED]    未マージのローカルブランチ（最終コミット日時付き）
  [STALE-REF]   リモートに存在しないリモートトラッキング参照（prune で除去）
  [NO-UPSTREAM] アップストリームが未設定のローカルブランチ

実行モード:
  デフォルト  インタラクティブに削除するブランチを選択します
  --dry-run   候補を表示するのみで実際の操作は行いません
  --yes       MERGED と STALE-REF を自動削除します（UNMERGED・NO-UPSTREAM は表示のみ）`,
	RunE: runRepoBranchClean,
}

func init() {
	repoCmd.AddCommand(repoBranchCleanCmd)

	repoBranchCleanCmd.Flags().StringVar(&repoRootOverride, "root", "", "対象のルートディレクトリ（指定時は設定を上書き）")
	repoBranchCleanCmd.Flags().BoolVarP(&repoBranchCleanDryRun, "dry-run", "n", false, "候補を表示するのみ（実際の操作は行わない）")
	repoBranchCleanCmd.Flags().BoolVarP(&repoBranchCleanYes, "yes", "y", false, "安全なブランチ（MERGED・STALE-REF）を自動削除")
	repoBranchCleanCmd.Flags().BoolVar(&repoBranchCleanNoFetch, "no-fetch", false, "スキャン前の git fetch をスキップ")
	repoBranchCleanCmd.Flags().StringArrayVar(&repoBranchCleanExclude, "exclude", nil, "除外するブランチ名（複数指定可）")
}

func runRepoBranchClean(cmd *cobra.Command, _ []string) error {
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

	if len(repoPaths) == 0 {
		fmt.Printf("📝 対象のリポジトリが見つかりませんでした: %s\n", root)

		return nil
	}

	scanOpts := repomgr.BranchScanOptions{
		Fetch:           !repoBranchCleanNoFetch,
		ExcludeBranches: repoBranchCleanExclude,
	}

	modeLabel := "インタラクティブモード"
	if repoBranchCleanDryRun {
		modeLabel = "ドライランモード"
	} else if repoBranchCleanYes {
		modeLabel = "自動実行モード"
	}

	fmt.Printf("🔍 ブランチスキャン開始 (%s, リポジトリ数: %d)\n\n", modeLabel, len(repoPaths))

	totalDeleted := 0
	totalPruned := 0
	totalErrors := 0

	for _, repoPath := range repoPaths {
		displayName := buildRepoJobDisplayName(repoPath, root)
		deleted, pruned, errors := processRepoBranchClean(ctx, repoPath, displayName, scanOpts)

		totalDeleted += deleted
		totalPruned += pruned
		totalErrors += errors
	}

	printSummary(totalDeleted, totalPruned, totalErrors, repoBranchCleanDryRun)

	return nil
}

// processRepoBranchClean は単一リポジトリのブランチクリーンアップを実行し、削除・プルーン・エラーの件数を返します。
func processRepoBranchClean(ctx context.Context, repoPath, displayName string, scanOpts repomgr.BranchScanOptions) (deleted, pruned, errors int) {
	result, scanErr := repomgr.ScanBranches(ctx, repoPath, scanOpts)
	if scanErr != nil {
		fmt.Fprintf(os.Stderr, "  ⚠️  %s: スキャン失敗 (%v)\n", displayName, scanErr)

		return 0, 0, 1
	}

	if len(result.Candidates) == 0 {
		fmt.Printf("  ✅ %s: クリーンアップ不要\n", displayName)

		return 0, 0, 0
	}

	fmt.Printf("  📁 %s (ブランチ: %s)\n", displayName, result.DefaultBranch)
	printRepoCandidates(result)

	if repoBranchCleanDryRun {
		return 0, 0, 0
	}

	toDelete, warnCount := selectBranchesToClean(result, displayName)

	errors += warnCount

	if len(toDelete) == 0 {
		fmt.Printf("  ⏭️  %s: 選択なし（スキップ）\n\n", displayName)

		return 0, 0, errors
	}

	cleanResult, cleanErr := repomgr.DeleteBranchCandidates(ctx, repoPath, toDelete, false)
	if cleanResult != nil {
		for _, errItem := range cleanResult.Errors {
			fmt.Fprintf(os.Stderr, "  ⚠️  %s: %v\n", displayName, errItem)
		}

		printCleanResult(displayName, cleanResult)

		return len(cleanResult.Deleted), len(cleanResult.Pruned), errors + len(cleanResult.Errors)
	}

	if cleanErr != nil {
		fmt.Fprintf(os.Stderr, "  ❌ %s: クリーンアップ失敗 (%v)\n", displayName, cleanErr)

		return 0, 0, errors + 1
	}

	return 0, 0, errors
}

// printRepoCandidates は単一リポジトリのブランチ候補一覧を表示します。
func printRepoCandidates(result *repomgr.BranchScanResult) {
	for _, c := range result.Candidates {
		label := repomgr.CategoryLabel(c.Category)

		age := ""
		if c.Age != "" {
			age = fmt.Sprintf("  (%s)", c.Age)
		}

		autoMark := ""
		if repomgr.IsSafeToAutoDelete(c.Category) {
			autoMark = " ✔"
		}

		fmt.Printf("    %s %s%s%s\n", label, c.Name, age, autoMark)
	}

	fmt.Println()
}

// selectBranchesToClean はモードに応じてクリーンアップ対象を選択します。
// 返り値 warnCount は --yes モードで自動削除対象外と判断された件数です（実行失敗ではなくスキップ警告）。
func selectBranchesToClean(result *repomgr.BranchScanResult, displayName string) (toDelete []repomgr.BranchCandidate, warnCount int) {
	if repoBranchCleanYes {
		return collectAutoTargets(result.Candidates, displayName)
	}

	selected, interactiveErr := askBranchSelection(result)
	if interactiveErr != nil {
		fmt.Fprintf(os.Stderr, "  ⚠️  %s: インタラクティブ選択失敗 (%v)\n", displayName, interactiveErr)

		return nil, 1
	}

	return selected, 0
}

// collectAutoTargets は --yes モードで自動削除対象を収集し、安全でない候補については警告します。
func collectAutoTargets(candidates []repomgr.BranchCandidate, displayName string) (toDelete []repomgr.BranchCandidate, warnCount int) {
	for _, c := range candidates {
		if repomgr.IsSafeToAutoDelete(c.Category) {
			toDelete = append(toDelete, c)

			continue
		}

		label := repomgr.CategoryLabel(c.Category)

		age := ""
		if c.Age != "" {
			age = fmt.Sprintf(" (%s)", c.Age)
		}

		fmt.Printf("  ⚠️  %s: %s %s%s は自動削除対象外です（手動で確認してください）\n",
			displayName, label, c.Name, age)

		warnCount++
	}

	return toDelete, warnCount
}

// askBranchSelection はインタラクティブモードでブランチ選択を行います。
// MERGED と STALE_REF をデフォルトで選択済みにします。
func askBranchSelection(result *repomgr.BranchScanResult) ([]repomgr.BranchCandidate, error) {
	type option struct {
		label     string
		candidate repomgr.BranchCandidate
	}

	options := make([]option, 0, len(result.Candidates))

	var defaultSelected []string

	for _, c := range result.Candidates {
		label := buildOptionLabel(c)

		options = append(options, option{label: label, candidate: c})

		if repomgr.IsSafeToAutoDelete(c.Category) {
			defaultSelected = append(defaultSelected, label)
		}
	}

	optionLabels := make([]string, len(options))
	labelToCandidate := make(map[string]repomgr.BranchCandidate, len(options))

	for i, o := range options {
		optionLabels[i] = o.label
		labelToCandidate[o.label] = o.candidate
	}

	var selectedLabels []string

	prompt := &survey.MultiSelect{
		Message: "削除するブランチを選択してください（スペースで選択、Enterで確定）",
		Options: optionLabels,
		Default: defaultSelected,
	}

	if err := survey.AskOne(prompt, &selectedLabels); err != nil {
		return nil, err
	}

	selected := make([]repomgr.BranchCandidate, 0, len(selectedLabels))

	for _, label := range selectedLabels {
		if c, ok := labelToCandidate[label]; ok {
			selected = append(selected, c)
		}
	}

	return selected, nil
}

// buildOptionLabel はブランチ候補の選択肢ラベル文字列を構築します。
func buildOptionLabel(c repomgr.BranchCandidate) string {
	parts := []string{
		repomgr.CategoryLabel(c.Category),
		c.Name,
	}

	if c.Age != "" {
		parts = append(parts, fmt.Sprintf("(%s)", c.Age))
	}

	return strings.Join(parts, " ")
}

// printCleanResult はクリーンアップ結果を表示します。
func printCleanResult(displayName string, result *repomgr.BranchCleanResult) {
	if len(result.Deleted) > 0 {
		names := make([]string, len(result.Deleted))

		for i, c := range result.Deleted {
			names[i] = c.Name
		}

		fmt.Printf("  🗑️  %s: ブランチ削除: %s\n", displayName, strings.Join(names, ", "))
	}

	if len(result.Pruned) > 0 {
		names := make([]string, len(result.Pruned))

		for i, c := range result.Pruned {
			names[i] = c.Name
		}

		fmt.Printf("  ✂️  %s: プルーン完了: %s\n", displayName, strings.Join(names, ", "))
	}

	fmt.Println()
}

// printSummary は全リポジトリ処理後のサマリーを表示します。
func printSummary(deleted, pruned, errors int, dryRun bool) {
	fmt.Println("─────────────────────────────────────────")

	if dryRun {
		fmt.Println("📋 ドライラン完了（実際の操作は行いませんでした）")

		return
	}

	fmt.Printf("✅ クリーンアップ完了: ブランチ削除 %d件, プルーン %d件", deleted, pruned)

	if errors > 0 {
		fmt.Printf(", エラー %d件", errors)
	}

	fmt.Println()
}
