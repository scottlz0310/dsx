package updater

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/scottlz0310/dsx/internal/snapshot"
	"github.com/stretchr/testify/require"
)

type restoreHTTPFake struct {
	t        *testing.T
	routes   map[string]string
	status   int
	err      error
	requests []string
}

func (f *restoreHTTPFake) Do(req *http.Request) (*http.Response, error) {
	f.t.Helper()
	require.Equal(f.t, http.MethodGet, req.Method)
	require.Equal(f.t, "https", req.URL.Scheme)

	f.requests = append(f.requests, req.URL.String())
	if f.err != nil {
		return nil, f.err
	}

	status := f.status

	body, found := f.routes[req.URL.String()]
	if status == 0 {
		status = http.StatusOK
		if !found {
			status = http.StatusNotFound
		}
	}

	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestRestoreResolver(t *testing.T) {
	for _, tc := range []struct {
		name, manager, pkg, requested, output string
		routes                                map[string]string
		want                                  []string
		invalid                               bool
	}{
		{name: "npm の指定版", manager: "npm", pkg: "@scope/tool", requested: "1.2.3", output: `{"version":"1.2.3","os":["linux"],"cpu":["x64"]}`, want: []string{"npm", "install", "--global", "@scope/tool@1.2.3"}},
		{name: "pnpm の最新提供版", manager: "pnpm", pkg: "tool", output: `{"version":"2.0.0"}`, want: []string{"pnpm", "install", "--global", "tool@2.0.0"}},
		{name: "npm の版のみの JSON 文字列", manager: "npm", pkg: "semver", requested: "7.7.2", output: `"7.7.2"`, want: []string{"npm", "install", "--global", "semver@7.7.2"}},
		{name: "npm 実機の文字列配列", manager: "npm", pkg: "semver", requested: "7.7.2", output: `["7.7.2"]`, want: []string{"npm", "install", "--global", "semver@7.7.2"}},
		{name: "npm 実機のオブジェクト配列", manager: "npm", pkg: "@esbuild/linux-x64", requested: "0.25.0", output: `[{"version":"0.25.0","os":["linux"],"cpu":["x64"]}]`, want: []string{"npm", "install", "--global", "@esbuild/linux-x64@0.25.0"}},
		{name: "npm の複数提供版を拒否", manager: "npm", pkg: "tool", output: `["1.0.0","2.0.0"]`, invalid: true},
		{name: "npm の空配列を拒否", manager: "npm", pkg: "tool", output: `[]`, invalid: true},
		{name: "npm の OS 非互換", manager: "npm", pkg: "tool", output: `{"version":"1.0.0","os":["darwin"]}`, invalid: true},
		{name: "npm の CPU 非互換", manager: "npm", pkg: "tool", output: `{"version":"1.0.0","cpu":["arm64"]}`, invalid: true},
		{name: "npm の指定版を変更しない", manager: "npm", pkg: "tool", requested: "1.0.0", output: `{"version":"2.0.0"}`, invalid: true},
		{name: "npm の不正な応答", manager: "npm", pkg: "tool", output: `null`, invalid: true},
		{name: "npm の版範囲を拒否", manager: "npm", pkg: "tool", requested: "1.0.0 - 2.0.0", invalid: true},
		{name: "uv の指定版", manager: "uv", pkg: "ruff", requested: "1.0", routes: map[string]string{"https://pypi.org/pypi/ruff/1.0/json": `{"info":{"version":"1.0"},"urls":[{"yanked":false}]}`}, want: []string{"uv", "tool", "install", "--index-url", "https://pypi.org/simple", "ruff==1.0"}},
		{name: "pipx の epoch 付き版", manager: "pipx", pkg: "tool", requested: "1!2.0", routes: map[string]string{"https://pypi.org/pypi/tool/1%212.0/json": `{"info":{"version":"1!2.0"},"urls":[{"yanked":false}]}`}, want: []string{"pipx", "install", "--index-url", "https://pypi.org/simple", "tool==1!2.0"}},
		{name: "PyPI の版撤回", manager: "uv", pkg: "tool", routes: map[string]string{"https://pypi.org/pypi/tool/json": `{"info":{"version":"1.0"},"urls":[{"yanked":true}]}`}, invalid: true},
		{name: "PyPI に配布なし", manager: "uv", pkg: "tool", routes: map[string]string{"https://pypi.org/pypi/tool/json": `{"info":{"version":"1.0"},"urls":[]}`}, invalid: true},
		{name: "PyPI にパッケージなし", manager: "pipx", pkg: "tool", invalid: true},
		{name: "Go のモジュール接頭辞と大文字", manager: "go", pkg: "example.org/Tool/cmd/tool", requested: "v1.0.0", routes: map[string]string{"https://proxy.golang.org/example.org/!tool/@v/v1.0.0.info": `{"Version":"v1.0.0"}`}, want: []string{"go", "install", "example.org/Tool/cmd/tool@v1.0.0"}},
		{name: "Go の latest 解決", manager: "go", pkg: "example.org/tool", routes: map[string]string{"https://proxy.golang.org/example.org/tool/@latest": `{"Version":"v2.0.0"}`}, want: []string{"go", "install", "example.org/tool@v2.0.0"}},
		{name: "Go の指定版不在", manager: "go", pkg: "example.org/tool", requested: "v1.0.0", invalid: true},
		{name: "Go の版違い", manager: "go", pkg: "example.org/tool", requested: "v1.0.0", routes: map[string]string{"https://proxy.golang.org/example.org/tool/@v/v1.0.0.info": `{"Version":"v2.0.0"}`}, invalid: true},
		{name: "cargo の安定版を選択", manager: "cargo", pkg: "tool", routes: map[string]string{"https://index.crates.io/to/ol/tool": "{\"vers\":\"1.0.0\",\"yanked\":false}\n{\"vers\":\"2.0.0\",\"yanked\":true}\n{\"vers\":\"3.0.0-rc.1\",\"yanked\":false}"}, want: []string{"cargo", "install", "--registry", "crates-io", "--version", "=1.0.0", "tool"}},
		{name: "cargo の指定 prerelease", manager: "cargo", pkg: "tool", requested: "3.0.0-rc.1", routes: map[string]string{"https://index.crates.io/to/ol/tool": `{"vers":"3.0.0-rc.1","yanked":false}`}, want: []string{"cargo", "install", "--registry", "crates-io", "--version", "=3.0.0-rc.1", "tool"}},
		{name: "cargo の撤回版を拒否", manager: "cargo", pkg: "tool", requested: "1.0.0", routes: map[string]string{"https://index.crates.io/to/ol/tool": `{"vers":"1.0.0","yanked":true}`}, invalid: true},
		{name: "gem の指定版", manager: "gem", pkg: "rake", requested: "1.0", routes: map[string]string{"https://rubygems.org/api/v2/rubygems/rake/versions/1.0.json": `{"version":"1.0"}`}, want: []string{"gem", "install", "rake", "--version", "1.0", "--source", "https://rubygems.org"}},
		{name: "gem の最新版", manager: "gem", pkg: "rake", routes: map[string]string{"https://rubygems.org/api/v1/versions/rake/latest.json": `{"version":"2.0"}`, "https://rubygems.org/api/v2/rubygems/rake/versions/2.0.json": `{"version":"2.0"}`}, want: []string{"gem", "install", "rake", "--version", "2.0", "--source", "https://rubygems.org"}},
		{name: "winget の指定版", manager: "winget", pkg: "Git.Git", requested: "2.51.0", want: []string{"winget", "install", "--id", "Git.Git", "--exact", "--version", "2.51.0", "--source", "winget"}},
		{name: "URL の注入を拒否", manager: "uv", pkg: "https://user:secret@example.org", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			httpClient := &restoreHTTPFake{t: t, routes: tc.routes}
			resolver := &restoreResolver{client: httpClient, goos: "linux", goarch: "amd64", run: func(_ context.Context, name string, args ...string) ([]byte, error) {
				require.Equal(t, tc.manager, name)

				if tc.manager == "winget" {
					require.Equal(t, []string{"show", "--id", "Git.Git", "--exact", "--disable-interactivity", "--accept-source-agreements", "--version", "2.51.0", "--source", "winget"}, args)
				} else {
					query := tc.requested
					if query == "" {
						query = "latest"
					}

					require.Equal(t, []string{"view", tc.pkg + "@" + query, "version", "os", "cpu", "--json"}, args)
				}

				return []byte(tc.output), nil
			}}

			tool := snapshot.Tool{Package: tc.pkg, Version: tc.requested}
			if tc.manager == "winget" {
				tool.Source = "winget"
			}

			command, err := resolver.resolve(context.Background(), tc.manager, tool)
			if tc.invalid {
				require.Error(t, err)
				require.Empty(t, command.Args)
				require.NotContains(t, err.Error(), "secret")
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, command.Args)
			}
		})
	}
}

