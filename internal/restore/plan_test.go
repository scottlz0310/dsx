package restore

import (
	"context"
	"errors"
	"testing"

	"github.com/scottlz0310/dsx/internal/snapshot"
	"github.com/stretchr/testify/require"
)

func TestBuild(t *testing.T) {
	for _, tc := range []struct {
		name, manager                                                        string
		wanted, installed                                                    []snapshot.Tool
		unavailable, incompatible, unsupported, inventoryError, resolveError bool
		want                                                                 []State
		wantQueries                                                          int
	}{
		{name: "不足ツール", manager: "uv", wanted: []snapshot.Tool{{Package: "ruff", Version: "1.0"}}, want: []State{Add}, wantQueries: 1},
		{name: "既存の一致", manager: "uv", wanted: []snapshot.Tool{{Package: "ruff", Version: "1.0"}}, installed: []snapshot.Tool{{Package: "ruff", Version: "1.0"}}, want: []State{Existing}},
		{name: "版未固定なら既存を維持", manager: "uv", wanted: []snapshot.Tool{{Package: "ruff"}}, installed: []snapshot.Tool{{Package: "ruff", Version: "1.0"}}, want: []State{Existing}},
		{name: "Python 名の正規化", manager: "pipx", wanted: []snapshot.Tool{{Package: "My_Package.Name"}}, installed: []snapshot.Tool{{Package: "my-package-name", Version: "1.0"}}, want: []State{Existing}},
		{name: "winget ID の大小文字", manager: "winget", wanted: []snapshot.Tool{{Package: "git.git", Source: "winget"}}, installed: []snapshot.Tool{{Package: "Git.Git", Source: "winget"}}, want: []State{Existing}},
		{name: "既存版の変更を拒否", manager: "uv", wanted: []snapshot.Tool{{Package: "ruff", Version: "1.0"}}, installed: []snapshot.Tool{{Package: "ruff", Version: "2.0"}}, want: []State{Unresolved}},
		{name: "source 違いを拒否", manager: "winget", wanted: []snapshot.Tool{{Package: "Git.Git", Source: "private"}}, installed: []snapshot.Tool{{Package: "Git.Git", Source: "winget"}}, want: []State{Unresolved}},
		{name: "同じツールの複数版を拒否", manager: "uv", wanted: []snapshot.Tool{{Package: "ruff", Version: "1.0"}, {Package: "ruff", Version: "2.0"}}, want: []State{Unresolved, Unresolved}},
		{name: "gem の複数版は併存", manager: "gem", wanted: []snapshot.Tool{{Package: "rake", Version: "1.0"}, {Package: "rake", Version: "2.0"}}, installed: []snapshot.Tool{{Package: "rake", Version: "2.0"}}, want: []State{Add, Existing}, wantQueries: 1},
		{name: "重複と順序", manager: "uv", wanted: []snapshot.Tool{{Package: "z"}, {Package: "a"}, {Package: "z"}}, want: []State{Add, Add}, wantQueries: 2},
		{name: "空一覧は照会しない", manager: "uv", wanted: []snapshot.Tool{}, unavailable: true, want: []State{}},
		{name: "未導入", manager: "uv", wanted: []snapshot.Tool{{Package: "ruff"}}, unavailable: true, want: []State{Unresolved}},
		{name: "OS 非互換", manager: "winget", wanted: []snapshot.Tool{{Package: "Git.Git"}}, incompatible: true, want: []State{Unresolved}},
		{name: "未対応", manager: "uv", wanted: []snapshot.Tool{{Package: "ruff"}}, unsupported: true, want: []State{Unresolved}},
		{name: "一覧取得失敗", manager: "uv", wanted: []snapshot.Tool{{Package: "ruff"}}, inventoryError: true, want: []State{Unresolved}},
		{name: "提供版の照会失敗", manager: "uv", wanted: []snapshot.Tool{{Package: "ruff"}}, resolveError: true, want: []State{Unresolved}, wantQueries: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			queries, inventoryCalls := 0, 0
			manager := Manager{
				Available:  func() bool { return !tc.unavailable },
				Compatible: func(string) bool { return !tc.incompatible },
				Installed: func(context.Context) ([]snapshot.Tool, error) {
					inventoryCalls++

					if tc.inventoryError {
						return nil, errors.New("一覧取得失敗")
					}

					return tc.installed, nil
				},
				Resolve: func(_ context.Context, tool snapshot.Tool) (Command, error) {
					queries++

					if tc.resolveError {
						return Command{}, errors.New("照会失敗")
					}

					return Command{Args: []string{"予定コマンド", tool.Package}, Version: "1.0"}, nil
				},
			}

			managers := map[string]Manager{tc.manager: manager}
			if tc.unsupported {
				managers = map[string]Manager{}
			}

			manifest := &snapshot.Manifest{Version: 1, Tools: map[string][]snapshot.Tool{tc.manager: tc.wanted}}
			plan, err := Build(context.Background(), manifest, managers, "linux")
			require.NoError(t, err)

			states := make([]State, 0, len(plan.Entries))

			unresolved := 0
			for i := range plan.Entries {
				states = append(states, plan.Entries[i].State)
				if plan.Entries[i].State == Unresolved {
					unresolved++
				}
			}

			require.Equal(t, tc.want, states)
			require.Equal(t, tc.wantQueries, queries)
			require.Equal(t, unresolved, plan.UnresolvedCount())

			if tc.unavailable || tc.incompatible || tc.unsupported {
				require.Zero(t, inventoryCalls)
			}
		})
	}
}

func TestBuildFailures(t *testing.T) {
	for _, mode := range []string{"不正な manifest", "開始前のキャンセル", "照会中のキャンセル"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			manifest := &snapshot.Manifest{Version: 1, Tools: map[string][]snapshot.Tool{"uv": {{Package: "ruff"}}}}
			if mode == "不正な manifest" {
				manifest.Version = 0
			}

			if mode == "開始前のキャンセル" {
				cancel()
			}

			manager := Manager{Available: func() bool { return true }, Compatible: func(string) bool { return true }, Installed: func(context.Context) ([]snapshot.Tool, error) { return nil, nil }, Resolve: func(context.Context, snapshot.Tool) (Command, error) { cancel(); return Command{}, context.Canceled }}
			_, err := Build(ctx, manifest, map[string]Manager{"uv": manager}, "linux")
			require.Error(t, err)

			if mode != "不正な manifest" {
				require.ErrorIs(t, err, context.Canceled)
			}
		})
	}
}
