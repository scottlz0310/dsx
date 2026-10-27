package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/scottlz0310/dsx/internal/snapshot"
	"github.com/stretchr/testify/require"
)

func TestSnapshotProviderCommands(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		output   string
		provider func(snapshotCommandRunner) SnapshotProvider
	}{
		{"npm", []string{"list", "--global", "--depth=0", "--json"}, `{"dependencies":{"tool":{"version":"1"}}}`, func(run snapshotCommandRunner) SnapshotProvider { return &NpmUpdater{snapshotRun: run} }},
		{"pnpm", []string{"list", "--global", "--depth=0", "--json"}, `[{"dependencies":{"tool":{"version":"1"}}}]`, func(run snapshotCommandRunner) SnapshotProvider { return &PnpmUpdater{snapshotRun: run} }},
		{"pipx", []string{"list", "--json"}, `{"venvs":{"tool":{"metadata":{"main_package":{"package":"tool","package_version":"1"}}}}}`, func(run snapshotCommandRunner) SnapshotProvider { return &PipxUpdater{snapshotRun: run} }},
		{"uv", []string{"tool", "list", "--color", "never"}, "tool v1\n- tool\n", func(run snapshotCommandRunner) SnapshotProvider { return &UVUpdater{snapshotRun: run} }},
		{"cargo", []string{"install", "--list"}, "tool v1:\n    tool\n", func(run snapshotCommandRunner) SnapshotProvider { return &CargoUpdater{snapshotRun: run} }},
		{"gem", []string{"list", "--local", "--all", "--no-verbose"}, "tool (1)\n", func(run snapshotCommandRunner) SnapshotProvider { return &GemUpdater{snapshotRun: run} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, mode := range []string{"正常", "コマンド失敗", "解析失敗"} {
				t.Run(mode, func(t *testing.T) {
					failure := errors.New("一覧取得エラー")
					provider := tc.provider(func(_ context.Context, name string, args ...string) ([]byte, error) {
						require.Equal(t, tc.name, name)
						require.Equal(t, tc.args, args)

						if mode == "コマンド失敗" {
							return []byte(tc.output), failure
						}

						if mode == "解析失敗" {
							return []byte("不正な出力"), nil
						}

						return []byte(tc.output), nil
					})

					tools, err := provider.Snapshot(context.Background())
					switch mode {
					case "コマンド失敗":
						require.ErrorIs(t, err, failure)
						require.Nil(t, tools)
					case "解析失敗":
						require.Error(t, err)
						require.Nil(t, tools)
					default:
						require.NoError(t, err)
						require.Equal(t, []snapshot.Tool{{Package: "tool", Version: "1"}}, tools)
					}
				})
			}
		})
	}
}

func TestWingetSnapshotExport(t *testing.T) {
	for _, mode := range []string{"正常", "コマンド失敗", "ファイル未生成", "解析失敗"} {
		t.Run(mode, func(t *testing.T) {
			var exportPath string

			failure := errors.New("export 失敗")
			w := &WingetUpdater{snapshotRun: func(_ context.Context, name string, args ...string) ([]byte, error) {
				require.Equal(t, "winget", name)
				require.Len(t, args, 6)
				require.Equal(t, []string{"export", "--output"}, args[:2])
				require.Equal(t, []string{"--include-versions", "--disable-interactivity", "--accept-source-agreements"}, args[3:])
				exportPath = args[2]

				if mode == "コマンド失敗" {
					return nil, failure
				}

				if mode == "ファイル未生成" {
					return nil, nil
				}

				data := `{"Sources":[{"SourceDetails":{"Name":"winget"},"Packages":[{"PackageIdentifier":"Git.Git","Version":"2"}]}]}`
				if mode == "解析失敗" {
					data = `{}`
				}

				require.NoError(t, os.WriteFile(exportPath, []byte(data), 0o600))

				return nil, nil
			}}

			tools, err := w.Snapshot(context.Background())
			if mode == "正常" {
				require.NoError(t, err)
				require.Equal(t, []snapshot.Tool{{Package: "Git.Git", Version: "2", Source: "winget"}}, tools)
			} else {
				require.Error(t, err)

				if mode == "コマンド失敗" {
					require.ErrorIs(t, err, failure)
				}
			}

			_, statErr := os.Stat(filepath.Dir(exportPath))
			require.ErrorIs(t, statErr, os.ErrNotExist)
		})
	}
}

func TestGoSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result *DiscoverResult
		err    error
		want   []snapshot.Tool
	}{
		{"検出結果からパスを除外", &DiscoverResult{Detected: []GoBinaryInfo{{BinaryPath: "/home/user/go/bin/tool", PackagePath: "example.org/tool", InstalledVersion: "v1.0.0"}, {PackagePath: "example.org/dev", InstalledVersion: "(devel)"}}}, nil, []snapshot.Tool{{Package: "example.org/dev"}, {Package: "example.org/tool", Version: "v1.0.0"}}},
		{"未導入ディレクトリ", nil, os.ErrNotExist, []snapshot.Tool{}},
		{"読み取り失敗", nil, os.ErrPermission, nil},
		{"キャンセル", nil, context.Canceled, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &GoUpdater{discoverGoBinaries: func(context.Context) (*DiscoverResult, error) { return tc.result, tc.err }}

			tools, err := g.Snapshot(context.Background())
			if tc.want == nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, tools)
			}
		})
	}
}

type snapshotFakeUpdater struct {
	mockUpdater
	tools []snapshot.Tool
	err   error
	calls int
}

func (f *snapshotFakeUpdater) Snapshot(context.Context) ([]snapshot.Tool, error) {
	f.calls++
	return f.tools, f.err
}

func TestCollectSnapshot(t *testing.T) {
	for _, mode := range []string{"正常", "取得失敗", "不正な識別子", "キャンセル", "期限切れ", "対応対象なし"} {
		t.Run(mode, func(t *testing.T) {
			uv := &snapshotFakeUpdater{mockUpdater: mockUpdater{name: "uv", available: true}, tools: []snapshot.Tool{{Package: "ruff", Version: "1"}}}
			npm := &snapshotFakeUpdater{mockUpdater: mockUpdater{name: "npm", available: true}, tools: []snapshot.Tool{}}
			unavailable := &snapshotFakeUpdater{mockUpdater: mockUpdater{name: "winget"}}
			managers := []Updater{uv, unavailable, &mockUpdater{name: "apt", available: true}, npm}
			ctx := context.Background()

			failure := errors.New("一覧取得エラー")
			switch mode {
			case "取得失敗":
				uv.err = failure
			case "不正な識別子":
				uv.tools[0].Package = "/home/user/secret"
			case "キャンセル":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()

				ctx = cancelled
			case "期限切れ":
				expired, cancel := context.WithTimeout(ctx, -1)
				defer cancel()

				ctx = expired
			case "対応対象なし":
				managers = []Updater{unavailable}
			}

			manifest, statuses, err := CollectSnapshot(ctx, managers)

			require.Zero(t, unavailable.calls)

			if mode != "正常" {
				require.Error(t, err)
				require.Nil(t, manifest)

				if mode == "取得失敗" {
					require.ErrorIs(t, err, failure)
					require.Equal(t, 1, npm.calls)
				}

				if ctx.Err() != nil {
					require.ErrorIs(t, err, ctx.Err())
				}

				return
			}

			require.NoError(t, err)
			require.Equal(t, map[string][]snapshot.Tool{"npm": {}, "uv": {{Package: "ruff", Version: "1"}}}, manifest.Tools)
			require.Equal(t, []SnapshotStatus{{Manager: "apt", Message: "スナップショット未対応"}, {Manager: "npm", Message: "0 件を取得"}, {Manager: "uv", Message: "1 件を取得"}, {Manager: "winget", Message: "利用不可（未導入または対象 OS 外）"}}, statuses)
			require.Equal(t, "uv", managers[0].Name())
		})
	}
}

func TestCollectSnapshotUnpinned(t *testing.T) {
	manager := &snapshotFakeUpdater{mockUpdater: mockUpdater{name: "winget", available: true}, tools: []snapshot.Tool{{Package: "Approx.App", Source: "winget"}}}
	manifest, statuses, err := CollectSnapshot(context.Background(), []Updater{manager})
	require.NoError(t, err)
	require.Empty(t, manifest.Tools["winget"][0].Version)
	require.Contains(t, statuses[0].Message, "1 件は正確な版が不明なため version を省略")
}
