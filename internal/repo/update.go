package repo

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

const (
	skipPullNoUpstreamMessage                = "upstream が未設定のため pull をスキップしました"
	skipPullNonDefaultUpstreamMessage        = "デフォルトブランチ以外を追跡しているため pull/submodule をスキップしました"
	skipPullUpstreamDetectFailedMessage      = "追跡ブランチの判定に失敗したため pull/submodule をスキップしました"
	skipPullDefaultBranchDetectFailedMessage = "デフォルトブランチの判定に失敗したため pull/submodule をスキップしました"
)

// UpdateOptions は repo update の実行オプションです。
type UpdateOptions struct {
	Prune           bool
	AutoStash       bool
	SubmoduleUpdate bool
	DryRun          bool
}

// UpdateResult は単一リポジトリの更新結果です。
type UpdateResult struct {
	RepoPath        string
	Commands        []string
	SkippedMessages []string
	UpstreamChecked bool
	HasUpstream     bool
}

// Update は単一リポジトリに対して fetch/pull/submodule update を実行します。
func Update(ctx context.Context, repoPath string, opts UpdateOptions) (*UpdateResult, error) {
	cleanPath := filepath.Clean(repoPath)

	result := &UpdateResult{
		RepoPath: cleanPath,
	}

	fetchArgs := buildFetchArgs(opts.Prune)
	result.Commands = append(result.Commands, formatGitCommand(cleanPath, fetchArgs))

	if !opts.DryRun {
		if err := runGitCommand(ctx, cleanPath, fetchArgs...); err != nil {
			return result, fmt.Errorf("fetch に失敗: %w", err)
		}
	}

	// fetch 完了後、安全性チェックと upstream 確認を並列で実行する。
	// upstream 結果は安全性チェックでスキップされなかった場合にのみ使用する。
	type upstreamResult struct {
		checked     bool
		hasUpstream bool
		err         error
	}

	var (
		skipMessages []string
		stateErr     error
		upstream     upstreamResult
		wg           sync.WaitGroup
	)

	wg.Add(2)

	go func() {
		defer wg.Done()

		skipMessages, stateErr = detectUnsafeRepoState(ctx, cleanPath)
	}()

	go func() {
		defer wg.Done()

		up, _, err := getAheadCount(ctx, cleanPath)
		upstream = upstreamResult{checked: err == nil, hasUpstream: up, err: err}
	}()

	wg.Wait()

	// 安全側優先:
	// 破壊的操作（pull/rebase など）につながり得る状態は、理由を表示してスキップする。
	if stateErr != nil {
		if opts.DryRun {
			result.SkippedMessages = append(result.SkippedMessages, fmt.Sprintf("リポジトリ状態の判定に失敗したため更新をスキップしました: %v", stateErr))
			return result, nil
		}

		return result, fmt.Errorf("リポジトリ状態の判定に失敗: %w", stateErr)
	}

	if len(skipMessages) > 0 {
		result.SkippedMessages = append(result.SkippedMessages, skipMessages...)
		return result, nil
	}

	// upstream 結果を result に反映
	if upstream.err != nil && !opts.DryRun {
		return result, fmt.Errorf("upstream 確認に失敗: %w", upstream.err)
	}

	applyUpstreamResult(result, upstream.checked, upstream.hasUpstream, upstream.err, opts)

	if err := planAndRunPull(ctx, cleanPath, opts, result); err != nil {
		return result, err
	}

	if err := planAndRunSubmodule(ctx, cleanPath, opts, result); err != nil {
		return result, err
	}

	return result, nil
}

// applyUpstreamResult は並列実行で取得した upstream 結果を result に反映します。
func applyUpstreamResult(result *UpdateResult, checked, hasUpstream bool, err error, opts UpdateOptions) {
	if err != nil {
		if opts.DryRun {
			result.SkippedMessages = append(result.SkippedMessages, fmt.Sprintf("upstream の確認に失敗したため pull の計画をスキップしました: %v", err))
		}

		return
	}

	result.UpstreamChecked = checked
	result.HasUpstream = hasUpstream
}

