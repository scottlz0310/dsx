package repo

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	cleanupTargetMerged   = "merged"
	cleanupTargetSquashed = "squashed"

	// defaultRemoteName は標準的なリモート名 "origin" を表す共通定数です。
	defaultRemoteName = "origin"
)

// DefaultBranchInfo はリポジトリのデフォルトブランチ情報です。
// Ref は "<remote>/<branch>" 形式（例: "origin/main"）を想定します。
type DefaultBranchInfo struct {
	Remote string
	Ref    string
	Branch string
}

// DetectDefaultBranch はリモートのデフォルトブランチ（refs/remotes/<remote>/HEAD）を元に情報を返します。
// upstream が設定されている場合はその remote を優先し、なければ origin を優先します。
func DetectDefaultBranch(ctx context.Context, repoPath string) (DefaultBranchInfo, error) {
	remote, err := detectCleanupRemote(ctx, repoPath)
	if err != nil {
		return DefaultBranchInfo{}, err
	}

	ref, err := getRemoteDefaultRef(ctx, repoPath, remote)
	if err != nil {
		return DefaultBranchInfo{}, err
	}

	_, branch, ok := strings.Cut(ref, "/")
	if !ok || strings.TrimSpace(branch) == "" {
		return DefaultBranchInfo{}, fmt.Errorf("リモートのデフォルトブランチ参照 %q が `<remote>/<branch>` 形式ではありません", ref)
	}

	return DefaultBranchInfo{
		Remote: remote,
		Ref:    ref,
		Branch: branch,
	}, nil
}

func detectCleanupRemote(ctx context.Context, repoPath string) (string, error) {
	remotes, err := listRemotes(ctx, repoPath)
	if err != nil {
		return "", err
	}

	upstreamRef, hasUpstream, err := getUpstreamRef(ctx, repoPath)
	if err != nil {
		return "", err
	}

	if hasUpstream {
		remote, _, ok := strings.Cut(upstreamRef, "/")
		if ok && strings.TrimSpace(remote) != "" {
			return remote, nil
		}
	}

	if containsString(remotes, defaultRemoteName) {
		return defaultRemoteName, nil
	}

	if len(remotes) == 1 {
		return remotes[0], nil
	}

	if len(remotes) == 0 {
		return "", fmt.Errorf("リモートが設定されていません")
	}

	return "", fmt.Errorf("リモートが複数あるため特定できません: %v", remotes)
}

func listRemotes(ctx context.Context, repoPath string) ([]string, error) {
	output, err := runGitCommandOutput(ctx, repoPath, "remote")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	result := make([]string, 0, len(lines))
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}

		result = append(result, name)
	}

	sort.Strings(result)

	return result, nil
}

func containsString(values []string, needle string) bool {
	for _, v := range values {
		if v == needle {
			return true
		}
	}

	return false
}

// CleanupPlan は削除対象ブランチと理由（target）を表します。
type CleanupPlan struct {
	Branch string
	Target string
	Force  bool
}

// CleanupOptions は repo cleanup の実行オプションです。
type CleanupOptions struct {
	Prune                  bool
	DryRun                 bool
	Targets                []string
	ExcludeBranches        []string
	SquashedPRHeadByBranch map[string]string
}

// CleanupResult は単一リポジトリの cleanup 結果です。
type CleanupResult struct {
	RepoPath        string
	Remote          string
	DefaultRef      string
	DefaultBranch   string
	Commands        []string
	PlannedDeletes  []CleanupPlan
	DeletedBranches []CleanupPlan
	SkippedMessages []string
	Errors          []error
}

// Cleanup は単一リポジトリのマージ済みローカルブランチを削除します。
// merged: git でマージ済みと判定できるブランチを削除（git branch -d）
// squashed: GitHub の PR 情報に基づき「PR は merged だが git 的には未マージ」なブランチを削除（git branch -D）
func Cleanup(ctx context.Context, repoPath string, opts CleanupOptions) (*CleanupResult, error) {
	cleanPath := filepath.Clean(repoPath)

	result := &CleanupResult{RepoPath: cleanPath}

	if err := planAndRunCleanupFetch(ctx, result, cleanPath, opts); err != nil {
		return result, err
	}

	skipped, err := skipCleanupOnUnsafeRepoState(ctx, result, cleanPath, opts.DryRun)
	if err != nil {
		return result, err
	}

	if skipped {
		return result, nil
	}

	defaultInfo, ok := resolveCleanupDefaultBranch(ctx, result, cleanPath)
	if !ok {
		return result, nil
	}

	doMerged, doSquashed, ok := validateCleanupTargets(result, opts.Targets)
	if !ok {
		return result, nil
	}

	currentBranch, err := getCurrentBranchName(ctx, cleanPath)
	if err != nil {
		return result, fmt.Errorf("現在のブランチ取得に失敗: %w", err)
	}

	excluded := buildExcludedBranchSet(currentBranch, defaultInfo.Branch, opts.ExcludeBranches)

	plans, err := buildCleanupPlans(ctx, cleanPath, defaultInfo.Ref, excluded, doMerged, doSquashed, opts.SquashedPRHeadByBranch)
	if err != nil {
		return result, err
	}

	if len(plans) == 0 {
		result.SkippedMessages = append(result.SkippedMessages, "削除対象のブランチがありません")
		return result, nil
	}

	if err := executeCleanupPlans(ctx, result, cleanPath, plans, opts.DryRun); err != nil {
		return result, err
	}

	return result, nil
}

