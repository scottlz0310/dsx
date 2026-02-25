package repo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanup_Merged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		setupRepo         func(t *testing.T) (repoPath, defaultBranch, featureBranch string)
		opts              func(defaultBranch, featureBranch string) CleanupOptions
		wantPlanContains  string
		wantSkipContains  string
		expectDeleteAfter bool
	}{
		{
			name:      "dry-run: mergedブランチを削除計画に含める",
			setupRepo: createRepoWithMergedFeatureBranch,
			opts: func(_ string, _ string) CleanupOptions {
				return CleanupOptions{
					Prune:           true,
					DryRun:          true,
					Targets:         []string{"merged"},
					ExcludeBranches: nil,
				}
			},
			wantPlanContains: "dsx-test-feature",
		},
		{
			name:      "exclude_branches に含まれる場合は削除しない",
			setupRepo: createRepoWithMergedFeatureBranch,
			opts: func(_ string, featureBranch string) CleanupOptions {
				return CleanupOptions{
					Prune:           true,
					DryRun:          true,
					Targets:         []string{"merged"},
					ExcludeBranches: []string{featureBranch},
				}
			},
			wantSkipContains: "削除対象のブランチがありません",
		},
		{
			name:      "現在のブランチは削除しない",
			setupRepo: createRepoWithMergedFeatureBranchCheckedOut,
			opts: func(_ string, _ string) CleanupOptions {
				return CleanupOptions{
					Prune:           true,
					DryRun:          true,
					Targets:         []string{"merged"},
					ExcludeBranches: nil,
				}
			},
			wantSkipContains: "削除対象のブランチがありません",
		},
		{
			name:      "未コミット変更がある場合は安全側にスキップ",
			setupRepo: createRepoWithMergedFeatureBranchAndDirty,
			opts: func(_ string, _ string) CleanupOptions {
				return CleanupOptions{
					Prune:           true,
					DryRun:          true,
					Targets:         []string{"merged"},
					ExcludeBranches: nil,
				}
			},
			wantSkipContains: "未コミットの変更があるため cleanup をスキップ",
		},
		{
			name:      "non-dry-run: mergedブランチを削除する",
			setupRepo: createRepoWithMergedFeatureBranch,
			opts: func(_ string, _ string) CleanupOptions {
				return CleanupOptions{
					Prune:           true,
					DryRun:          false,
					Targets:         []string{"merged"},
					ExcludeBranches: nil,
				}
			},
			expectDeleteAfter: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repoPath, _, featureBranch := tt.setupRepo(t)

			result, err := Cleanup(context.Background(), repoPath, tt.opts("", featureBranch))
			if err != nil {
				t.Fatalf("Cleanup() error = %v", err)
			}

			if result == nil {
				t.Fatalf("Cleanup() result is nil")
			}

			assertPlannedMergedBranch(t, result.PlannedDeletes, tt.wantPlanContains)
			assertSkipMessageContainsOrEmpty(t, result.SkippedMessages, tt.wantSkipContains)
			assertBranchDeleted(t, repoPath, featureBranch, tt.expectDeleteAfter)
		})
	}
}

