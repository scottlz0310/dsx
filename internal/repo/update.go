package repo

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

const skipPullNoUpstreamMessage = "upstream が未設定のため pull をスキップしました"

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

	// 安全側優先:
	// 破壊的操作（pull/rebase など）につながり得る状態は、理由を表示してスキップする。
	skipMessages, err := detectUnsafeRepoState(ctx, cleanPath)
	if err != nil {
		if opts.DryRun {
			result.SkippedMessages = append(result.SkippedMessages, fmt.Sprintf("リポジトリ状態の判定に失敗したため更新をスキップしました: %v", err))
			return result, nil
		}

		return result, fmt.Errorf("リポジトリ状態の判定に失敗: %w", err)
	}

	if len(skipMessages) > 0 {
		result.SkippedMessages = append(result.SkippedMessages, skipMessages...)
		return result, nil
	}

	if err := inspectUpstream(ctx, cleanPath, opts, result); err != nil {
		return result, err
	}

	if err := planAndRunPull(ctx, cleanPath, opts, result); err != nil {
		return result, err
	}

	if err := planAndRunSubmodule(ctx, cleanPath, opts, result); err != nil {
		return result, err
	}

	return result, nil
}

func inspectUpstream(ctx context.Context, repoPath string, opts UpdateOptions, result *UpdateResult) error {
	upstream, _, err := getAheadCount(ctx, repoPath)
	if err != nil {
		if !opts.DryRun {
			return fmt.Errorf("upstream 確認に失敗: %w", err)
		}

		result.SkippedMessages = append(result.SkippedMessages, fmt.Sprintf("upstream の確認に失敗したため pull の計画をスキップしました: %v", err))

		return nil
	}

	result.UpstreamChecked = true
	result.HasUpstream = upstream

	return nil
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

func detectUnsafeRepoState(ctx context.Context, repoPath string) ([]string, error) {
	messages := make([]string, 0, 3)

	dirty, err := isDirty(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	if dirty {
		messages = append(messages, "未コミットの変更があるため pull/submodule をスキップしました（tracked/untracked を含む）")
	}

	hasStash, err := hasStash(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	if hasStash {
		messages = append(messages, "stash が残っているため pull/submodule をスキップしました（git stash list で確認してください）")
	}

	detached, err := isDetachedHEAD(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	if detached {
		messages = append(messages, "detached HEAD のため pull/submodule をスキップしました（ブランチをチェックアウトしてください）")
	}

	return messages, nil
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
