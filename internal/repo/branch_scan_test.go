package repo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanBranches_Merged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		setupRepo    func(t *testing.T) (repoPath, featureBranch string)
		wantInMerged bool
		wantBranch   string
	}{
		{
			name: "マージ済みブランチが MERGED カテゴリに含まれる",
			setupRepo: func(t *testing.T) (string, string) {
				t.Helper()

				repoPath := createRepoWithUpstream(t)
				defaultBranch := getOriginDefaultBranchName(t, repoPath)
				branch := "dsx-test-bs-merged"
				runGit(t, repoPath, "checkout", "-b", branch)
				writeTempFile(t, repoPath, "feature.txt", "content\n")
				runGit(t, repoPath, "add", "feature.txt")
				runGit(t, repoPath, "commit", "-m", "feature commit")
				runGit(t, repoPath, "checkout", defaultBranch)
				runGit(t, repoPath, "merge", "--no-ff", branch)

				return repoPath, branch
			},
			wantInMerged: true,
			wantBranch:   "dsx-test-bs-merged",
		},
		{
			name: "未マージブランチは MERGED カテゴリに含まれない",
			setupRepo: func(t *testing.T) (string, string) {
				t.Helper()

				repoPath := createRepoWithUpstream(t)
				branch := "dsx-test-bs-unmerged"
				runGit(t, repoPath, "checkout", "-b", branch)
				writeTempFile(t, repoPath, "wip.txt", "wip\n")
				runGit(t, repoPath, "add", "wip.txt")
				runGit(t, repoPath, "commit", "-m", "wip commit")
				defaultBranch := getOriginDefaultBranchName(t, repoPath)
				runGit(t, repoPath, "checkout", defaultBranch)

				return repoPath, branch
			},
			wantInMerged: false,
			wantBranch:   "dsx-test-bs-unmerged",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repoPath, branch := tt.setupRepo(t)

			result, err := ScanBranches(context.Background(), repoPath, BranchScanOptions{Fetch: false})
			if err != nil {
				t.Fatalf("ScanBranches() error = %v", err)
			}

			inMerged := false

			for _, c := range result.Candidates {
				if c.Category == BranchCategoryMerged && c.Name == branch {
					inMerged = true

					break
				}
			}

			if inMerged != tt.wantInMerged {
				t.Errorf("MERGED に %q が含まれる = %v, want %v (candidates: %+v)", branch, inMerged, tt.wantInMerged, result.Candidates)
			}
		})
	}
}

// TestScanBranches_Unmerged は未マージブランチが UNMERGED カテゴリに含まれることを確認します。
func TestScanBranches_Unmerged(t *testing.T) {
	t.Parallel()

	repoPath := createRepoWithUpstream(t)
	branch := "dsx-test-bs-wip"
	defaultBranch := getOriginDefaultBranchName(t, repoPath)
	runGit(t, repoPath, "checkout", "-b", branch)
	writeTempFile(t, repoPath, "wip.txt", "wip\n")
	runGit(t, repoPath, "add", "wip.txt")
	runGit(t, repoPath, "commit", "-m", "wip commit")
	// upstream を設定して push → リモート側を削除して gone 状態を作る（新仕様の UNMERGED 条件）
	runGit(t, repoPath, "push", "-u", "origin", branch)
	runGit(t, repoPath, "push", "origin", "--delete", branch)
	runGit(t, repoPath, "fetch", "--prune")
	runGit(t, repoPath, "checkout", defaultBranch)

	result, err := ScanBranches(context.Background(), repoPath, BranchScanOptions{Fetch: false})
	if err != nil {
		t.Fatalf("ScanBranches() error = %v", err)
	}

	found := false

	for _, c := range result.Candidates {
		if c.Category == BranchCategoryUnmerged && c.Name == branch {
			found = true

			if c.Age == "" {
				t.Errorf("UNMERGED 候補の Age が空です")
			}

			break
		}
	}

	if !found {
		t.Errorf("UNMERGED に %q が含まれていません (candidates: %+v)", branch, result.Candidates)
	}
}

