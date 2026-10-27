package snapshot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name, manager string
		tools         []Tool
		want          []Tool
		invalid       bool
	}{
		{name: "空一覧", manager: "uv", want: []Tool{}},
		{name: "Python の epoch 付きバージョン", manager: "uv", tools: []Tool{{Package: "tool", Version: "1!2.0"}}, want: []Tool{{Package: "tool", Version: "1!2.0"}}},
		{name: "順序と重複", manager: "npm", tools: []Tool{{Package: "z", Version: "2"}, {Package: "@a/tool", Version: "1"}, {Package: "z", Version: "2"}}, want: []Tool{{Package: "@a/tool", Version: "1"}, {Package: "z", Version: "2"}}},
		{name: "複数バージョン", manager: "gem", tools: []Tool{{Package: "rake", Version: "2"}, {Package: "rake", Version: "1"}}, want: []Tool{{Package: "rake", Version: "1"}, {Package: "rake", Version: "2"}}},
		{name: "Go パッケージ", manager: "go", tools: []Tool{{Package: "example.org/tools/v2/cmd/tool", Version: "v2.1.0-rc.1"}}, want: []Tool{{Package: "example.org/tools/v2/cmd/tool", Version: "v2.1.0-rc.1"}}},
		{name: "winget source", manager: "winget", tools: []Tool{{Package: "Git.Git", Source: "winget"}}, want: []Tool{{Package: "Git.Git", Source: "winget"}}},
		{name: "winget ID のプラス記号", manager: "winget", tools: []Tool{{Package: "Microsoft.VCRedist.2015+.x64", Source: "winget"}}, want: []Tool{{Package: "Microsoft.VCRedist.2015+.x64", Source: "winget"}}},
		{name: "未知マネージャ", manager: "unknown", invalid: true},
		{name: "空識別子", manager: "uv", tools: []Tool{{Version: "1"}}, invalid: true},
		{name: "認証 URL", manager: "npm", tools: []Tool{{Package: "https://user:secret@example.org/tool"}}, invalid: true},
		{name: "Unix パス", manager: "go", tools: []Tool{{Package: "/home/user/tool"}}, invalid: true},
		{name: "Windows パス", manager: "go", tools: []Tool{{Package: `C:\Users\user\tool`}}, invalid: true},
		{name: "相対パス", manager: "go", tools: []Tool{{Package: "../user/tool"}}, invalid: true},
		{name: "バージョンに URL", manager: "npm", tools: []Tool{{Package: "tool", Version: "file:/home/user/tool"}}, invalid: true},
		{name: "バージョンに改行", manager: "uv", tools: []Tool{{Package: "tool", Version: "1\nsecret"}}, invalid: true},
		{name: "source に URL", manager: "winget", tools: []Tool{{Package: "Git.Git", Source: "https://user:secret@example.org"}}, invalid: true},
		{name: "source の対象外", manager: "gem", tools: []Tool{{Package: "rake", Source: "rubygems"}}, invalid: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Normalize(tc.manager, tc.tools)
			if tc.invalid {
				require.Error(t, err)
				require.Nil(t, got)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestMarshalDeterministic(t *testing.T) {
	manifest := &Manifest{Version: 1, Tools: map[string][]Tool{
		"winget": {{Package: "Git.Git", Version: "2.51.0", Source: "winget"}},
		"uv":     {{Package: "z", Version: "2"}, {Package: "a", Version: "1"}},
		"go":     nil,
	}}

	want := "version: 1\ntools:\n    go: []\n    uv:\n        - package: a\n          version: \"1\"\n        - package: z\n          version: \"2\"\n    winget:\n        - package: Git.Git\n          version: 2.51.0\n          source: winget\n"
	for range 10 {
		data, err := Marshal(manifest)
		require.NoError(t, err)
		require.Equal(t, want, string(data))
	}

	manifest.Version = 0
	_, err := Marshal(manifest)
	require.Error(t, err)
}

func TestSave(t *testing.T) {
	tests := []struct {
		name                                               string
		existing, force, invalid, missingParent, directory bool
		wantError                                          bool
	}{
		{name: "新規保存"},
		{name: "既存保護", existing: true, wantError: true},
		{name: "明示上書き", existing: true, force: true},
		{name: "force で新規作成", force: true},
		{name: "検証失敗で既存保護", existing: true, force: true, invalid: true, wantError: true},
		{name: "親ディレクトリなし", missingParent: true, wantError: true},
		{name: "ディレクトリ指定", directory: true, force: true, wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()

			path := filepath.Join(dir, "tools.yaml")
			if tc.missingParent {
				path = filepath.Join(dir, "missing", "tools.yaml")
			}

			if tc.directory {
				path = dir
			}

			if tc.existing {
				require.NoError(t, os.WriteFile(path, []byte("既存の内容"), 0o600))
			}

			manifest := &Manifest{Version: 1, Tools: map[string][]Tool{"uv": {{Package: "ruff", Version: "1"}}}}
			if tc.invalid {
				manifest.Version = -1
			}

			err := Save(path, manifest, tc.force)
			if tc.wantError {
				require.Error(t, err)

				if tc.existing {
					data, readErr := os.ReadFile(path)
					require.NoError(t, readErr)
					require.Equal(t, "既存の内容", string(data))
				}
			} else {
				require.NoError(t, err)

				data, readErr := os.ReadFile(path)
				require.NoError(t, readErr)
				require.Contains(t, string(data), "package: ruff")
			}

			files, err := filepath.Glob(filepath.Join(dir, ".dsx-snapshot-*"))
			require.NoError(t, err)
			require.Empty(t, files)
		})
	}
}

func TestCheckOutputEmpty(t *testing.T) {
	require.Error(t, CheckOutput("", false))
}
