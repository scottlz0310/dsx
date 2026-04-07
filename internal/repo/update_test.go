package repo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBuildFetchArgs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		prune bool
		want  []string
	}{
		{
			name:  "prune有効",
			prune: true,
			want:  []string{"fetch", "--all", "--prune"},
		},
		{
			name:  "prune無効",
			prune: false,
			want:  []string{"fetch", "--all"},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := buildFetchArgs(tc.prune)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("buildFetchArgs(%v) = %v, want %v", tc.prune, got, tc.want)
			}
		})
	}
}

func TestBuildPullArgs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		autoStash bool
		want      []string
	}{
		{
			name:      "autoStash有効",
			autoStash: true,
			want:      []string{"pull", "--rebase", "--autostash"},
		},
		{
			name:      "autoStash無効",
			autoStash: false,
			want:      []string{"pull", "--rebase"},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := buildPullArgs(tc.autoStash)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("buildPullArgs(%v) = %v, want %v", tc.autoStash, got, tc.want)
			}
		})
	}
}

func TestBuildSubmoduleArgs(t *testing.T) {
	t.Parallel()

	want := []string{"submodule", "update", "--init", "--recursive", "--remote"}
	got := buildSubmoduleArgs()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildSubmoduleArgs() = %v, want %v", got, want)
	}
}

func TestFormatGitCommand(t *testing.T) {
	t.Parallel()

	got := formatGitCommand("/tmp/repo", []string{"fetch", "--all", "--prune"})
	want := "git -C /tmp/repo fetch --all --prune"

	if got != want {
		t.Fatalf("formatGitCommand() = %q, want %q", got, want)
	}
}

func TestUpdateDryRunPlan(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		setupRepo          func(t *testing.T) string
		expectPullCommand  bool
		expectSkipContains string
	}{
		{
			name:              "upstreamありはpull計画を含む",
			setupRepo:         createRepoWithUpstream,
			expectPullCommand: true,
		},
		{
			name:              "ローカルブランチ名が異なってもデフォルト追跡ならpull計画を含む",
			setupRepo:         createRepoWithDifferentLocalBranchTrackingDefault,
			expectPullCommand: true,
		},
		{
			name:              "refs/remotes/origin/HEAD 未設定でも upstream があれば pull 計画を含む",
			setupRepo:         createRepoWithUpstreamWithoutRemoteHead,
			expectPullCommand: true,
		},
		{
			name:               "upstreamなしはpull計画を除外",
			setupRepo:          createLocalRepoWithoutUpstream,
			expectPullCommand:  false,
			expectSkipContains: skipPullNoUpstreamMessage,
		},
		{
			name:               "リポジトリ状態の判定失敗時はpull計画を除外して継続",
			setupRepo:          createBrokenWorktreeRepo,
			expectPullCommand:  false,
			expectSkipContains: "リポジトリ状態の判定に失敗",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repoPath := tc.setupRepo(t)

			result, err := Update(context.Background(), repoPath, UpdateOptions{
				Prune:           true,
				AutoStash:       true,
				SubmoduleUpdate: false,
				DryRun:          true,
			})
			if err != nil {
				t.Fatalf("Update() error = %v", err)
			}

			if result == nil {
				t.Fatalf("Update() result is nil")
			}

			if len(result.Commands) == 0 {
				t.Fatalf("Update() commands should not be empty")
			}

			if !hasCommandContaining(result.Commands, " fetch --all --prune") {
				t.Fatalf("fetch コマンドが計画に含まれていません: %v", result.Commands)
			}

			gotPull := hasCommandContaining(result.Commands, " pull --rebase --autostash")
			if gotPull != tc.expectPullCommand {
				t.Fatalf("pull コマンド有無 = %v, want %v, commands=%v", gotPull, tc.expectPullCommand, result.Commands)
			}

			if tc.expectSkipContains != "" && !hasMessageContaining(result.SkippedMessages, tc.expectSkipContains) {
				t.Fatalf("skipメッセージに %q が含まれていません: %v", tc.expectSkipContains, result.SkippedMessages)
			}
		})
	}
}