// TestScanBranches_MergeTransition は UNMERGED（upstream gone）ブランチがマージ後に MERGED に移動することを確認します。
func TestScanBranches_MergeTransition(t *testing.T) {
	t.Parallel()

	repoPath := createRepoWithUpstream(t)
	branch := "dsx-test-bs-merge-transition"
	defaultBranch := getOriginDefaultBranchName(t, repoPath)
	runGit(t, repoPath, "checkout", "-b", branch)
	runGit(t, repoPath, "commit", "--allow-empty", "-m", "empty commit")
	// upstream を gone 状態にしてマージ前は UNMERGED に分類されることを確認
	runGit(t, repoPath, "push", "-u", "origin", branch)
	runGit(t, repoPath, "push", "origin", "--delete", branch)
	runGit(t, repoPath, "fetch", "--prune")
	runGit(t, repoPath, "checkout", defaultBranch)

	// マージ前: UNMERGED にあるはず
	result, err := ScanBranches(context.Background(), repoPath, BranchScanOptions{Fetch: false})
	if err != nil {
		t.Fatalf("ScanBranches() error = %v", err)
	}

	hasUnmerged := false

	for _, c := range result.Candidates {
		if c.Category == BranchCategoryUnmerged && c.Name == branch {
			hasUnmerged = true

			break
		}
	}

	if !hasUnmerged {
		t.Errorf("マージ前: UNMERGED に %q が含まれていません", branch)
	}

	// マージ後: MERGED にあるはず
	runGit(t, repoPath, "merge", "--no-ff", branch)

	result2, err := ScanBranches(context.Background(), repoPath, BranchScanOptions{Fetch: false})
	if err != nil {
		t.Fatalf("ScanBranches() error = %v (after merge)", err)
	}

	hasMerged := false

	for _, c := range result2.Candidates {
		if c.Category == BranchCategoryMerged && c.Name == branch {
			hasMerged = true

			break
		}
	}

	if !hasMerged {
		t.Errorf("マージ後: MERGED に %q が含まれていません", branch)
	}
}

func TestScanBranches_NoUpstream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		setupRepo    func(t *testing.T) (repoPath, branch string)
		wantCategory BranchCategory
		wantFound    bool
	}{
		{
			name: "アップストリーム未設定の未マージブランチは NO_UPSTREAM に分類される",
			setupRepo: func(t *testing.T) (string, string) {
				t.Helper()

				repoPath := createRepoWithUpstream(t)
				defaultBranch := getOriginDefaultBranchName(t, repoPath)
				branch := "dsx-test-bs-noup"
				runGit(t, repoPath, "checkout", "-b", branch)
				runGit(t, repoPath, "commit", "--allow-empty", "-m", "local only")
				// アップストリームを設定しない
				runGit(t, repoPath, "checkout", defaultBranch)

				return repoPath, branch
			},
			wantCategory: BranchCategoryNoUpstream,
			wantFound:    true,
		},
		{
			name: "アップストリーム設定済みのブランチは NO_UPSTREAM に含まれない",
			setupRepo: func(t *testing.T) (string, string) {
				t.Helper()

				repoPath := createRepoWithUpstream(t)
				defaultBranch := getOriginDefaultBranchName(t, repoPath)
				branch := "dsx-test-bs-has-upstream"
				runGit(t, repoPath, "checkout", "-b", branch)
				runGit(t, repoPath, "commit", "--allow-empty", "-m", "with upstream")
				runGit(t, repoPath, "push", "-u", "origin", "HEAD")
				runGit(t, repoPath, "checkout", defaultBranch)

				return repoPath, branch
			},
			wantCategory: BranchCategoryNoUpstream,
			wantFound:    false,
		},
		{
			name: "アップストリーム未設定でもマージ済みなら MERGED に含まれ NO_UPSTREAM には含まれない",
			setupRepo: func(t *testing.T) (string, string) {
				t.Helper()

				repoPath := createRepoWithUpstream(t)
				defaultBranch := getOriginDefaultBranchName(t, repoPath)
				branch := "dsx-test-bs-merged-noup"
				runGit(t, repoPath, "checkout", "-b", branch)
				runGit(t, repoPath, "commit", "--allow-empty", "-m", "merged without upstream")
				runGit(t, repoPath, "checkout", defaultBranch)
				runGit(t, repoPath, "merge", "--no-ff", branch)

				return repoPath, branch
			},
			wantCategory: BranchCategoryNoUpstream,
			wantFound:    false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repoPath, branch := tt.setupRepo(t)

			result, err := ScanBranches(context.Background(), repoPath, BranchScanOptions{Fetch: false})
			if err != nil {
				t.Fatalf("ScanBranches() error = %v", err)
			}

			found := false

			for _, c := range result.Candidates {
				if c.Category == tt.wantCategory && c.Name == branch {
					found = true

					break
				}
			}

			if found != tt.wantFound {
				t.Errorf("カテゴリ %s に %q が含まれる = %v, want %v (candidates: %+v)",
					tt.wantCategory, branch, found, tt.wantFound, result.Candidates)
			}
		})
	}
}

