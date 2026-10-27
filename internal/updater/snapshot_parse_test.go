package updater

import (
	"testing"

	"github.com/scottlz0310/dsx/internal/snapshot"
	"github.com/stretchr/testify/require"
)

func TestSnapshotParsers(t *testing.T) {
	tests := []struct {
		name, manager, input string
		parse                func([]byte) ([]snapshot.Tool, error)
		want                 []snapshot.Tool
		invalid              bool
	}{
		{name: "npm 直接依存のみ", manager: "npm", parse: parseNpmSnapshot, input: `{"name":"user","path":"/home/user","dependencies":{"@scope/tool":{"version":"1.2.3","resolved":"https://user:secret@example.org","dependencies":{"transitive":{"version":"1"}}}}}`, want: []snapshot.Tool{{Package: "@scope/tool", Version: "1.2.3"}}},
		{name: "npm 空", manager: "npm", parse: parseNpmSnapshot, input: `{}`, want: []snapshot.Tool{}},
		{name: "npm 不正 JSON", parse: parseNpmSnapshot, input: `{`, invalid: true},
		{name: "npm null", parse: parseNpmSnapshot, input: `null`, invalid: true},
		{name: "npm エラー結果", parse: parseNpmSnapshot, input: `{"error":{"code":"ERROR"}}`, invalid: true},
		{name: "npm バージョンなし", parse: parseNpmSnapshot, input: `{"dependencies":{"tool":{"path":"/home/user/tool"}}}`, invalid: true},
		{name: "pnpm 直接依存", manager: "pnpm", parse: parsePnpmSnapshot, input: `[{"path":"C:\\Users\\user","dependencies":{"tool":{"version":"2.0.0","path":"/local/tool"}}}]`, want: []snapshot.Tool{{Package: "tool", Version: "2.0.0"}}},
		{name: "pnpm 空", manager: "pnpm", parse: parsePnpmSnapshot, input: `[]`, want: []snapshot.Tool{}},
		{name: "pnpm 形式違い", parse: parsePnpmSnapshot, input: `{}`, invalid: true},
		{name: "pnpm null", parse: parsePnpmSnapshot, input: `null`, invalid: true},
		{name: "pnpm 配列要素が null", parse: parsePnpmSnapshot, input: `[null]`, invalid: true},
		{name: "pipx suffix を識別子にしない", manager: "pipx", parse: parsePipxSnapshot, input: `{"venvs":{"ruff-suffix":{"metadata":{"main_package":{"package":"ruff","package_version":"0.12.0","package_or_url":"https://user:secret@example.org"},"injected_packages":{"other":{"package_version":"2"}}}}}}`, want: []snapshot.Tool{{Package: "ruff", Version: "0.12.0"}}},
		{name: "pipx 空", manager: "pipx", parse: parsePipxSnapshot, input: `{"venvs":{}}`, want: []snapshot.Tool{}},
		{name: "pipx 構造欠落", parse: parsePipxSnapshot, input: `{}`, invalid: true},
		{name: "pipx 主パッケージ欠落", parse: parsePipxSnapshot, input: `{"venvs":{"ruff":{}}}`, invalid: true},
		{name: "winget ID と source", manager: "winget", parse: parseWingetSnapshot, input: "\ufeff" + `{"CreationDate":"2026-10-27","Sources":[{"SourceDetails":{"Name":"winget","Argument":"https://user:secret@example.org"},"Packages":[{"PackageIdentifier":"Git.Git","Version":"2.51.0"},{"PackageIdentifier":"Unknown.Version"}]}]}`, want: []snapshot.Tool{{Package: "Git.Git", Version: "2.51.0", Source: "winget"}, {Package: "Unknown.Version", Source: "winget"}}},
		{name: "winget 空", manager: "winget", parse: parseWingetSnapshot, input: `{"Sources":[]}`, want: []snapshot.Tool{}},
		{name: "winget 概算版は固定しない", manager: "winget", parse: parseWingetSnapshot, input: `{"Sources":[{"SourceDetails":{"Name":"winget"},"Packages":[{"PackageIdentifier":"Approx.App","Version":"> 1.2.3"},{"PackageIdentifier":"Unknown.App","Version":"Unknown"},{"PackageIdentifier":"Unknown.Japanese","Version":"不明"}]}]}`, want: []snapshot.Tool{{Package: "Approx.App", Source: "winget"}, {Package: "Unknown.App", Source: "winget"}, {Package: "Unknown.Japanese", Source: "winget"}}},
		{name: "winget 構造欠落", parse: parseWingetSnapshot, input: `{}`, invalid: true},
		{name: "winget source 欠落", parse: parseWingetSnapshot, input: `{"Sources":[{"Packages":[]}]}`, invalid: true},
		{name: "winget ID 欠落", parse: parseWingetSnapshot, input: `{"Sources":[{"SourceDetails":{"Name":"winget"},"Packages":[{}]}]}`, invalid: true},
		{name: "uv 実行ファイルと URL は保存しない", manager: "uv", parse: parseUVSnapshot, input: "ruff v0.12.0 (from file:///home/user/tool)\r\n- ruff\r\n", want: []snapshot.Tool{{Package: "ruff", Version: "0.12.0"}}},
		{name: "uv 空", manager: "uv", parse: parseUVSnapshot, want: []snapshot.Tool{}},
		{name: "uv 不正行", parse: parseUVSnapshot, input: "unexpected output", invalid: true},
		{name: "uv 空バージョン", parse: parseUVSnapshot, input: "ruff v", invalid: true},
		{name: "cargo パスとバイナリを除外", manager: "cargo", parse: parseCargoSnapshot, input: "tool v1.2.3 (/home/user/tool):\r\n    tool\r\n", want: []snapshot.Tool{{Package: "tool", Version: "1.2.3"}}},
		{name: "cargo 空", manager: "cargo", parse: parseCargoSnapshot, want: []snapshot.Tool{}},
		{name: "cargo 不正行", parse: parseCargoSnapshot, input: "tool unknown:", invalid: true},
		{name: "gem 全バージョン", manager: "gem", parse: parseGemSnapshot, input: "rake (13.2.1, default: 13.1.0)\r\njson (2.8.0 ruby, 2.8.0 x64-mingw-ucrt)\n", want: []snapshot.Tool{{Package: "json", Version: "2.8.0"}, {Package: "rake", Version: "13.1.0"}, {Package: "rake", Version: "13.2.1"}}},
		{name: "gem 空", manager: "gem", parse: parseGemSnapshot, want: []snapshot.Tool{}},
		{name: "gem ヘッダー", manager: "gem", parse: parseGemSnapshot, input: "\n*** LOCAL GEMS ***\n\nrake (13.0.0)\n", want: []snapshot.Tool{{Package: "rake", Version: "13.0.0"}}},
		{name: "gem 空バージョン", parse: parseGemSnapshot, input: "rake ()", invalid: true},
		{name: "gem 不正行", parse: parseGemSnapshot, input: "rake 1.0", invalid: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tools, err := tc.parse([]byte(tc.input))
			if tc.invalid {
				require.Error(t, err)
				require.Nil(t, tools)
				return
			}

			require.NoError(t, err)
			got, err := snapshot.Normalize(tc.manager, tools)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
