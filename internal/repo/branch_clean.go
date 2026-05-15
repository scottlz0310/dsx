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
	// Skipped は未 push commit 等のため安全に削除できず保持したブランチ（警告レベル、エラーではない）
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
//   - MERGED:       git branch -d（安全削除）。-d が HEAD 依存で失敗した場合は
//     git merge-base --is-ancestor でデフォルトブランチへのマージを再確認し、
//     マージ済みであれば git branch -D で削除します。
//   - UNMERGED:     デフォルトは安全側（git branch -d）。--force 指定時のみ git branch -D で強制削除
//   - NO_UPSTREAM:  デフォルトは安全側（git branch -d）。--force 指定時のみ git branch -D で強制削除
//   - STALE_REF:    git remote prune <remote>（リモートごとに1回まとめて実行）
//
// UNMERGED/NO_UPSTREAM の -d 失敗時はエラーではなく Skipped に記録します
// （未 push commit を含むブランチを誤って失う事故を防ぐため）。
// dryRun が true の場合は実際の操作は行わず、結果に候補を記録するのみです。
func DeleteBranchCandidates(ctx context.Context, repoPath string, candidates []BranchCandidate, dryRun, force bool, defaultBranch string) (*BranchCleanResult, error) {
	cleanPath := filepath.Clean(repoPath)
	result := &BranchCleanResult{RepoPath: cleanPath}

	staleByRemote, localBranches := separateBranchCandidates(candidates)

	for _, c := range localBranches {
		deleteLocalBranch(ctx, cleanPath, c, dryRun, force, defaultBranch, result)
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
// 安全性方針（デフォルト）: UNMERGED/NO_UPSTREAM でも --force 未指定のときは `-d`（安全削除）を使い、
// 失敗時は Skipped に記録します（未 push commit を含むブランチを誤って失う事故を防ぐため）。
// --force 指定時のみ `-D`（強制削除）を使います。
//
// MERGED 救済: `-d` は「現在の HEAD にマージ済みか」を判定するため、ユーザーが
// デフォルトブランチ以外を checkout している状態だと、デフォルトブランチへマージ済みでも
// `-d` が失敗します。この場合は git merge-base --is-ancestor でデフォルトブランチへの
// マージを再確認し、確認できれば `-D` で削除します（判定基準と削除条件を揃える）。
func deleteLocalBranch(ctx context.Context, cleanPath string, c BranchCandidate, dryRun, force bool, defaultBranch string, result *BranchCleanResult) {
	flag := "-d"
	if force && (c.Category == BranchCategoryUnmerged || c.Category == BranchCategoryNoUpstream) {
		flag = "-D"
	}

	if dryRun {
		result.Deleted = append(result.Deleted, c)

		return
	}

	if err := runGitCommand(ctx, cleanPath, "branch", flag, c.Name); err != nil {
		if flag == "-d" && c.Category == BranchCategoryMerged && defaultBranch != "" {
			if mergedIntoDefault(ctx, cleanPath, defaultBranch, c.Name) {
				if forceErr := runGitCommand(ctx, cleanPath, "branch", "-D", c.Name); forceErr == nil {
					result.Deleted = append(result.Deleted, c)

					return
				}
			}
		}

		// 安全モードでは UNMERGED/NO_UPSTREAM の -d 失敗は想定内（--force で再実行を促す）
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

// mergedIntoDefault はブランチがデフォルトブランチへマージ済みかどうかを
// git merge-base --is-ancestor <branch> <defaultBranch> で確認します。
// 確認できない場合（コマンド失敗など）は false を返します。
func mergedIntoDefault(ctx context.Context, cleanPath, defaultBranch, branch string) bool {
	if defaultBranch == "" || branch == "" {
		return false
	}

	err := runGitCommand(ctx, cleanPath, "merge-base", "--is-ancestor", branch, defaultBranch)

	return err == nil
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
