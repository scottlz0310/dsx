package repo

import (
	"context"
	"fmt"
	"path/filepath"
)

// BranchCleanResult は単一リポジトリのブランチクリーンアップ実行結果です。
type BranchCleanResult struct {
	RepoPath string
	// Deleted は削除が完了したローカルブランチ（dryRun 時は削除予定のブランチ）
	Deleted []BranchCandidate
	// Pruned は prune が完了したリモートトラッキング参照（dryRun 時は prune 予定の参照）
	Pruned []BranchCandidate
	// Errors は各操作で発生したエラーのリスト
	Errors []error
}

// DeleteBranchCandidates は選択されたブランチ候補を削除・プルーンします。
//
// 削除フラグ:
//   - MERGED:       git branch -d（安全削除）
//   - UNMERGED:     git branch -D（強制削除）
//   - NO_UPSTREAM:  git branch -D（強制削除）
//   - STALE_REF:    git remote prune <remote>（リモートごとに1回まとめて実行）
//
// dryRun が true の場合は実際の操作は行わず、結果に候補を記録するのみです。
func DeleteBranchCandidates(ctx context.Context, repoPath string, candidates []BranchCandidate, dryRun bool) (*BranchCleanResult, error) {
	cleanPath := filepath.Clean(repoPath)
	result := &BranchCleanResult{RepoPath: cleanPath}

	staleByRemote, localBranches := separateBranchCandidates(candidates)

	for _, c := range localBranches {
		deleteLocalBranch(ctx, cleanPath, c, dryRun, result)
	}

	for remote, refs := range staleByRemote {
		pruneRemoteRefs(ctx, cleanPath, remote, refs, dryRun, result)
	}

	if len(result.Errors) > 0 {
		return result, fmt.Errorf("%d 件の操作に失敗しました", len(result.Errors))
	}

	return result, nil
}

// separateBranchCandidates は候補をローカルブランチとリモートごとのスタレ参照に分離します。
func separateBranchCandidates(candidates []BranchCandidate) (staleByRemote map[string][]BranchCandidate, localBranches []BranchCandidate) {
	staleByRemote = make(map[string][]BranchCandidate)

	for _, c := range candidates {
		if c.Category != BranchCategoryStaleRef {
			localBranches = append(localBranches, c)

			continue
		}

		remote := c.Remote
		if remote == "" {
			remote = "origin"
		}

		staleByRemote[remote] = append(staleByRemote[remote], c)
	}

	return staleByRemote, localBranches
}

// deleteLocalBranch は1つのローカルブランチを削除します。
func deleteLocalBranch(ctx context.Context, cleanPath string, c BranchCandidate, dryRun bool, result *BranchCleanResult) {
	flag := "-d"
	if c.Category == BranchCategoryUnmerged || c.Category == BranchCategoryNoUpstream {
		flag = "-D"
	}

	if dryRun {
		result.Deleted = append(result.Deleted, c)

		return
	}

	if err := runGitCommand(ctx, cleanPath, "branch", flag, c.Name); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("%s の削除に失敗: %w", c.Name, err))

		return
	}

	result.Deleted = append(result.Deleted, c)
}

// pruneRemoteRefs は指定リモートのスタレ参照をまとめて prune します。
func pruneRemoteRefs(ctx context.Context, cleanPath, remote string, refs []BranchCandidate, dryRun bool, result *BranchCleanResult) {
	if dryRun {
		result.Pruned = append(result.Pruned, refs...)

		return
	}

	if err := runGitCommand(ctx, cleanPath, "remote", "prune", remote); err != nil {
		for _, ref := range refs {
			result.Errors = append(result.Errors, fmt.Errorf("%s のプルーンに失敗: %w", ref.Name, err))
		}

		return
	}

	result.Pruned = append(result.Pruned, refs...)
}