// TestScanBranches_NoCategoryDuplication は、同じブランチが複数のカテゴリに
// 重複して分類されないことを検証します。重複があると DeleteBranchCandidates が
// 同じブランチを2回削除しようとして失敗するため、回帰テストとして固定します。
func TestScanBranches_NoCategoryDuplication(t *testing.T) {
	t.Parallel()

	repoPath := createRepoWithUpstream(t)
	defaultBranch := getOriginDefaultBranchName(t, repoPath)

	// アップストリーム未設定かつ未マージのブランチを作成
	branch := "dsx-test-bs-dup"
	runGit(t, repoPath, "checkout", "-b", branch)
	runGit(t, repoPath, "commit", "--allow-empty", "-m", "no upstream and unmerged")
	runGit(t, repoPath, "checkout", defaultBranch)

	result, err := ScanBranches(context.Background(), repoPath, BranchScanOptions{Fetch: false})
	if err != nil {
		t.Fatalf("ScanBranches() error = %v", err)
	}

	count := 0

	for _, c := range result.Candidates {
		if c.Name == branch {
			count++
		}
	}

	if count != 1 {
		t.Errorf("ブランチ %q が %d 回出現しました (期待値: 1, candidates: %+v)",
			branch, count, result.Candidates)
	}
}

func TestScanBranches_StaleRef(t *testing.T) {
	t.Parallel()

	t.Run("削除されたリモートブランチが STALE_REF に含まれる", func(t *testing.T) {
		t.Parallel()

		base := t.TempDir()
		remotePath := filepath.Join(base, "remote.git")
		workPath := filepath.Join(base, "work")

		runGit(t, "", "init", "--bare", remotePath)
		runGit(t, "", "clone", remotePath, workPath)
		runGit(t, workPath, "config", "user.email", "dsx-test@example.com")
		runGit(t, workPath, "config", "user.name", "dsx-test")

		// 初期コミット
		writeTempFile(t, workPath, "README.md", "# repo\n")
		runGit(t, workPath, "add", "README.md")
		runGit(t, workPath, "commit", "-m", "initial commit")
		runGit(t, workPath, "push", "-u", "origin", "HEAD")
		// origin/HEAD を設定（DetectDefaultBranch が必要とする）
		runGit(t, workPath, "remote", "set-head", "origin", "--auto")

		// ブランチをプッシュしてリモートに作成
		branch := "dsx-test-bs-stale"
		runGit(t, workPath, "checkout", "-b", branch)
		runGit(t, workPath, "commit", "--allow-empty", "-m", "stale branch commit")
		runGit(t, workPath, "push", "origin", branch)

		defaultBranch := getOriginDefaultBranchName(t, workPath)
		runGit(t, workPath, "checkout", defaultBranch)

		// リモート（ベア）でブランチを削除 → work 側はまだ origin/dsx-test-bs-stale を持つ
		// bare リポジトリでは -C が使えないため --git-dir を使う
		runGit(t, "", "--git-dir="+remotePath, "update-ref", "-d", "refs/heads/"+branch)

		result, err := ScanBranches(context.Background(), workPath, BranchScanOptions{Fetch: false})
		if err != nil {
			t.Fatalf("ScanBranches() error = %v", err)
		}

		found := false
		expectedRef := "origin/" + branch

		for _, c := range result.Candidates {
			if c.Category == BranchCategoryStaleRef && c.Name == expectedRef {
				found = true

				break
			}
		}

		if !found {
			t.Errorf("STALE_REF に %q が含まれていません (candidates: %+v)", expectedRef, result.Candidates)
		}
	})
}

