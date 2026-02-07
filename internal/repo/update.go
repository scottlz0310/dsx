package repo

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
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
		result.SkippedMessages = append(result.SkippedMessages, "upstream が未設定のため pull の計画をスキップしました")
	case !opts.DryRun && result.HasUpstream:
		result.Commands = append(result.Commands, formatGitCommand(repoPath, pullArgs))
		if err := runGitCommand(ctx, repoPath, pullArgs...); err != nil {
			return fmt.Errorf("pull に失敗: %w", err)
		}
	case !opts.DryRun:
		result.SkippedMessages = append(result.SkippedMessages, "upstream が未設定のため pull をスキップしました")
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
