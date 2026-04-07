package repo

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/scottlz0310/dsx/internal/testutil"
)

func TestDiscover(t *testing.T) {
	t.Run("正常系: root自身と直下のリポジトリを検出", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		createGitDir(t, root)

		repoA := filepath.Join(root, "repo-a")
		repoB := filepath.Join(root, "repo-b")
		fakeRepo := filepath.Join(root, "fake-repo")
		notRepo := filepath.Join(root, "docs")

		createGitDir(t, repoA)
		createGitFile(t, repoB)
		createInvalidGitFile(t, fakeRepo)
		mustMkdir(t, notRepo)

		got, err := Discover(root)
		if err != nil {
			t.Fatalf("Discover() error = %v", err)
		}

		want := []string{
			filepath.Clean(root),
			filepath.Clean(repoA),
			filepath.Clean(repoB),
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Discover() = %v, want %v", got, want)
		}
	})

	t.Run("正常系: チルダ展開", func(t *testing.T) {
		home := t.TempDir()
		testutil.SetTestHome(t, home)

		workspace := filepath.Join(home, "src")
		repoA := filepath.Join(workspace, "repo-a")

		mustMkdir(t, workspace)
		createGitDir(t, repoA)

		got, err := Discover("~/src")
		if err != nil {
			t.Fatalf("Discover() error = %v", err)
		}

		want := []string{filepath.Clean(repoA)}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Discover() = %v, want %v", got, want)
		}
	})

	t.Run("異常系: root が空", func(t *testing.T) {
		t.Parallel()

		if _, err := Discover(""); err == nil {
			t.Fatalf("Discover() error = nil, want error")
		}
	})

	t.Run("異常系: root が存在しない", func(t *testing.T) {
		t.Parallel()

		notFound := filepath.Join(t.TempDir(), "not-found")
		if _, err := Discover(notFound); err == nil {
			t.Fatalf("Discover() error = nil, want error")
		}
	})
}

func TestClassifyStatus(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		dirty       bool
		hasUpstream bool
		ahead       int
		want        Status
	}{
		{
			name:        "dirty は常にダーティ",
			dirty:       true,
			hasUpstream: true,
			ahead:       99,
			want:        StatusDirty,
		},
		{
			name:        "upstreamなし",
			dirty:       false,
			hasUpstream: false,
			ahead:       10,
			want:        StatusNoUpstream,
		},
		{
			name:        "aheadあり",
			dirty:       false,
			hasUpstream: true,
			ahead:       2,
			want:        StatusUnpushed,
		},
		{
			name:        "aheadゼロはクリーン",
			dirty:       false,
			hasUpstream: true,
			ahead:       0,
			want:        StatusClean,
		},
		{
			name:        "ahead負数もクリーン扱い",
			dirty:       false,
			hasUpstream: true,
			ahead:       -1,
			want:        StatusClean,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := classifyStatus(tc.dirty, tc.hasUpstream, tc.ahead)
			if got != tc.want {
				t.Fatalf("classifyStatus(%v, %v, %d) = %v, want %v", tc.dirty, tc.hasUpstream, tc.ahead, got, tc.want)
			}
		})
	}
}

func TestStatusLabel(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		in   Status
		want string
	}{
		{name: "clean", in: StatusClean, want: "クリーン"},
		{name: "dirty", in: StatusDirty, want: "ダーティ"},
		{name: "unpushed", in: StatusUnpushed, want: "未プッシュ"},
		{name: "no_upstream", in: StatusNoUpstream, want: "追跡なし"},
		{name: "unknown", in: Status("xxx"), want: "不明"},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := StatusLabel(tc.in)
			if got != tc.want {
				t.Fatalf("StatusLabel(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestHasGitMetadata(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		setup       func(t *testing.T, root string)
		expectFound bool
	}{
		{
			name: "gitディレクトリを検出",
			setup: func(t *testing.T, root string) {
				createGitDir(t, root)
			},
			expectFound: true,
		},
		{
			name: "worktree形式のgitファイルを検出",
			setup: func(t *testing.T, root string) {
				createGitFile(t, root)
			},
			expectFound: true,
		},
		{
			name: "無効なgitファイルは除外",
			setup: func(t *testing.T, root string) {
				createInvalidGitFile(t, root)
			},
			expectFound: false,
		},
		{
			name: "gitメタデータなしは除外",
			setup: func(t *testing.T, root string) {
				mustMkdir(t, root)
			},
			expectFound: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			tc.setup(t, root)

			got := hasGitMetadata(root)
			if got != tc.expectFound {
				t.Fatalf("hasGitMetadata() = %v, want %v", got, tc.expectFound)
			}
		})
	}
}