func TestScanBranches_Exclusion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		exclude     []string
		checkoutOn  string // 空の場合は default ブランチのまま
		setupBranch string
		wantPresent bool
	}{
		{
			name:        "現在のブランチは候補に含まれない",
			checkoutOn:  "dsx-test-bs-current",
			setupBranch: "dsx-test-bs-current",
			wantPresent: false,
		},
		{
			name:        "--exclude に指定したブランチは候補に含まれない",
			exclude:     []string{"dsx-test-bs-excluded"},
			setupBranch: "dsx-test-bs-excluded",
			wantPresent: false,
		},
		{
			name:        "--exclude 未指定の場合はブランチが候補に含まれる",
			setupBranch: "dsx-test-bs-included",
			wantPresent: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repoPath := createRepoWithUpstream(t)
			defaultBranch := getOriginDefaultBranchName(t, repoPath)

			runGit(t, repoPath, "checkout", "-b", tt.setupBranch)
			runGit(t, repoPath, "commit", "--allow-empty", "-m", "test commit")

			if tt.checkoutOn == "" {
				runGit(t, repoPath, "checkout", defaultBranch)
			}
			// checkoutOn != "" の場合はそのブランチに留まる

			result, err := ScanBranches(context.Background(), repoPath, BranchScanOptions{
				Fetch:           false,
				ExcludeBranches: tt.exclude,
			})
			if err != nil {
				t.Fatalf("ScanBranches() error = %v", err)
			}

			found := false

			for _, c := range result.Candidates {
				if c.Name == tt.setupBranch {
					found = true

					break
				}
			}

			if found != tt.wantPresent {
				t.Errorf("候補に %q が含まれる = %v, want %v (candidates: %+v)",
					tt.setupBranch, found, tt.wantPresent, result.Candidates)
			}
		})
	}
}

func TestScanBranches_NoRemote(t *testing.T) {
	t.Parallel()

	t.Run("リモートなしリポジトリはエラーを返す", func(t *testing.T) {
		t.Parallel()

		repoPath := t.TempDir()
		runGit(t, repoPath, "init")
		runGit(t, repoPath, "config", "user.email", "dsx-test@example.com")
		runGit(t, repoPath, "config", "user.name", "dsx-test")
		writeTempFile(t, repoPath, "README.md", "# repo\n")
		runGit(t, repoPath, "add", "README.md")
		runGit(t, repoPath, "commit", "-m", "initial commit")

		_, err := ScanBranches(context.Background(), repoPath, BranchScanOptions{Fetch: false})
		if err == nil {
			t.Fatalf("ScanBranches() error = nil, want error（リモートなし）")
		}
	})
}

