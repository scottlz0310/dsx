package repo

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
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
		t.Setenv("HOME", home)

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