func TestRestoreMetadataFailure(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		status                  int
		networkError, cancelled bool
	}{
		{name: "認証エラー", status: 401}, {name: "サーバーエラー", status: 500}, {name: "通信エラー", networkError: true}, {name: "キャンセル", networkError: true, cancelled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tc.cancelled {
				cancel()
			}

			client := &restoreHTTPFake{t: t, status: tc.status}
			if tc.networkError {
				client.err = errors.New("https://user:secret@example.org")
			}

			resolver := &restoreResolver{client: client}
			_, err := resolver.resolve(ctx, "go", snapshot.Tool{Package: "example.org/tool/cmd/tool", Version: "v1.0.0"})
			require.Error(t, err)
			require.Len(t, client.requests, 1)
			require.NotContains(t, err.Error(), "secret")

			if tc.networkError && !tc.cancelled {
				require.ErrorIs(t, err, client.err)
			}

			if tc.cancelled {
				require.ErrorIs(t, err, context.Canceled)
			}
		})
	}
}

func TestRestoreCommandFailure(t *testing.T) {
	for _, manager := range []string{"npm", "pnpm", "winget"} {
		t.Run(manager, func(t *testing.T) {
			failure := errors.New("照会コマンド失敗")
			resolver := &restoreResolver{run: func(context.Context, string, ...string) ([]byte, error) {
				return []byte(`{"version":"1.0.0"}`), failure
			}}
			_, err := resolver.resolve(context.Background(), manager, snapshot.Tool{Package: "tool", Version: "1.0.0"})
			require.ErrorIs(t, err, failure)
		})
	}
}

func TestRestorePlatformAndIndex(t *testing.T) {
	for _, tc := range []struct {
		name       string
		conditions []string
		current    string
		want       bool
	}{
		{"制約なし", nil, "linux", true}, {"許可", []string{"linux"}, "linux", true}, {"不一致", []string{"darwin"}, "linux", false}, {"除外", []string{"!linux"}, "linux", false}, {"別 OS のみ除外", []string{"!darwin"}, "linux", true}, {"明示的な除外を優先", []string{"linux", "!linux"}, "linux", false},
	} {
		t.Run(tc.name, func(t *testing.T) { require.Equal(t, tc.want, nodeRestorePlatformMatches(tc.conditions, tc.current)) })
	}

	for _, tc := range []struct{ name, prefix string }{{"a", "1"}, {"ab", "2"}, {"abc", "3/a"}, {"abcd", "ab/cd"}} {
		t.Run(tc.name, func(t *testing.T) { require.Equal(t, tc.prefix, cargoIndexPrefix(tc.name)) })
	}
}