func TestScanBranches_DefaultBranchExcluded(t *testing.T) {
	t.Parallel()

	t.Run("デフォルトブランチは候補に含まれない", func(t *testing.T) {
		t.Parallel()

		repoPath := createRepoWithUpstream(t)
		defaultBranch := getOriginDefaultBranchName(t, repoPath)

		result, err := ScanBranches(context.Background(), repoPath, BranchScanOptions{Fetch: false})
		if err != nil {
			t.Fatalf("ScanBranches() error = %v", err)
		}

		for _, c := range result.Candidates {
			if c.Name == defaultBranch {
				t.Errorf("デフォルトブランチ %q が候補に含まれています (category: %s)", defaultBranch, c.Category)
			}
		}
	})
}

func TestCategoryLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		category BranchCategory
		want     string
	}{
		{BranchCategoryMerged, "[MERGED]"},
		{BranchCategoryUnmerged, "[UNMERGED]"},
		{BranchCategoryStaleRef, "[STALE-REF]"},
		{BranchCategoryNoUpstream, "[NO-UPSTREAM]"},
		{"unknown_category", "[UNKNOWN]"},
	}

	for _, tt := range tests {
		got := CategoryLabel(tt.category)
		if got != tt.want {
			t.Errorf("CategoryLabel(%q) = %q, want %q", tt.category, got, tt.want)
		}
	}
}

func TestIsSafeToAutoDelete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		category BranchCategory
		want     bool
	}{
		{BranchCategoryMerged, true},
		{BranchCategoryStaleRef, true},
		{BranchCategoryUnmerged, false},
		{BranchCategoryNoUpstream, false},
		{"unknown_category", false},
	}

	for _, tt := range tests {
		got := IsSafeToAutoDelete(tt.category)
		if got != tt.want {
			t.Errorf("IsSafeToAutoDelete(%q) = %v, want %v", tt.category, got, tt.want)
		}
	}
}

// writeTempFile はテスト用にリポジトリ内にファイルを書き込みます。
func writeTempFile(t *testing.T, repoPath, filename, content string) {
	t.Helper()

	filePath := filepath.Join(repoPath, filename)
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write file %s: %v", filename, err)
	}
}

func TestDeleteBranchCandidates_DryRun(t *testing.T) {
	t.Parallel()

	t.Run("dryRun=true では実際の削除は行われない", func(t *testing.T) {
		t.Parallel()

		repoPath, featureBranch := setupRepoWithMergedBranch(t)

		candidates := []BranchCandidate{
			{Name: featureBranch, Category: BranchCategoryMerged},
		}

		result, err := DeleteBranchCandidates(context.Background(), repoPath, candidates, true, false)
		if err != nil {
			t.Fatalf("DeleteBranchCandidates() error = %v", err)
		}

		if len(result.Deleted) != 1 || result.Deleted[0].Name != featureBranch {
			t.Errorf("Deleted = %+v, want [{%s MERGED}]", result.Deleted, featureBranch)
		}

		// dryRun なので実際のブランチは残っているはず
		exists, err := localBranchExists(context.Background(), repoPath, featureBranch)
		if err != nil {
			t.Fatalf("localBranchExists() error = %v", err)
		}

		if !exists {
			t.Errorf("dryRun=true なのにブランチ %q が削除されました", featureBranch)
		}
	})
}