func TestCleanup_Squashed(t *testing.T) {
	t.Parallel()

	t.Run("dry-run: PR headが一致するブランチを強制削除計画に含める", func(t *testing.T) {
		t.Parallel()

		repoPath := createRepoWithUpstream(t)
		defaultBranch := getOriginDefaultBranchName(t, repoPath)

		branch := "dsx-test-squashed"
		runGit(t, repoPath, "checkout", "-b", branch)

		filePath := filepath.Join(repoPath, "README.md")
		if err := os.WriteFile(filePath, []byte("# upstream\nsquashed\n"), 0o644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}

		runGit(t, repoPath, "add", "README.md")
		runGit(t, repoPath, "commit", "-m", "squashed commit")

		tip := strings.TrimSpace(string(runGitCommandOutputOrFail(t, repoPath, "rev-parse", branch)))
		runGit(t, repoPath, "checkout", defaultBranch)

		result, err := Cleanup(context.Background(), repoPath, CleanupOptions{
			Prune:                  true,
			DryRun:                 true,
			Targets:                []string{"squashed"},
			SquashedPRHeadByBranch: map[string]string{branch: tip},
		})
		if err != nil {
			t.Fatalf("Cleanup() error = %v", err)
		}

		found := false
		for _, plan := range result.PlannedDeletes {
			if plan.Branch == branch && plan.Target == cleanupTargetSquashed && plan.Force {
				found = true
				break
			}
		}

		if !found {
			t.Fatalf("PlannedDeletes does not contain squashed plan: %+v", result.PlannedDeletes)
		}
	})

	t.Run("non-dry-run: squashed対象は強制削除する", func(t *testing.T) {
		t.Parallel()

		repoPath := createRepoWithUpstream(t)
		defaultBranch := getOriginDefaultBranchName(t, repoPath)

		branch := "dsx-test-squashed-delete"
		runGit(t, repoPath, "checkout", "-b", branch)
		runGit(t, repoPath, "commit", "--allow-empty", "-m", "squashed commit")

		tip := strings.TrimSpace(string(runGitCommandOutputOrFail(t, repoPath, "rev-parse", branch)))
		runGit(t, repoPath, "checkout", defaultBranch)

		_, err := Cleanup(context.Background(), repoPath, CleanupOptions{
			Prune:                  true,
			DryRun:                 false,
			Targets:                []string{"squashed"},
			SquashedPRHeadByBranch: map[string]string{branch: tip},
		})
		if err != nil {
			t.Fatalf("Cleanup() error = %v", err)
		}

		exists, err := localBranchExists(context.Background(), repoPath, branch)
		if err != nil {
			t.Fatalf("localBranchExists() error = %v", err)
		}

		if exists {
			t.Fatalf("branch should be deleted: %s", branch)
		}
	})

	t.Run("PR headが一致しない場合は削除しない", func(t *testing.T) {
		t.Parallel()

		repoPath := createRepoWithUpstream(t)
		defaultBranch := getOriginDefaultBranchName(t, repoPath)

		branch := "dsx-test-squashed-mismatch"
		runGit(t, repoPath, "checkout", "-b", branch)
		runGit(t, repoPath, "commit", "--allow-empty", "-m", "squashed commit")
		runGit(t, repoPath, "checkout", defaultBranch)

		result, err := Cleanup(context.Background(), repoPath, CleanupOptions{
			Prune:                  true,
			DryRun:                 true,
			Targets:                []string{"squashed"},
			SquashedPRHeadByBranch: map[string]string{branch: "deadbeef"},
		})
		if err != nil {
			t.Fatalf("Cleanup() error = %v", err)
		}

		if len(result.PlannedDeletes) != 0 {
			t.Fatalf("PlannedDeletes should be empty: %+v", result.PlannedDeletes)
		}

		if !hasMessageContaining(result.SkippedMessages, "削除対象のブランチがありません") {
			t.Fatalf("SkippedMessages should mention no targets: %v", result.SkippedMessages)
		}
	})
}

func TestCleanup_DryRun_FetchDoesNotPrune(t *testing.T) {
	t.Parallel()

	repoPath := createRepoWithUpstream(t)

	result, err := Cleanup(context.Background(), repoPath, CleanupOptions{
		Prune:           true,
		DryRun:          true,
		Targets:         []string{"merged"},
		ExcludeBranches: nil,
	})
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	if result == nil {
		t.Fatalf("Cleanup() result is nil")
	}

	fetchCommand := ""
	for _, command := range result.Commands {
		if strings.Contains(command, "fetch --all") {
			fetchCommand = command
			break
		}
	}

	if fetchCommand == "" {
		t.Fatalf("Commands should contain fetch: %v", result.Commands)
	}

	if strings.Contains(fetchCommand, "--prune") {
		t.Fatalf("DryRun fetch should not include --prune: %s", fetchCommand)
	}
}