func planAndRunPull(ctx context.Context, repoPath string, opts UpdateOptions, result *UpdateResult) error {
	pullArgs := buildPullArgs(opts.AutoStash)

	switch {
	case opts.DryRun && result.UpstreamChecked && result.HasUpstream:
		result.Commands = append(result.Commands, formatGitCommand(repoPath, pullArgs))
	case opts.DryRun && result.UpstreamChecked && !result.HasUpstream:
		result.SkippedMessages = append(result.SkippedMessages, skipPullNoUpstreamMessage)
	case !opts.DryRun && result.HasUpstream:
		result.Commands = append(result.Commands, formatGitCommand(repoPath, pullArgs))
		if err := runGitCommand(ctx, repoPath, pullArgs...); err != nil {
			return fmt.Errorf("pull に失敗: %w", err)
		}
	case !opts.DryRun:
		result.SkippedMessages = append(result.SkippedMessages, skipPullNoUpstreamMessage)
	}

	return nil
}

func planAndRunSubmodule(ctx context.Context, repoPath string, opts UpdateOptions, result *UpdateResult) error {
	if !opts.SubmoduleUpdate {
		return nil
	}

	submoduleArgs := buildSubmoduleArgs()
	result.Commands = append(result.Commands, formatGitCommand(repoPath, submoduleArgs))

	if opts.DryRun {
		return nil
	}

	if err := runGitCommand(ctx, repoPath, submoduleArgs...); err != nil {
		return fmt.Errorf("submodule update に失敗: %w", err)
	}

	return nil
}

func buildFetchArgs(prune bool) []string {
	args := []string{"fetch", "--all"}
	if prune {
		args = append(args, "--prune")
	}

	return args
}

func buildPullArgs(autoStash bool) []string {
	args := []string{"pull", "--rebase"}
	if autoStash {
		args = append(args, "--autostash")
	}

	return args
}

func buildSubmoduleArgs() []string {
	return []string{"submodule", "update", "--init", "--recursive", "--remote"}
}

func formatGitCommand(repoPath string, args []string) string {
	parts := append([]string{"git", "-C", repoPath}, args...)
	return strings.Join(parts, " ")
}

func runGitCommand(ctx context.Context, repoPath string, args ...string) error {
	commandArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.CommandContext(ctx, "git", commandArgs...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return err
		}

		return fmt.Errorf("%w: %s", err, message)
	}

	return nil
}

// repoStateCheckResult は並列で実行する安全性チェックの結果を保持します。
type repoStateCheckResult struct {
	dirty    bool
	hasStash bool
	detached bool
}