func TestUpdateSkipsOnUnsafeRepoState(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		setupRepo          func(t *testing.T) string
		autoStash          bool
		expectSkipContains string
	}{
		{
			// AutoStash=false のとき DIRTY はスキップされる。
			name:               "tracked の未コミット変更がある場合はスキップ（AutoStash=false）",
			setupRepo:          createRepoWithUpstreamAndDirtyTracked,
			autoStash:          false,
			expectSkipContains: "未コミットの変更があるため pull/submodule をスキップ",
		},
		{
			name:               "untracked の未コミット変更がある場合はスキップ（AutoStash=false）",
			setupRepo:          createRepoWithUpstreamAndDirtyUntracked,
			autoStash:          false,
			expectSkipContains: "未コミットの変更があるため pull/submodule をスキップ",
		},
		{
			name:               "stash が残っている場合はスキップ",
			setupRepo:          createRepoWithUpstreamAndStash,
			autoStash:          false,
			expectSkipContains: "stash が残っているため pull/submodule をスキップ",
		},
		{
			name:               "detached HEAD の場合はスキップ",
			setupRepo:          createRepoWithUpstreamAndDetachedHEAD,
			autoStash:          false,
			expectSkipContains: "detached HEAD のため pull/submodule をスキップ",
		},
		{
			name:               "upstream が <remote>/<branch> 形式でない場合はスキップ",
			setupRepo:          createRepoWithUpstreamAndLocalUpstreamRef,
			autoStash:          false,
			expectSkipContains: skipPullUpstreamDetectFailedMessage,
		},
		{
			name:               "デフォルトブランチ以外を追跡している場合はスキップ",
			setupRepo:          createRepoWithNonDefaultUpstream,
			autoStash:          false,
			expectSkipContains: skipPullNonDefaultUpstreamMessage,
		},
		{
			// AutoStash=true でも stash 残存はスキップされる（autostash との競合を避けるため）。
			name:               "stash が残っている場合は AutoStash=true でもスキップ",
			setupRepo:          createRepoWithUpstreamAndStash,
			autoStash:          true,
			expectSkipContains: "stash が残っているため pull/submodule をスキップ",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repoPath := tc.setupRepo(t)

			result, err := Update(context.Background(), repoPath, UpdateOptions{
				Prune:           true,
				AutoStash:       tc.autoStash,
				SubmoduleUpdate: false,
				DryRun:          true,
			})
			if err != nil {
				t.Fatalf("Update() error = %v", err)
			}

			if result == nil {
				t.Fatalf("Update() result is nil")
			}

			if len(result.Commands) == 0 {
				t.Fatalf("Update() commands should not be empty")
			}

			if !hasCommandContaining(result.Commands, " fetch --all --prune") {
				t.Fatalf("fetch コマンドが計画に含まれていません: %v", result.Commands)
			}

			if hasCommandContaining(result.Commands, " pull --rebase") {
				t.Fatalf("危険状態のはずなのに pull コマンドが計画に含まれています: %v", result.Commands)
			}

			if !hasMessageContaining(result.SkippedMessages, tc.expectSkipContains) {
				t.Fatalf("skipメッセージに %q が含まれていません: %v", tc.expectSkipContains, result.SkippedMessages)
			}
		})
	}
}

