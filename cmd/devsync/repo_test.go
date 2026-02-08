package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestResolveRepoJobs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		configJobs int
		flagJobs   int
		want       int
	}{
		{
			name:       "フラグ優先",
			configJobs: 8,
			flagJobs:   3,
			want:       3,
		},
		{
			name:       "フラグ未指定なら設定値",
			configJobs: 6,
			flagJobs:   0,
			want:       6,
		},
		{
			name:       "設定が不正なら1",
			configJobs: 0,
			flagJobs:   0,
			want:       1,
		},
		{
			name:       "負数フラグは設定値にフォールバック",
			configJobs: 5,
			flagJobs:   -1,
			want:       5,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := resolveRepoJobs(tc.configJobs, tc.flagJobs)
			if got != tc.want {
				t.Fatalf("resolveRepoJobs(%d, %d) = %d, want %d", tc.configJobs, tc.flagJobs, got, tc.want)
			}
		})
	}
}

func TestResolveRepoSubmoduleUpdate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		configValue     bool
		enableOverride  bool
		disableOverride bool
		want            bool
		expectErr       bool
	}{
		{
			name:            "上書きなしは設定値を採用",
			configValue:     true,
			enableOverride:  false,
			disableOverride: false,
			want:            true,
			expectErr:       false,
		},
		{
			name:            "有効化上書き",
			configValue:     false,
			enableOverride:  true,
			disableOverride: false,
			want:            true,
			expectErr:       false,
		},
		{
			name:            "無効化上書き",
			configValue:     true,
			enableOverride:  false,
			disableOverride: true,
			want:            false,
			expectErr:       false,
		},
		{
			name:            "矛盾指定はエラー",
			configValue:     true,
			enableOverride:  true,
			disableOverride: true,
			want:            false,
			expectErr:       true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveRepoSubmoduleUpdate(tc.configValue, tc.enableOverride, tc.disableOverride)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("resolveRepoSubmoduleUpdate() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("resolveRepoSubmoduleUpdate() unexpected error: %v", err)
			}

			if got != tc.want {
				t.Fatalf("resolveRepoSubmoduleUpdate() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBuildRepoJobDisplayName(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		root     string
		repoPath string
		want     string
	}{
		{
			name:     "root直下は相対パス",
			root:     "/work/src",
			repoPath: "/work/src/devsync",
			want:     "devsync",
		},
		{
			name:     "ネストしたパスは相対表示",
			root:     "/work/src",
			repoPath: "/work/src/team-a/api",
			want:     "team-a/api",
		},
		{
			name:     "root自身はドット表示",
			root:     "/work/src",
			repoPath: "/work/src",
			want:     ".",
		},
		{
			name:     "root外はベース名表示",
			root:     "/work/src",
			repoPath: "/opt/repos/sample",
			want:     "sample",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := buildRepoJobDisplayName(tc.root, tc.repoPath)
			if got != tc.want {
				t.Fatalf("buildRepoJobDisplayName(%q, %q) = %q, want %q", tc.root, tc.repoPath, got, tc.want)
			}
		})
	}
}

func TestWrapRepoRootError(t *testing.T) {
	t.Parallel()

	notFoundErr := fmt.Errorf("ルートディレクトリにアクセスできません: %w", os.ErrNotExist)

	testCases := []struct {
		name           string
		err            error
		root           string
		rootOverridden bool
		configExists   bool
		configPath     string
		wantHint       bool
	}{
		{
			name:           "設定未初期化なら config init を案内",
			err:            notFoundErr,
			root:           "/tmp/src",
			rootOverridden: false,
			configExists:   false,
			configPath:     "/tmp/.config/devsync/config.yaml",
			wantHint:       true,
		},
		{
			name:           "設定ファイルがあれば案内しない",
			err:            notFoundErr,
			root:           "/tmp/src",
			rootOverridden: false,
			configExists:   true,
			configPath:     "/tmp/.config/devsync/config.yaml",
			wantHint:       false,
		},
		{
			name:           "root上書き時は案内しない",
			err:            notFoundErr,
			root:           "/tmp/src",
			rootOverridden: true,
			configExists:   false,
			configPath:     "/tmp/.config/devsync/config.yaml",
			wantHint:       false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wrapped := wrapRepoRootError(tc.err, tc.root, tc.rootOverridden, tc.configExists, tc.configPath)

			if hasHint := strings.Contains(wrapped.Error(), "devsync config init"); hasHint != tc.wantHint {
				t.Fatalf("wrapRepoRootError() hint = %v, want %v. got=%q", hasHint, tc.wantHint, wrapped.Error())
			}
		})
	}
}
