package repo

import (
	"context"
	"errors"
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
	// Skipped は未 push commit 等のため安全に削除できず KEEP したブランチ（警告レベル、エラーではない）
	Skipped []BranchSkip
	// Errors は各操作で発生したエラーのリスト
	Errors []error
}

// BranchSkip は削除をスキップしたブランチと理由を表します。
type BranchSkip struct {
	Candidate BranchCandidate
	Reason    string
}

// DeleteBranchCandidates は選択されたブランチ候補を削除・プルーンします。
//
// 削除フラグ:
//   - MERGED:       git branch -d（安全削除）
//   - UNMERGED:     git branch -d（safe by default; force=true の場合は -D で強制削除）
//   - NO_UPSTREAM:  git branch -d（safe by default; force=true の場合は -D で強制削除）
//   - STALE_REF:    git remote prune <remote>（リモートごとに1回まとめて実行）
//
// -d 失敗時はエラーではなく Skipped に記録します（未 push commit を含むブランチを誤って失う事故を防ぐため）。
// dryRun が true の場合は実際の操作は行わず、結果に候補を記録するのみです。
func DeleteBranchCandidates(ctx context.Context, repoPath string, candidates []BranchCandidate, dryRun, force bool) (*BranchCleanResult, error) {
	cleanPath := filepath.Clean(repoPath)
	result := &BranchCleanResult{RepoPath: cleanPath}

	staleByRemote, localBranches := separateBranchCandidates(candidates)

	for _, c := range localBranches {
		deleteLocalBranch(ctx, cleanPath, c, dryRun, force, result)
	}

	for remote, refs := range staleByRemote {
		pruneRemoteRefs(ctx, cleanPath, remote, refs, dryRun, result)
	}

	if len(result.Errors) > 0 {
		joined := errors.Join(result.Errors...)

		return result, fmt.Errorf("%d 件の操作に失敗しました: %w", len(result.Errors), joined)
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
// safe by default: UNMERGED/NO_UPSTREAM でも force=false のときは `-d`（安全削除）を使い、
// 失敗時は Skipped に記録します（未 push commit を含むブランチを誤って失う事故を防ぐため）。
// force=true のときのみ `-D`（強制削除）を使います。
func deleteLocalBranch(ctx context.Context, cleanPath string, c BranchCandidate, dryRun, force bool, result *BranchCleanResult) {
	flag := "-d"
	if force && (c.Category == BranchCategoryUnmerged || c.Category == BranchCategoryNoUpstream) {
		flag = "-D"
	}

	if dryRun {
		result.Deleted = append(result.Deleted, c)

		return
	}

	if err := runGitCommand(ctx, cleanPath, "branch", flag, c.Name); err != nil {
		// safe by default モードでは -d が失敗しても通常想定（例: --force で再実行を促す）
		if flag == "-d" && (c.Category == BranchCategoryUnmerged || c.Category == BranchCategoryNoUpstream) {
			result.Skipped = append(result.Skipped, BranchSkip{
				Candidate: c,
				Reason:    "未 push commit がある可能性があるため安全削除に失敗しました（強制削除するには --force を指定）",
			})

			return
		}

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