func TestDeleteBranchCandidates_Delete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setupRepo func(t *testing.T) (repoPath, branch string)
		category  BranchCategory
	}{
		{
			name:      "MERGED ブランチを安全削除（-d）",
			setupRepo: setupRepoWithMergedBranch,
			category:  BranchCategoryMerged,
		},
		{
			name: "UNMERGED ブランチを強制削除（-D）",
			setupRepo: func(t *testing.T) (string, string) {
				t.Helper()

				repoPath := createRepoWithUpstream(t)
				defaultBranch := getOriginDefaultBranchName(t, repoPath)
				branch := "dsx-test-bc-unmerged"
				runGit(t, repoPath, "checkout", "-b", branch)
				runGit(t, repoPath, "commit", "--allow-empty", "-m", "unmerged commit")
				runGit(t, repoPath, "checkout", defaultBranch)

				return repoPath, branch
			},
			category: BranchCategoryUnmerged,
		},
		{
			name: "NO_UPSTREAM ブランチを強制削除（-D）",
			setupRepo: func(t *testing.T) (string, string) {
				t.Helper()

				repoPath := createRepoWithUpstream(t)
				defaultBranch := getOriginDefaultBranchName(t, repoPath)
				branch := "dsx-test-bc-noup"
				runGit(t, repoPath, "checkout", "-b", branch)
				runGit(t, repoPath, "commit", "--allow-empty", "-m", "local only")
				runGit(t, repoPath, "checkout", defaultBranch)

				return repoPath, branch
			},
			category: BranchCategoryNoUpstream,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repoPath, branch := tt.setupRepo(t)
			candidates := []BranchCandidate{{Name: branch, Category: tt.category}}

			result, err := DeleteBranchCandidates(context.Background(), repoPath, candidates, false, true)
			if err != nil {
				t.Fatalf("DeleteBranchCandidates() error = %v", err)
			}

			if len(result.Deleted) != 1 {
				t.Errorf("Deleted = %+v, want 1 item", result.Deleted)
			}

			exists, err := localBranchExists(context.Background(), repoPath, branch)
			if err != nil {
				t.Fatalf("localBranchExists() error = %v", err)
			}

			if exists {
				t.Errorf("ブランチ %q が削除されていません", branch)
			}
		})
	}
}

// setupRepoWithMergedBranch はマージ済みフィーチャーブランチを持つリポジトリを作成します。
func setupRepoWithMergedBranch(t *testing.T) (repoPath, branch string) {
	t.Helper()

	repoPath = createRepoWithUpstream(t)
	defaultBranch := getOriginDefaultBranchName(t, repoPath)
	branch = "dsx-test-bc-merged"
	runGit(t, repoPath, "checkout", "-b", branch)
	runGit(t, repoPath, "commit", "--allow-empty", "-m", "merged commit")
	runGit(t, repoPath, "checkout", defaultBranch)
	runGit(t, repoPath, "merge", "--no-ff", branch)

	return repoPath, branch
}

func TestScanBranches_EmptyRepo(t *testing.T) {
	t.Parallel()

	t.Run("候補なしリポジトリは空スライスを返す", func(t *testing.T) {
		t.Parallel()

		// ブランチを追加せずにリポジトリだけ作成
		repoPath := createRepoWithUpstream(t)

		result, err := ScanBranches(context.Background(), repoPath, BranchScanOptions{Fetch: false})
		if err != nil {
			t.Fatalf("ScanBranches() error = %v", err)
		}

		// デフォルトブランチしかないので候補は0件
		// (デフォルトブランチは excluded なのでゼロ件のはず)
		// ただしアップストリーム未設定の他のブランチが出ないかチェック
		for _, c := range result.Candidates {
			t.Errorf("候補なしリポジトリに候補が含まれています: %+v", c)
		}
	})
}

func TestScanBranches_ResultFields(t *testing.T) {
	t.Parallel()

	t.Run("BranchScanResult のフィールドが正しく設定される", func(t *testing.T) {
		t.Parallel()

		repoPath := createRepoWithUpstream(t)

		result, err := ScanBranches(context.Background(), repoPath, BranchScanOptions{Fetch: false})
		if err != nil {
			t.Fatalf("ScanBranches() error = %v", err)
		}

		if strings.TrimSpace(result.DefaultBranch) == "" {
			t.Errorf("DefaultBranch が空です")
		}

		if strings.TrimSpace(result.CurrentBranch) == "" {
			t.Errorf("CurrentBranch が空です")
		}

		if strings.TrimSpace(result.RepoPath) == "" {
			t.Errorf("RepoPath が空です")
		}
	})
}
