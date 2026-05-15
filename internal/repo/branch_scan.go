package repo

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// BranchCategory はブランチクリーンアップ候補のカテゴリです。
type BranchCategory string

const (
	// BranchCategoryMerged はデフォルトブランチにマージ済みのローカルブランチです。安全に削除できます。
	BranchCategoryMerged BranchCategory = "merged"
	// BranchCategoryUnmerged はデフォルトブランチに未マージのローカルブランチです。強制削除が必要です。
	BranchCategoryUnmerged BranchCategory = "unmerged"
	// BranchCategoryStaleRef はリモートに存在しないリモートトラッキング参照です。prune で除去できます。
	BranchCategoryStaleRef BranchCategory = "stale_ref"
	// BranchCategoryNoUpstream はアップストリームが設定されていないローカルブランチです。強制削除が必要です。
	BranchCategoryNoUpstream BranchCategory = "no_upstream"
)

// BranchCandidate はブランチクリーンアップ候補を表します。
type BranchCandidate struct {
	// Name はブランチ名、または STALE_REF の場合は "<remote>/<branch>" 形式
	Name string
	// Category はブランチのカテゴリ
	Category BranchCategory
	// Age は最終コミットからの経過時間（BranchCategoryUnmerged のみ設定）
	Age string
	// Remote はリモート名（BranchCategoryStaleRef のみ設定）
	Remote string
}

// BranchScanResult は単一リポジトリのブランチスキャン結果です。
type BranchScanResult struct {
	RepoPath      string
	DefaultBranch string
	CurrentBranch string
	Candidates    []BranchCandidate
}

// BranchScanOptions はブランチスキャンのオプションです。
type BranchScanOptions struct {
	// Fetch が true の場合、スキャン前に git fetch --prune を実行します。
	Fetch           bool
	ExcludeBranches []string
}

// ScanBranches は単一リポジトリのブランチクリーンアップ候補を収集します。
//
// 収集するカテゴリ:
//   - merged:      デフォルトブランチにマージ済みローカルブランチ（安全に削除可能）
//   - unmerged:    未マージのローカルブランチ（最終コミット経過時間付き）
//   - stale_ref:   リモートに存在しないリモートトラッキング参照（prune で除去）
//   - no_upstream: アップストリームが未設定のローカルブランチ
func ScanBranches(ctx context.Context, repoPath string, opts BranchScanOptions) (*BranchScanResult, error) {
	cleanPath := filepath.Clean(repoPath)
	result := &BranchScanResult{RepoPath: cleanPath}

	if opts.Fetch {
		if err := runGitCommand(ctx, cleanPath, "fetch", "--quiet", "--prune"); err != nil {
			return result, fmt.Errorf("fetch に失敗: %w", err)
		}
	}

	defaultInfo, err := DetectDefaultBranch(ctx, cleanPath)
	if err != nil {
		return result, fmt.Errorf("デフォルトブランチの検出に失敗: %w", err)
	}

	result.DefaultBranch = defaultInfo.Branch

	currentBranch, err := getCurrentBranchName(ctx, cleanPath)
	if err != nil {
		return result, fmt.Errorf("現在のブランチの取得に失敗: %w", err)
	}

	result.CurrentBranch = currentBranch

	excluded := buildExcludedBranchSet(currentBranch, defaultInfo.Branch, opts.ExcludeBranches)

	// ローカルのデフォルトブランチ名で判定することで、git push 前のローカルマージも検出できる
	mergedBranches, err := listMergedLocalBranches(ctx, cleanPath, defaultInfo.Branch)
	if err != nil {
		return result, fmt.Errorf("マージ済みブランチの取得に失敗: %w", err)
	}

	mergedSet := make(map[string]struct{}, len(mergedBranches))

	for _, b := range mergedBranches {
		if isExcludedBranch(excluded, b) {
			continue
		}

		mergedSet[b] = struct{}{}

		result.Candidates = append(result.Candidates, BranchCandidate{
			Name:     b,
			Category: BranchCategoryMerged,
		})
	}

	unmerged, err := scanUnmergedBranches(ctx, cleanPath, defaultInfo.Branch, excluded, mergedSet)
	if err != nil {
		return result, fmt.Errorf("未マージブランチの取得に失敗: %w", err)
	}

	result.Candidates = append(result.Candidates, unmerged...)

	noUpstream, err := scanNoUpstreamBranches(ctx, cleanPath, excluded, mergedSet)
	if err != nil {
		return result, fmt.Errorf("アップストリーム未設定ブランチの取得に失敗: %w", err)
	}

	result.Candidates = append(result.Candidates, noUpstream...)

	// スタレ参照の検出は非必須（ネットワーク不可など失敗する場合はスキップ）
	staleRefs, err := scanStaleRemoteRefs(ctx, cleanPath, defaultInfo.Remote)
	if err == nil {
		result.Candidates = append(result.Candidates, staleRefs...)
	}

	return result, nil
}