func TestLocalBranchExists(t *testing.T) {
	t.Parallel()

	t.Run("存在しないブランチはfalse,nil", func(t *testing.T) {
		t.Parallel()

		repoPath := createRepoWithUpstream(t)

		exists, err := localBranchExists(context.Background(), repoPath, "dsx-test-branch-not-exists")
		if err != nil {
			t.Fatalf("localBranchExists() error = %v", err)
		}

		if exists {
			t.Fatalf("localBranchExists() = true, want false")
		}
	})

	t.Run("gitリポジトリでない場合はエラー", func(t *testing.T) {
		t.Parallel()

		repoPath := t.TempDir()

		_, err := localBranchExists(context.Background(), repoPath, "main")
		if err == nil {
			t.Fatalf("localBranchExists() error = nil, want error")
		}
	})
}

func createRepoWithMergedFeatureBranch(t *testing.T) (repoPath, defaultBranch, featureBranch string) {
	t.Helper()

	repoPath = createRepoWithUpstream(t)
	defaultBranch = getOriginDefaultBranchName(t, repoPath)
	featureBranch = "dsx-test-feature"

	runGit(t, repoPath, "checkout", "-b", featureBranch)

	filePath := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(filePath, []byte("# upstream\nfeature\n"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	runGit(t, repoPath, "add", "README.md")
	runGit(t, repoPath, "commit", "-m", "feature commit")
	runGit(t, repoPath, "push", "-u", "origin", "HEAD")

	runGit(t, repoPath, "checkout", defaultBranch)
	runGit(t, repoPath, "merge", "--no-ff", featureBranch)
	runGit(t, repoPath, "push", "origin", defaultBranch)

	return repoPath, defaultBranch, featureBranch
}

func createRepoWithMergedFeatureBranchCheckedOut(t *testing.T) (repoPath, defaultBranch, featureBranch string) {
	t.Helper()

	repoPath, defaultBranch, featureBranch = createRepoWithMergedFeatureBranch(t)
	runGit(t, repoPath, "checkout", featureBranch)

	return repoPath, defaultBranch, featureBranch
}

func createRepoWithMergedFeatureBranchAndDirty(t *testing.T) (repoPath, defaultBranch, featureBranch string) {
	t.Helper()

	repoPath, defaultBranch, featureBranch = createRepoWithMergedFeatureBranch(t)

	filePath := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(filePath, []byte("# upstream\ndirty\n"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	return repoPath, defaultBranch, featureBranch
}

func runGitCommandOutputOrFail(t *testing.T, repoPath string, args ...string) []byte {
	t.Helper()

	output, err := runGitCommandOutput(context.Background(), repoPath, args...)
	if err != nil {
		t.Fatalf("git output command failed: %v", err)
	}

	return output
}

func assertPlannedMergedBranch(t *testing.T, plans []CleanupPlan, wantBranch string) {
	t.Helper()

	if strings.TrimSpace(wantBranch) == "" {
		if len(plans) != 0 {
			t.Fatalf("PlannedDeletes should be empty: %+v", plans)
		}

		return
	}

	for _, plan := range plans {
		if plan.Branch == wantBranch && plan.Target == cleanupTargetMerged {
			return
		}
	}

	t.Fatalf("PlannedDeletes does not contain %q: %+v", wantBranch, plans)
}

func assertSkipMessageContainsOrEmpty(t *testing.T, messages []string, wantSubstr string) {
	t.Helper()

	if strings.TrimSpace(wantSubstr) == "" {
		if len(messages) != 0 {
			t.Fatalf("SkippedMessages should be empty: %v", messages)
		}

		return
	}

	for _, msg := range messages {
		if strings.Contains(msg, wantSubstr) {
			return
		}
	}

	t.Fatalf("SkippedMessages does not contain %q: %v", wantSubstr, messages)
}

func assertBranchDeleted(t *testing.T, repoPath, branch string, wantDeleted bool) {
	t.Helper()

	exists, err := localBranchExists(context.Background(), repoPath, branch)
	if err != nil {
		t.Fatalf("localBranchExists() error = %v", err)
	}

	if wantDeleted && exists {
		t.Fatalf("branch should be deleted: %s", branch)
	}

	if !wantDeleted && !exists {
		t.Fatalf("branch should exist: %s", branch)
	}
}