func planAndRunCleanupFetch(ctx context.Context, result *CleanupResult, repoPath string, opts CleanupOptions) error {
	// DryRun でも参照を最新化するために fetch は常に実行する。
	// ただし DryRun 時は prune を無効化して、リモート参照の削除は行わない。
	prune := opts.Prune
	if opts.DryRun && prune {
		prune = false
	}

	fetchArgs := buildFetchArgs(prune)
	result.Commands = append(result.Commands, formatGitCommand(repoPath, fetchArgs))

	if err := runGitCommand(ctx, repoPath, fetchArgs...); err != nil {
		return fmt.Errorf("fetch に失敗: %w", err)
	}

	return nil
}

func skipCleanupOnUnsafeRepoState(ctx context.Context, result *CleanupResult, repoPath string, dryRun bool) (bool, error) {
	skipMessages, err := detectUnsafeCleanupRepoState(ctx, repoPath)
	if err != nil {
		if dryRun {
			result.SkippedMessages = append(result.SkippedMessages, fmt.Sprintf("リポジトリ状態の判定に失敗したため cleanup をスキップしました: %v", err))
			return true, nil
		}

		return false, fmt.Errorf("リポジトリ状態の判定に失敗: %w", err)
	}

	if len(skipMessages) == 0 {
		return false, nil
	}

	result.SkippedMessages = append(result.SkippedMessages, skipMessages...)

	return true, nil
}

func resolveCleanupDefaultBranch(ctx context.Context, result *CleanupResult, repoPath string) (DefaultBranchInfo, bool) {
	defaultInfo, err := DetectDefaultBranch(ctx, repoPath)
	if err != nil {
		result.SkippedMessages = append(result.SkippedMessages, fmt.Sprintf("デフォルトブランチの判定に失敗したため cleanup をスキップしました: %v", err))
		return DefaultBranchInfo{}, false
	}

	result.Remote = defaultInfo.Remote
	result.DefaultRef = defaultInfo.Ref
	result.DefaultBranch = defaultInfo.Branch

	return defaultInfo, true
}

func validateCleanupTargets(result *CleanupResult, targets []string) (doMerged, doSquashed, ok bool) {
	if len(targets) == 0 {
		result.SkippedMessages = append(result.SkippedMessages, "repo.cleanup.target が空のため cleanup をスキップしました")
		return false, false, false
	}

	doMerged, doSquashed = resolveCleanupTargets(targets)
	if !doMerged && !doSquashed {
		result.SkippedMessages = append(result.SkippedMessages, fmt.Sprintf("repo.cleanup.target が不正なため cleanup をスキップしました: %v", targets))
		return false, false, false
	}

	return doMerged, doSquashed, true
}

func resolveCleanupTargets(targets []string) (merged, squashed bool) {
	for _, target := range targets {
		switch strings.ToLower(strings.TrimSpace(target)) {
		case cleanupTargetMerged:
			merged = true
		case cleanupTargetSquashed:
			squashed = true
		}
	}

	return merged, squashed
}

func buildCleanupPlans(ctx context.Context, repoPath, defaultRef string, excluded map[string]struct{}, doMerged, doSquashed bool, squashedHeads map[string]string) ([]CleanupPlan, error) {
	plannedSet := make(map[string]CleanupPlan)

	if doMerged {
		if err := addMergedCleanupPlans(ctx, plannedSet, repoPath, defaultRef, excluded); err != nil {
			return nil, err
		}
	}

	if doSquashed && len(squashedHeads) > 0 {
		if err := addSquashedCleanupPlans(ctx, plannedSet, repoPath, excluded, squashedHeads); err != nil {
			return nil, err
		}
	}

	plans := make([]CleanupPlan, 0, len(plannedSet))
	for _, plan := range plannedSet {
		plans = append(plans, plan)
	}

	sort.Slice(plans, func(i, j int) bool {
		return plans[i].Branch < plans[j].Branch
	})

	return plans, nil
}