func TestUpdateSkipsOnUnsafeRepoStateNonDryRun(t *testing.T) {
	t.Parallel()

	repoPath := createRepoWithUpstreamAndDirtyTracked(t)

	result, err := Update(context.Background(), repoPath, UpdateOptions{
		Prune:           true,
		AutoStash:       false,
		SubmoduleUpdate: true,
		DryRun:          false,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if result == nil {
		t.Fatalf("Update() result is nil")
	}

	if hasCommandContaining(result.Commands, " pull --rebase") {
		t.Fatalf("危険状態のはずなのに pull コマンドが計画に含まれています: %v", result.Commands)
	}

	if hasCommandContaining(result.Commands, " submodule update") {
		t.Fatalf("危険状態のはずなのに submodule update コマンドが計画に含まれています: %v", result.Commands)
	}

	if !hasMessageContaining(result.SkippedMessages, "pull/submodule をスキップ") {
		t.Fatalf("skipメッセージに %q が含まれていません: %v", "pull/submodule をスキップ", result.SkippedMessages)
	}
}

func TestUpdateSkipsOnNonDefaultTrackingNonDryRun(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		setupRepo          func(t *testing.T) string
		expectSkipContains string
	}{
		{
			name:               "デフォルトブランチ以外を追跡している場合はスキップ",
			setupRepo:          createRepoWithNonDefaultUpstream,
			expectSkipContains: skipPullNonDefaultUpstreamMessage,
		},
		{
			name:               "upstream が <remote>/<branch> 形式でない場合はスキップ",
			setupRepo:          createRepoWithUpstreamAndLocalUpstreamRef,
			expectSkipContains: skipPullUpstreamDetectFailedMessage,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repoPath := tc.setupRepo(t)

			result, err := Update(context.Background(), repoPath, UpdateOptions{
				Prune:           true,
				AutoStash:       true,
				SubmoduleUpdate: true,
				DryRun:          false,
			})
			if err != nil {
				t.Fatalf("Update() error = %v", err)
			}

			if result == nil {
				t.Fatalf("Update() result is nil")
			}

			if !hasCommandContaining(result.Commands, " fetch --all --prune") {
				t.Fatalf("fetch コマンドが計画に含まれていません: %v", result.Commands)
			}

			if hasCommandContaining(result.Commands, " pull --rebase") {
				t.Fatalf("安全側スキップのはずなのに pull コマンドが計画に含まれています: %v", result.Commands)
			}

			if hasCommandContaining(result.Commands, " submodule update") {
				t.Fatalf("安全側スキップのはずなのに submodule update コマンドが計画に含まれています: %v", result.Commands)
			}

			if !hasMessageContaining(result.SkippedMessages, tc.expectSkipContains) {
				t.Fatalf("skipメッセージに %q が含まれていません: %v", tc.expectSkipContains, result.SkippedMessages)
			}
		})
	}
}

func TestAutoStashWithDirtyRepo(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name              string
		setupRepo         func(t *testing.T) string
		autoStash         bool
		expectPullCommand bool
		expectSkipContains string
	}{
		{
			name:              "AutoStash=true + DIRTY(tracked) → pull --autostash を計画する",
			setupRepo:         createRepoWithUpstreamAndDirtyTracked,
			autoStash:         true,
			expectPullCommand: true,
		},
		{
			name:              "AutoStash=true + DIRTY(untracked) → pull --autostash を計画する",
			setupRepo:         createRepoWithUpstreamAndDirtyUntracked,
			autoStash:         true,
			expectPullCommand: true,
		},
		{
			name:               "AutoStash=false + DIRTY(tracked) → スキップ（回帰）",
			setupRepo:          createRepoWithUpstreamAndDirtyTracked,
			autoStash:          false,
			expectPullCommand:  false,
			expectSkipContains: "未コミットの変更があるため pull/submodule をスキップ",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repoPath := tc.setupRepo(t)

			result, err := Update(context.Background(), repoPath, UpdateOptions{
				Prune:           true,
				AutoStash:       tc.autoStash,
				SubmoduleUpdate: false,
				DryRun:          true,
			})
			if err != nil {
				t.Fatalf("Update() error = %v", err)
			}

			if result == nil {
				t.Fatalf("Update() result is nil")
			}

			gotPull := hasCommandContaining(result.Commands, " pull --rebase --autostash")
			if gotPull != tc.expectPullCommand {
				t.Fatalf("pull --autostash コマンド有無 = %v, want %v, commands=%v, skipped=%v",
					gotPull, tc.expectPullCommand, result.Commands, result.SkippedMessages)
			}

			if tc.expectSkipContains != "" && !hasMessageContaining(result.SkippedMessages, tc.expectSkipContains) {
				t.Fatalf("skipメッセージに %q が含まれていません: %v", tc.expectSkipContains, result.SkippedMessages)
			}

			if tc.expectPullCommand && len(result.SkippedMessages) > 0 {
				t.Fatalf("AutoStash=true + DIRTY で pull 計画があるのに SkippedMessages が非空です: %v", result.SkippedMessages)
			}
		})
	}
}

func TestGetBehindCount(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		setupRepo func(t *testing.T) string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "同期済みリポジトリは 0 を返す",
			setupRepo: createRepoWithUpstream,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "リモートに新しいコミットがある場合は BEHIND 件数を返す",
			setupRepo: createRepoWithUpstreamAndRemoteAhead,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:      "upstream 未設定の場合はエラー",
			setupRepo: createLocalRepoWithoutUpstream,
			wantCount: 0,
			wantErr:   true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repoPath := tc.setupRepo(t)

			count, err := getBehindCount(context.Background(), repoPath)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("getBehindCount() エラーが期待されましたが nil でした")
				}
				return
			}

			if err != nil {
				t.Fatalf("getBehindCount() error = %v", err)
			}

			if count != tc.wantCount {
				t.Fatalf("getBehindCount() = %d, want %d", count, tc.wantCount)
			}
		})
	}
}

