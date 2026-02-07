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
			name:               "upstreamなしはpull計画を除外",
			setupRepo:          createLocalRepoWithoutUpstream,
			expectPullCommand:  false,
			expectSkipContains: "upstream が未設定",
		},
		{
			name: "upstream確認失敗時はpull計画を除外して継続",
			setupRepo: func(t *testing.T) string {
				return t.TempDir()
			},
			expectPullCommand:  false,
			expectSkipContains: "upstream の確認に失敗",
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

func createLocalRepoWithoutUpstream(t *testing.T) string {
	t.Helper()

	repoPath := t.TempDir()
	runGit(t, "", "init", repoPath)
	runGit(t, repoPath, "config", "user.email", "devsync-test@example.com")
	runGit(t, repoPath, "config", "user.name", "devsync-test")

	filePath := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(filePath, []byte("# local\n"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	runGit(t, repoPath, "add", "README.md")
	runGit(t, repoPath, "commit", "-m", "initial commit")

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
	runGit(t, sourcePath, "config", "user.email", "devsync-test@example.com")
	runGit(t, sourcePath, "config", "user.name", "devsync-test")

	filePath := filepath.Join(sourcePath, "README.md")
	if err := os.WriteFile(filePath, []byte("# upstream\n"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	runGit(t, sourcePath, "add", "README.md")
	runGit(t, sourcePath, "commit", "-m", "initial commit")
	runGit(t, sourcePath, "push", "-u", "origin", "HEAD")
	runGit(t, "", "clone", remotePath, workPath)

	return workPath
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