func addMergedCleanupPlans(ctx context.Context, plannedSet map[string]CleanupPlan, repoPath, defaultRef string, excluded map[string]struct{}) error {
	mergedBranches, err := listMergedLocalBranches(ctx, repoPath, defaultRef)
	if err != nil {
		return fmt.Errorf("マージ済みブランチ一覧の取得に失敗: %w", err)
	}

	for _, branch := range mergedBranches {
		if isExcludedBranch(excluded, branch) {
			continue
		}

		plannedSet[branch] = CleanupPlan{
			Branch: branch,
			Target: cleanupTargetMerged,
			Force:  false,
		}
	}

	return nil
}

func addSquashedCleanupPlans(ctx context.Context, plannedSet map[string]CleanupPlan, repoPath string, excluded map[string]struct{}, squashedHeads map[string]string) error {
	for branch, head := range squashedHeads {
		branch = strings.TrimSpace(branch)
		head = strings.TrimSpace(head)

		if branch == "" || head == "" {
			continue
		}

		if isExcludedBranch(excluded, branch) {
			continue
		}

		if _, already := plannedSet[branch]; already {
			continue
		}

		exists, err := localBranchExists(ctx, repoPath, branch)
		if err != nil {
			return fmt.Errorf("ローカルブランチ存在確認に失敗: %w", err)
		}

		if !exists {
			continue
		}

		tip, err := getBranchTip(ctx, repoPath, branch)
		if err != nil {
			return fmt.Errorf("%s の先頭コミット取得に失敗: %w", branch, err)
		}

		if tip != head {
			continue
		}

		plannedSet[branch] = CleanupPlan{
			Branch: branch,
			Target: cleanupTargetSquashed,
			Force:  true,
		}
	}

	return nil
}

func executeCleanupPlans(ctx context.Context, result *CleanupResult, repoPath string, plans []CleanupPlan, dryRun bool) error {
	for _, plan := range plans {
		args := []string{"branch", "-d", plan.Branch}
		if plan.Force {
			args = []string{"branch", "-D", plan.Branch}
		}

		result.Commands = append(result.Commands, formatGitCommand(repoPath, args))

		if dryRun {
			result.PlannedDeletes = append(result.PlannedDeletes, plan)
			continue
		}

		if err := runGitCommand(ctx, repoPath, args...); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("%s の削除に失敗: %w", plan.Branch, err))
			continue
		}

		result.DeletedBranches = append(result.DeletedBranches, plan)
	}

	if len(result.Errors) == 0 {
		return nil
	}

	return fmt.Errorf("%d 件のブランチ削除に失敗しました", len(result.Errors))
}

func buildExcludedBranchSet(currentBranch, defaultBranch string, excludeBranches []string) map[string]struct{} {
	set := make(map[string]struct{}, len(excludeBranches)+2)

	if strings.TrimSpace(defaultBranch) != "" {
		set[defaultBranch] = struct{}{}
	}

	if strings.TrimSpace(currentBranch) != "" && currentBranch != "HEAD" {
		set[currentBranch] = struct{}{}
	}

	for _, branch := range excludeBranches {
		branch = strings.TrimSpace(branch)
		if branch == "" {
			continue
		}

		set[branch] = struct{}{}
	}

	return set
}

func isExcludedBranch(excluded map[string]struct{}, branch string) bool {
	_, ok := excluded[branch]
	return ok
}

func detectUnsafeCleanupRepoState(ctx context.Context, repoPath string) ([]string, error) {
	messages := make([]string, 0, 3)

	dirty, err := isDirty(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	if dirty {
		messages = append(messages, "未コミットの変更があるため cleanup をスキップしました（tracked/untracked を含む）")
	}

	hasStash, err := hasStash(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	if hasStash {
		messages = append(messages, "stash が残っているため cleanup をスキップしました（git stash list で確認してください）")
	}

	detached, err := isDetachedHEAD(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	if detached {
		messages = append(messages, "detached HEAD のため cleanup をスキップしました（ブランチをチェックアウトしてください）")
	}

	return messages, nil
}

func listMergedLocalBranches(ctx context.Context, repoPath, baseRef string) ([]string, error) {
	if strings.TrimSpace(baseRef) == "" {
		return nil, fmt.Errorf("baseRef が空です")
	}

	output, err := runGitCommandOutput(ctx, repoPath, "for-each-ref", "--format=%(refname:short)", "--merged="+baseRef, "refs/heads")
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	result := make([]string, 0, len(lines))
	for _, line := range lines {
		branch := strings.TrimSpace(line)
		if branch == "" {
			continue
		}

		result = append(result, branch)
	}

	return result, nil
}

func localBranchExists(ctx context.Context, repoPath, branch string) (bool, error) {
	ref := fmt.Sprintf("refs/heads/%s", branch)

	_, err := runGitCommandOutput(ctx, repoPath, "show-ref", "--verify", "--quiet", ref)
	if err == nil {
		return true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}

	return false, err
}

func getBranchTip(ctx context.Context, repoPath, branch string) (string, error) {
	output, err := runGitCommandOutput(ctx, repoPath, "rev-parse", branch)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}