func TestGetAheadBehindCount(t *testing.T) {
	t.Run("upstream なしは hasUpstream=false と 0 件を返す", func(t *testing.T) {
		t.Parallel()

		repoPath := createLocalRepoWithoutUpstream(t)

		hasUpstream, ahead, behind, err := getAheadBehindCount(context.Background(), repoPath)
		if err != nil {
			t.Fatalf("getAheadBehindCount() error = %v", err)
		}

		if hasUpstream {
			t.Fatalf("hasUpstream = true, want false")
		}

		if ahead != 0 || behind != 0 {
			t.Fatalf("ahead/behind = %d/%d, want 0/0", ahead, behind)
		}
	})

	t.Run("behind がある場合に behind 件数を返す", func(t *testing.T) {
		t.Parallel()

		repoPath := createRepoWithUpstreamAndRemoteAhead(t)

		hasUpstream, ahead, behind, err := getAheadBehindCount(context.Background(), repoPath)
		if err != nil {
			t.Fatalf("getAheadBehindCount() error = %v", err)
		}

		if !hasUpstream {
			t.Fatalf("hasUpstream = false, want true")
		}

		if ahead != 0 || behind != 1 {
			t.Fatalf("ahead/behind = %d/%d, want 0/1", ahead, behind)
		}
	})

	t.Run("ahead がある場合に ahead 件数を返す", func(t *testing.T) {
		t.Parallel()

		repoPath := createRepoWithUpstreamAndLocalAhead(t)

		hasUpstream, ahead, behind, err := getAheadBehindCount(context.Background(), repoPath)
		if err != nil {
			t.Fatalf("getAheadBehindCount() error = %v", err)
		}

		if !hasUpstream {
			t.Fatalf("hasUpstream = false, want true")
		}

		if ahead != 1 || behind != 0 {
			t.Fatalf("ahead/behind = %d/%d, want 1/0", ahead, behind)
		}
	})

	t.Run("異常出力はパースエラーになる", func(t *testing.T) {
		prependFakeGitToPath(t, "invalid-output")

		_, _, _, err := getAheadBehindCount(context.Background(), t.TempDir())
		if err == nil {
			t.Fatalf("getAheadBehindCount() error = nil, want error")
		}

		if !strings.Contains(err.Error(), "ahead/behind 件数のパースに失敗") {
			t.Fatalf("error = %v, want parse error", err)
		}
	})

	t.Run("実行失敗時は stderr を含むエラーを返す", func(t *testing.T) {
		prependFakeGitToPath(t, "stderr-failure")

		_, _, _, err := getAheadBehindCount(context.Background(), t.TempDir())
		if err == nil {
			t.Fatalf("getAheadBehindCount() error = nil, want error")
		}

		errText := err.Error()
		if !strings.Contains(errText, "ahead/behind 件数の取得に失敗") {
			t.Fatalf("error = %v, want wrapped message", err)
		}

		if !strings.Contains(errText, "simulated git failure") {
			t.Fatalf("error = %v, want stderr message", err)
		}
	})
}

func createRepoWithUpstreamAndLocalAhead(t *testing.T) string {
	t.Helper()

	repoPath := createRepoWithUpstream(t)

	filePath := filepath.Join(repoPath, "LOCAL_AHEAD.txt")
	if err := os.WriteFile(filePath, []byte("local ahead\n"), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	runGit(t, repoPath, "add", "LOCAL_AHEAD.txt")
	runGit(t, repoPath, "commit", "-m", "local ahead commit")

	return repoPath
}

func prependFakeGitToPath(t *testing.T, mode string) {
	t.Helper()

	dir := t.TempDir()
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+oldPath)

	if runtime.GOOS == "windows" {
		var script string
		switch mode {
		case "invalid-output":
			script = "@echo off\r\necho invalid\r\nexit /b 0\r\n"
		case "stderr-failure":
			script = "@echo off\r\n>&2 echo simulated git failure\r\nexit /b 1\r\n"
		default:
			t.Fatalf("unknown mode: %s", mode)
		}

		filePath := filepath.Join(dir, "git.bat")
		if err := os.WriteFile(filePath, []byte(script), 0o755); err != nil {
			t.Fatalf("failed to write fake git script: %v", err)
		}

		return
	}

	var script string
	switch mode {
	case "invalid-output":
		script = "#!/bin/sh\nprintf '%s\\n' 'invalid'\nexit 0\n"
	case "stderr-failure":
		script = "#!/bin/sh\nprintf '%s\\n' 'simulated git failure' >&2\nexit 1\n"
	default:
		t.Fatalf("unknown mode: %s", mode)
	}

	filePath := filepath.Join(dir, "git")
	if err := os.WriteFile(filePath, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write fake git script: %v", err)
	}
}

func createGitDir(t *testing.T, root string) {
	t.Helper()

	mustMkdir(t, root)
	mustMkdir(t, filepath.Join(root, ".git"))
}

func createGitFile(t *testing.T, root string) {
	t.Helper()

	mustMkdir(t, root)

	gitFilePath := filepath.Join(root, ".git")
	if err := os.WriteFile(gitFilePath, []byte("gitdir: ../.git/worktrees/test\n"), 0o644); err != nil {
		t.Fatalf("failed to create .git file: %v", err)
	}
}

func createInvalidGitFile(t *testing.T, root string) {
	t.Helper()

	mustMkdir(t, root)

	gitFilePath := filepath.Join(root, ".git")
	if err := os.WriteFile(gitFilePath, []byte("this is not git metadata\n"), 0o644); err != nil {
		t.Fatalf("failed to create invalid .git file: %v", err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("failed to create directory %s: %v", path, err)
	}
}