// CategoryLabel はカテゴリの固定幅表示ラベルを返します。
func CategoryLabel(c BranchCategory) string {
	switch c {
	case BranchCategoryMerged:
		return "[MERGED]"
	case BranchCategoryUnmerged:
		return "[UNMERGED]"
	case BranchCategoryStaleRef:
		return "[STALE-REF]"
	case BranchCategoryNoUpstream:
		return "[NO-UPSTREAM]"
	default:
		return "[UNKNOWN]"
	}
}

// IsSafeToAutoDelete は --yes モードで確認なしに自動削除できるカテゴリかどうかを返します。
// MERGED と STALE_REF のみ自動削除安全とみなします。
func IsSafeToAutoDelete(c BranchCategory) bool {
	return c == BranchCategoryMerged || c == BranchCategoryStaleRef
}

// scanUnmergedBranches はデフォルトブランチに未マージのローカルブランチを収集します。
// フォーマット文字列に %00（NUL バイト）を使い、相対日時のスペースを安全に扱います。
func scanUnmergedBranches(ctx context.Context, repoPath, defaultRef string, excluded, mergedSet map[string]struct{}) ([]BranchCandidate, error) {
	output, err := runGitCommandOutput(ctx, repoPath,
		"for-each-ref",
		"--format=%(refname:short)%00%(committerdate:relative)",
		"--no-merged="+defaultRef,
		"refs/heads",
	)
	if err != nil {
		return nil, err
	}

	var candidates []BranchCandidate

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.SplitN(line, "\x00", 2)

		branch := strings.TrimSpace(parts[0])
		if branch == "" {
			continue
		}

		age := ""
		if len(parts) == 2 {
			age = strings.TrimSpace(parts[1])
		}

		if isExcludedBranch(excluded, branch) {
			continue
		}

		if _, isMerged := mergedSet[branch]; isMerged {
			continue
		}

		candidates = append(candidates, BranchCandidate{
			Name:     branch,
			Category: BranchCategoryUnmerged,
			Age:      age,
		})
	}

	return candidates, nil
}

// scanNoUpstreamBranches はアップストリームが未設定のローカルブランチを収集します。
// 既に MERGED に分類されたブランチは除外して重複を防ぎます。
func scanNoUpstreamBranches(ctx context.Context, repoPath string, excluded, mergedSet map[string]struct{}) ([]BranchCandidate, error) {
	output, err := runGitCommandOutput(ctx, repoPath,
		"for-each-ref",
		"--format=%(refname:short)%00%(upstream:short)",
		"refs/heads",
	)
	if err != nil {
		return nil, err
	}

	var candidates []BranchCandidate

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "\x00", 2)

		branch := strings.TrimSpace(parts[0])
		if branch == "" {
			continue
		}

		// アップストリームが設定されている場合は2番目のフィールドが空でない
		if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
			continue
		}

		if isExcludedBranch(excluded, branch) {
			continue
		}

		if _, isMerged := mergedSet[branch]; isMerged {
			continue
		}

		candidates = append(candidates, BranchCandidate{
			Name:     branch,
			Category: BranchCategoryNoUpstream,
		})
	}

	return candidates, nil
}

// scanStaleRemoteRefs は git remote prune --dry-run で除去対象となるリモートトラッキング参照を収集します。
func scanStaleRemoteRefs(ctx context.Context, repoPath, remote string) ([]BranchCandidate, error) {
	if remote == "" {
		remote = "origin"
	}

	output, err := runGitCommandOutput(ctx, repoPath, "remote", "prune", remote, "--dry-run")
	if err != nil {
		return nil, err
	}

	const wouldPruneMarker = "[would prune]"

	var candidates []BranchCandidate

	for _, line := range strings.Split(string(output), "\n") {
		idx := strings.Index(line, wouldPruneMarker)
		if idx < 0 {
			continue
		}

		ref := strings.TrimSpace(line[idx+len(wouldPruneMarker):])
		if ref == "" {
			continue
		}

		candidates = append(candidates, BranchCandidate{
			Name:     ref,
			Category: BranchCategoryStaleRef,
			Remote:   remote,
		})
	}

	return candidates, nil
}