func createLocalRepoWithoutUpstream(t *testing.T) string {
	t.Helper()

	repoPath := t.TempDir()
	runGit(t, "", "init", repoPath)
	runGit(t, repoPath, "config", "user.email", "dsx-test@example.com")
	runGit(t, repoPath, "config", "user.name", "dsx-test")

	filePath := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(filePath, []byte("# local\n"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	runGit(t, repoPath, "add", "README.md")
	runGit(t, repoPath, "commit", "-m", "initial commit")

	return repoPath
}

func createBrokenWorktreeRepo(t *testing.T) string {
	t.Helper()

	repoPath := t.TempDir()

	// .git が壊れているワークツリーを模擬する（Discover の検出対象になり得るケース）。
	filePath := filepath.Join(repoPath, ".git")
	if err := os.WriteFile(filePath, []byte("gitdir: /path/to/nowhere\n"), 0o644); err != nil {
		t.Fatalf("failed to write .git file: %v", err)
	}

	return repoPath
}

func createRepoWithUpstream(t *testing.T) string {
	t.Helper()

	base := t.TempDir()
	remotePath := filepath.Join(base, "remote.git")
	sourcePath := filepath.Join(base, "source")
	workPath := filepath.Join(base, "work")

	runGit(t, "", "init", "--bare", remotePath)
	runGit(t, "", "clone", remotePath, sourcePath)
	runGit(t, sourcePath, "config", "user.email", "dsx-test@example.com")
	runGit(t, sourcePath, "config", "user.name", "dsx-test")

	filePath := filepath.Join(sourcePath, "README.md")
	if err := os.WriteFile(filePath, []byte("# upstream\n"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	runGit(t, sourcePath, "add", "README.md")
	runGit(t, sourcePath, "commit", "-m", "initial commit")
	runGit(t, sourcePath, "push", "-u", "origin", "HEAD")
	runGit(t, "", "clone", remotePath, workPath)
	runGit(t, workPath, "config", "user.email", "dsx-test@example.com")
	runGit(t, workPath, "config", "user.name", "dsx-test")

	return workPath
}

func createRepoWithUpstreamAndDirtyTracked(t *testing.T) string {
	t.Helper()

	repoPath := createRepoWithUpstream(t)

	filePath := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(filePath, []byte("# upstream\nmodified\n"), 0o644); err != nil {
		t.Fatalf("failed to modify file: %v", err)
	}

	return repoPath
}

func createRepoWithUpstreamAndDirtyUntracked(t *testing.T) string {
	t.Helper()

	repoPath := createRepoWithUpstream(t)

	filePath := filepath.Join(repoPath, "UNTRACKED.txt")
	if err := os.WriteFile(filePath, []byte("untracked\n"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	return repoPath
}

func createRepoWithUpstreamAndStash(t *testing.T) string {
	t.Helper()

	repoPath := createRepoWithUpstream(t)

	filePath := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(filePath, []byte("# upstream\nstash\n"), 0o644); err != nil {
		t.Fatalf("failed to modify file: %v", err)
	}

	runGit(t, repoPath, "stash", "push", "-m", "dsx-test")

	return repoPath
}

func createRepoWithUpstreamAndDetachedHEAD(t *testing.T) string {
	t.Helper()

	repoPath := createRepoWithUpstream(t)

	runGit(t, repoPath, "checkout", "--detach", "HEAD")

	return repoPath
}

func createRepoWithUpstreamAndLocalUpstreamRef(t *testing.T) string {
	t.Helper()

	repoPath := createRepoWithUpstream(t)

	branch, err := getCurrentBranchName(context.Background(), repoPath)
	if err != nil {
		t.Fatalf("failed to detect branch name: %v", err)
	}

	// upstream をローカルブランチに設定し、@{u} が "<remote>/<branch>" 形式にならない状態を模擬する。
	runGit(t, repoPath, "branch", "dsx-test-local-upstream-target")
	runGit(t, repoPath, "branch", "--set-upstream-to=dsx-test-local-upstream-target", branch)

	return repoPath
}

func createRepoWithUpstreamWithoutRemoteHead(t *testing.T) string {
	t.Helper()

	repoPath := createRepoWithUpstream(t)

	// Update() は先に fetch を実行するため、fetch 後も remote HEAD が復元されない状態を作る。
	// remote.origin.followRemoteHEAD=never により、fetch が refs/remotes/origin/HEAD を再生成しないようにする。
	runGit(t, repoPath, "config", "remote.origin.followRemoteHEAD", "never")

	runGit(t, repoPath, "symbolic-ref", "-d", "refs/remotes/origin/HEAD")

	return repoPath
}

func createRepoWithUpstreamAndRemoteAhead(t *testing.T) string {
	t.Helper()

	base := t.TempDir()
	remotePath := filepath.Join(base, "remote.git")
	sourcePath := filepath.Join(base, "source")
	workPath := filepath.Join(base, "work")

	runGit(t, "", "init", "--bare", remotePath)
	runGit(t, "", "clone", remotePath, sourcePath)
	runGit(t, sourcePath, "config", "user.email", "dsx-test@example.com")
	runGit(t, sourcePath, "config", "user.name", "dsx-test")

	filePath := filepath.Join(sourcePath, "README.md")
	if err := os.WriteFile(filePath, []byte("# upstream\n"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	runGit(t, sourcePath, "add", "README.md")
	runGit(t, sourcePath, "commit", "-m", "initial commit")
	runGit(t, sourcePath, "push", "-u", "origin", "HEAD")

	// work リポジトリを clone して fetch のみ行い、pull していない状態にする
	runGit(t, "", "clone", remotePath, workPath)
	runGit(t, workPath, "config", "user.email", "dsx-test@example.com")
	runGit(t, workPath, "config", "user.name", "dsx-test")

	// remote にさらにコミットを積んで work が BEHIND になる状態を作る
	if err := os.WriteFile(filePath, []byte("# upstream\nremote update\n"), 0o644); err != nil {
		t.Fatalf("failed to update file: %v", err)
	}

	runGit(t, sourcePath, "add", "README.md")
	runGit(t, sourcePath, "commit", "-m", "remote commit")
	runGit(t, sourcePath, "push", "origin", "HEAD")

	// work で fetch のみ実行（pull はしない）
	runGit(t, workPath, "fetch", "origin")

	return workPath
}

func createRepoWithNonDefaultUpstream(t *testing.T) string {
	t.Helper()

	repoPath := createRepoWithUpstream(t)

	runGit(t, repoPath, "checkout", "-b", "dsx-test-feature")

	filePath := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(filePath, []byte("# upstream\nfeature\n"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	runGit(t, repoPath, "add", "README.md")
	runGit(t, repoPath, "commit", "-m", "feature commit")
	runGit(t, repoPath, "push", "-u", "origin", "HEAD")

	return repoPath
}

func createRepoWithDifferentLocalBranchTrackingDefault(t *testing.T) string {
	t.Helper()

	repoPath := createRepoWithUpstream(t)
	defaultBranch := getOriginDefaultBranchName(t, repoPath)

	runGit(t, repoPath, "checkout", "-b", "dsx-test-local-default", "--track", "origin/"+defaultBranch)

	return repoPath
}

func getOriginDefaultBranchName(t *testing.T, repoPath string) string {
	t.Helper()

	defaultRef, err := getRemoteDefaultRef(context.Background(), repoPath, "origin")
	if err != nil {
		t.Fatalf("failed to detect origin default branch: %v", err)
	}

	_, branch, ok := strings.Cut(defaultRef, "/")
	if !ok || branch == "" {
		t.Fatalf("failed to parse origin default branch: %q", defaultRef)
	}

	return branch
}

func runGit(t *testing.T, repoPath string, args ...string) {
	t.Helper()

	commandArgs := args
	if repoPath != "" {
		commandArgs = append([]string{"-C", repoPath}, args...)
	}

	cmd := exec.CommandContext(context.Background(), "git", commandArgs...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v: %s", strings.Join(commandArgs, " "), err, strings.TrimSpace(string(output)))
	}
}

func hasCommandContaining(commands []string, needle string) bool {
	for _, command := range commands {
		if strings.Contains(command, needle) {
			return true
		}
	}

	return false
}

func hasMessageContaining(messages []string, needle string) bool {
	for _, message := range messages {
		if strings.Contains(message, needle) {
			return true
		}
	}

	return false
}