func detectUnsafeRepoState(ctx context.Context, repoPath string) ([]string, error) {
	// isDirty / hasStash / isDetachedHEAD は互いに独立なので並列実行する。
	result, err := runParallelStateChecks(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	messages := buildUnsafeMessages(ctx, repoPath, result)

	return messages, nil
}

// runParallelStateChecks は isDirty, hasStash, isDetachedHEAD を並列実行します。
func runParallelStateChecks(ctx context.Context, repoPath string) (repoStateCheckResult, error) {
	var (
		result repoStateCheckResult
		mu     sync.Mutex
		errs   []error
		wg     sync.WaitGroup
	)

	wg.Add(3)

	go func() {
		defer wg.Done()

		dirty, err := isDirty(ctx, repoPath)

		mu.Lock()
		defer mu.Unlock()

		if err != nil {
			errs = append(errs, err)
			return
		}

		result.dirty = dirty
	}()

	go func() {
		defer wg.Done()

		stash, err := hasStash(ctx, repoPath)

		mu.Lock()
		defer mu.Unlock()

		if err != nil {
			errs = append(errs, err)
			return
		}

		result.hasStash = stash
	}()

	go func() {
		defer wg.Done()

		detached, err := isDetachedHEAD(ctx, repoPath)

		mu.Lock()
		defer mu.Unlock()

		if err != nil {
			errs = append(errs, err)
			return
		}

		result.detached = detached
	}()

	wg.Wait()

	if len(errs) > 0 {
		return repoStateCheckResult{}, errors.Join(errs...)
	}

	return result, nil
}

// buildUnsafeMessages はチェック結果からスキップメッセージを構築します。
func buildUnsafeMessages(ctx context.Context, repoPath string, result repoStateCheckResult) []string {
	messages := make([]string, 0, 3)

	if result.dirty {
		messages = append(messages, "未コミットの変更があるため pull/submodule をスキップしました（tracked/untracked を含む）")
	}

	if result.hasStash {
		messages = append(messages, "stash が残っているため pull/submodule をスキップしました（git stash list で確認してください）")
	}

	if result.detached {
		messages = append(messages, "detached HEAD のため pull/submodule をスキップしました（ブランチをチェックアウトしてください）")
	}

	if !result.detached {
		if msg := detectNonDefaultTrackingBranch(ctx, repoPath); msg != "" {
			messages = append(messages, msg)
		}
	}

	return messages
}

func detectNonDefaultTrackingBranch(ctx context.Context, repoPath string) string {
	upstreamRef, hasUpstream, err := getUpstreamRef(ctx, repoPath)
	if err != nil {
		return fmt.Sprintf("%s: %v", skipPullUpstreamDetectFailedMessage, err)
	}

	if !hasUpstream {
		return ""
	}

	remote, _, ok := strings.Cut(upstreamRef, "/")
	if !ok || strings.TrimSpace(remote) == "" {
		return fmt.Sprintf("%s: upstream の参照 %q が `<remote>/<branch>` 形式ではありません", skipPullUpstreamDetectFailedMessage, upstreamRef)
	}

	defaultRef, err := getRemoteDefaultRef(ctx, repoPath, remote)
	if err != nil {
		return fmt.Sprintf("%s: %v。`refs/remotes/%s/HEAD` が存在しないか壊れている可能性があります。`git remote set-head %s -a` または `git fetch %s` を実行してから再実行してください。", skipPullDefaultBranchDetectFailedMessage, err, remote, remote, remote)
	}

	if defaultRef == upstreamRef {
		return ""
	}

	branch := "<unknown>"
	if current, branchErr := getCurrentBranchName(ctx, repoPath); branchErr == nil && current != "" {
		branch = current
	}

	return fmt.Sprintf("%s（現在: %s, 追跡: %s, デフォルト: %s）。デフォルトブランチをチェックアウトして再実行してください。", skipPullNonDefaultUpstreamMessage, branch, upstreamRef, defaultRef)
}

func getUpstreamRef(ctx context.Context, repoPath string) (upstreamRef string, hasUpstream bool, err error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")

	output, err := cmd.Output()
	if err != nil {
		if isNoUpstreamError(err) {
			return "", false, nil
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if stderr != "" {
				return "", false, fmt.Errorf("git rev-parse --abbrev-ref --symbolic-full-name @{u} に失敗しました: %w: %s", err, stderr)
			}
		}

		return "", false, fmt.Errorf("git rev-parse --abbrev-ref --symbolic-full-name @{u} に失敗しました: %w", err)
	}

	ref := strings.TrimSpace(string(output))
	if ref == "" {
		return "", false, fmt.Errorf("upstream の取得に失敗しました（出力が空です）")
	}

	return ref, true, nil
}

func getRemoteDefaultRef(ctx context.Context, repoPath, remote string) (defaultRef string, err error) {
	refName := fmt.Sprintf("refs/remotes/%s/HEAD", remote)

	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "symbolic-ref", "--quiet", refName)

	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if stderr != "" {
				return "", fmt.Errorf("git symbolic-ref --quiet %s に失敗しました: %w: %s", refName, err, stderr)
			}
		}

		return "", fmt.Errorf("git symbolic-ref --quiet %s に失敗しました: %w", refName, err)
	}

	ref := strings.TrimPrefix(strings.TrimSpace(string(output)), "refs/remotes/")
	if ref == "" {
		return "", fmt.Errorf("リモートのデフォルトブランチ取得に失敗しました（出力が空です）")
	}

	return ref, nil
}

func getCurrentBranchName(ctx context.Context, repoPath string) (string, error) {
	output, err := runGitCommandOutput(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

func hasStash(ctx context.Context, repoPath string) (bool, error) {
	output, err := runGitCommandOutput(ctx, repoPath, "stash", "list")
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(string(output)) != "", nil
}

func isDetachedHEAD(ctx context.Context, repoPath string) (bool, error) {
	output, err := runGitCommandOutput(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(string(output)) == "HEAD", nil
}

func runGitCommandOutput(ctx context.Context, repoPath string, args ...string) ([]byte, error) {
	commandArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.CommandContext(ctx, "git", commandArgs...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			return nil, err
		}

		return nil, fmt.Errorf("%w: %s", err, message)
	}

	return output, nil
}
