package repo

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// BranchCategory はブランチクリーンアップ候補のカテゴリです。
type BranchCategory string

const (
	// BranchCategoryMerged はデフォルトブランチにマージ済みのローカルブランチです。安全に削除できます。
	BranchCategoryMerged BranchCategory = "merged"
	// BranchCategoryUnmerged はデフォルトブランチに未マージのローカルブランチです（upstream が gone のもののみ候補化）。デフォルトでは git branch -d で安全削除を試み、失敗時は Skipped 扱いとし、`--force` 指定時のみ -D で強制削除します。
	BranchCategoryUnmerged BranchCategory = "unmerged"
	// BranchCategoryStaleRef はリモートに存在しないリモートトラッキング参照です。prune で除去できます。
	BranchCategoryStaleRef BranchCategory = "stale_ref"
	// BranchCategoryNoUpstream はアップストリームが設定されていないローカルブランチです。デフォルトでは git branch -d で安全削除を試み、失敗時は Skipped 扱いとし、`--force` 指定時のみ -D で強制削除します。
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

	unmergedSet := make(map[string]struct{}, len(unmerged))
	for _, c := range unmerged {
		unmergedSet[c.Name] = struct{}{}
	}

	noUpstream, err := scanNoUpstreamBranches(ctx, cleanPath, excluded, mergedSet, unmergedSet)
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

// scanUnmergedBranches は upstream が gone（リモート側で削除済み）のローカルブランチを収集します。
//
// Issue #1 の Must 要件「未 push commit があるブランチは絶対に保持」「upstream gone のみ削除対象」を満たすため、
// 通常の作業ブランチ（upstream 生存）や未 push commit を含むブランチ（ahead）は候補に含めません。
// upstream:track の出力解析はロケール依存のため LANG/LC_ALL=C を強制します。
func scanUnmergedBranches(ctx context.Context, repoPath, defaultRef string, excluded, mergedSet map[string]struct{}) ([]BranchCandidate, error) {
	_ = defaultRef // 互換のため引数は保持（カテゴリ判定は upstream:track に切り替え）

	output, err := runGitCommandOutputLocaleC(ctx, repoPath,
		"for-each-ref",
		"--format=%(refname:short)%00%(upstream:short)%00%(upstream:track)%00%(committerdate:unix)",
		"refs/heads",
	)
	if err != nil {
		return nil, err
	}

	var candidates []BranchCandidate

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		c, ok := parseUnmergedBranchLine(line, excluded, mergedSet)
		if !ok {
			continue
		}

		candidates = append(candidates, c)
	}

	return candidates, nil
}

// parseUnmergedBranchLine は for-each-ref の1行を解析し、UNMERGED 候補に該当するか判定します。
// 該当しない（ヘッダ空行・除外・既にMERGED・upstream未設定・ahead・gone以外）場合は ok=false を返します。
func parseUnmergedBranchLine(line string, excluded, mergedSet map[string]struct{}) (BranchCandidate, bool) {
	if strings.TrimSpace(line) == "" {
		return BranchCandidate{}, false
	}

	parts := strings.SplitN(line, "\x00", 4)

	branch := strings.TrimSpace(parts[0])
	if branch == "" {
		return BranchCandidate{}, false
	}

	if isExcludedBranch(excluded, branch) {
		return BranchCandidate{}, false
	}

	if _, isMerged := mergedSet[branch]; isMerged {
		return BranchCandidate{}, false
	}

	upstream := ""
	if len(parts) >= 2 {
		upstream = strings.TrimSpace(parts[1])
	}

	// upstream 未設定は NO_UPSTREAM 側で扱う
	if upstream == "" {
		return BranchCandidate{}, false
	}

	track := ""
	if len(parts) >= 3 {
		track = strings.TrimSpace(parts[2])
	}

	// 未 push commit を含むブランチ（ahead）は絶対に候補に入れない（Issue #1 Must）
	// upstream gone のみ削除候補に入れる
	if strings.Contains(track, "ahead") || !strings.Contains(track, "gone") {
		return BranchCandidate{}, false
	}

	age := ""
	if len(parts) >= 4 {
		age = formatRelativeAgeJP(strings.TrimSpace(parts[3]), time.Now())
	}

	return BranchCandidate{
		Name:     branch,
		Category: BranchCategoryUnmerged,
		Age:      age,
	}, true
}

// formatRelativeAgeJP は committerdate:unix 形式の文字列を日本語の相対表現に整形します。
// ロケール非依存にするため git の relative 文字列ではなく Go 側で算出します。
// 解析失敗時は空文字列を返します。
func formatRelativeAgeJP(unixStr string, now time.Time) string {
	if unixStr == "" {
		return ""
	}

	sec, err := strconv.ParseInt(unixStr, 10, 64)
	if err != nil {
		return ""
	}

	diff := now.Sub(time.Unix(sec, 0))
	if diff < 0 {
		diff = 0
	}

	switch {
	case diff < time.Minute:
		return "たった今"
	case diff < time.Hour:
		return fmt.Sprintf("%d分前", int(diff/time.Minute))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%d時間前", int(diff/time.Hour))
	case diff < 7*24*time.Hour:
		return fmt.Sprintf("%d日前", int(diff/(24*time.Hour)))
	case diff < 30*24*time.Hour:
		return fmt.Sprintf("%d週間前", int(diff/(7*24*time.Hour)))
	case diff < 365*24*time.Hour:
		return fmt.Sprintf("%dヶ月前", int(diff/(30*24*time.Hour)))
	default:
		return fmt.Sprintf("%d年前", int(diff/(365*24*time.Hour)))
	}
}

// scanNoUpstreamBranches はアップストリームが未設定のローカルブランチを収集します。
// 既に MERGED または UNMERGED に分類されたブランチは除外して重複を防ぎます。
func scanNoUpstreamBranches(ctx context.Context, repoPath string, excluded, mergedSet, unmergedSet map[string]struct{}) ([]BranchCandidate, error) {
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

		if _, isUnmerged := unmergedSet[branch]; isUnmerged {
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
// 出力中の "[would prune]" マーカーはロケール依存のため LANG/LC_ALL=C を強制します。
func scanStaleRemoteRefs(ctx context.Context, repoPath, remote string) ([]BranchCandidate, error) {
	if remote == "" {
		remote = defaultRemoteName
	}

	output, err := runGitCommandOutputLocaleC(ctx, repoPath, "remote", "prune", remote, "--dry-run")
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
