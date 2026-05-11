package selfupdate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "空文字列", in: "", want: ""},
		{name: "vあり", in: "v1.2.3", want: "v1.2.3"},
		{name: "vなし", in: "1.2.3", want: "v1.2.3"},
		{name: "前後空白", in: " 1.2.3 ", want: "v1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NormalizeVersion(tt.in))
		})
	}
}

func TestInstallTarget(t *testing.T) {
	assert.Equal(t, GoInstallPackage+"@v1.2.3", InstallTarget("1.2.3"))
	assert.Equal(t, GoInstallPackage+"@v1.2.3", InstallTarget("v1.2.3"))
}

func TestIsDevelopmentBuildVersion(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "空文字列", in: "", want: true},
		{name: "dev", in: "dev", want: true},
		{name: "devel", in: DevelVersion, want: true},
		{name: "前後空白つきdev", in: " dev ", want: true},
		{name: "通常バージョン", in: "v1.2.3", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsDevelopmentBuildVersion(tt.in))
		})
	}
}

func TestParseSemverCore(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want SemverCore
		ok   bool
	}{
		{name: "vあり", in: "v1.2.3", want: SemverCore{Major: 1, Minor: 2, Patch: 3}, ok: true},
		{name: "vなし", in: "1.2.3", want: SemverCore{Major: 1, Minor: 2, Patch: 3}, ok: true},
		{name: "prereleaseはcoreだけ比較", in: "v1.2.3-beta.1", want: SemverCore{Major: 1, Minor: 2, Patch: 3}, ok: true},
		{name: "build metadataはcoreだけ比較", in: "v1.2.3+meta", want: SemverCore{Major: 1, Minor: 2, Patch: 3}, ok: true},
		{name: "前後空白", in: " v1.2.3 ", want: SemverCore{Major: 1, Minor: 2, Patch: 3}, ok: true},
		{name: "segment不足", in: "v1.2", ok: false},
		{name: "segment過多", in: "v1.2.3.4", ok: false},
		{name: "major不正", in: "vx.2.3", ok: false},
		{name: "minor不正", in: "v1.x.3", ok: false},
		{name: "patch不正", in: "v1.2.x", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseSemverCore(tt.in)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCompareSemverCore(t *testing.T) {
	tests := []struct {
		name  string
		left  SemverCore
		right SemverCore
		want  int
	}{
		{name: "majorが大きい", left: SemverCore{Major: 2}, right: SemverCore{Major: 1}, want: 1},
		{name: "majorが小さい", left: SemverCore{Major: 1}, right: SemverCore{Major: 2}, want: -1},
		{name: "minorが大きい", left: SemverCore{Major: 1, Minor: 3}, right: SemverCore{Major: 1, Minor: 2}, want: 1},
		{name: "minorが小さい", left: SemverCore{Major: 1, Minor: 2}, right: SemverCore{Major: 1, Minor: 3}, want: -1},
		{name: "patchが大きい", left: SemverCore{Major: 1, Minor: 2, Patch: 4}, right: SemverCore{Major: 1, Minor: 2, Patch: 3}, want: 1},
		{name: "patchが小さい", left: SemverCore{Major: 1, Minor: 2, Patch: 3}, right: SemverCore{Major: 1, Minor: 2, Patch: 4}, want: -1},
		{name: "同値", left: SemverCore{Major: 1, Minor: 2, Patch: 3}, right: SemverCore{Major: 1, Minor: 2, Patch: 3}, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, CompareSemverCore(tt.left, tt.right))
		})
	}
}

func TestCheckAvailable(t *testing.T) {
	tests := []struct {
		name            string
		currentVersion  string
		fetchVersion    string
		fetchURL        string
		fetchErr        error
		wantFetchCalls  int
		wantInfo        *Info
		wantErrContains string
	}{
		{
			name:           "開発版は取得しない",
			currentVersion: "dev",
		},
		{
			name:           "現在バージョン不正は取得しない",
			currentVersion: "not-semver",
		},
		{
			name:            "取得エラーを返す",
			currentVersion:  "v1.2.3",
			fetchErr:        errors.New("api error"),
			wantFetchCalls:  1,
			wantErrContains: "api error",
		},
		{
			name:           "最新バージョン不正は更新なし",
			currentVersion: "v1.2.3",
			fetchVersion:   "latest",
			wantFetchCalls: 1,
		},
		{
			name:           "同一バージョンは更新なし",
			currentVersion: "v1.2.3",
			fetchVersion:   "v1.2.3",
			wantFetchCalls: 1,
		},
		{
			name:           "古い最新バージョンは更新なし",
			currentVersion: "v1.2.3",
			fetchVersion:   "v1.2.2",
			wantFetchCalls: 1,
		},
		{
			name:           "新しい最新バージョンはInfoを返す",
			currentVersion: " v1.2.3 ",
			fetchVersion:   " v1.2.4 ",
			fetchURL:       " https://example.com/release ",
			wantFetchCalls: 1,
			wantInfo: &Info{
				CurrentVersion: "v1.2.3",
				LatestVersion:  "v1.2.4",
				ReleaseURL:     "https://example.com/release",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetchCalls := 0
			fetch := func(context.Context) (string, string, error) {
				fetchCalls++
				return tt.fetchVersion, tt.fetchURL, tt.fetchErr
			}

			got, err := CheckAvailable(context.Background(), tt.currentVersion, fetch)
			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantInfo, got)
			}

			assert.Equal(t, tt.wantFetchCalls, fetchCalls)
		})
	}
}

func TestFetchLatestRelease(t *testing.T) {
	tests := []struct {
		name            string
		status          int
		body            string
		wantVersion     string
		wantURL         string
		wantErrContains string
	}{
		{
			name:        "正常に最新リリースを取得する",
			status:      http.StatusOK,
			body:        `{"tag_name":" v1.2.3 ","html_url":" https://example.com/release "}`,
			wantVersion: "v1.2.3",
			wantURL:     "https://example.com/release",
		},
		{
			name:            "非200はエラー",
			status:          http.StatusForbidden,
			body:            `{}`,
			wantErrContains: "status=403 Forbidden",
		},
		{
			name:            "JSON不正はエラー",
			status:          http.StatusOK,
			body:            `{"tag_name":`,
			wantErrContains: "unexpected EOF",
		},
		{
			name:            "タグ空はエラー",
			status:          http.StatusOK,
			body:            `{"tag_name":"   "}`,
			wantErrContains: "最新リリースのタグ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "application/vnd.github+json", r.Header.Get("Accept"))
				assert.Equal(t, "dsx/v1.2.0", r.Header.Get("User-Agent"))

				w.WriteHeader(tt.status)
				_, err := w.Write([]byte(tt.body))
				require.NoError(t, err)
			}))
			defer server.Close()

			restore := restoreFetchLatestReleaseHTTP(server.URL, server.Client())
			defer restore()

			gotVersion, gotURL, err := FetchLatestRelease(context.Background(), "v1.2.0")
			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantVersion, gotVersion)
			assert.Equal(t, tt.wantURL, gotURL)
		})
	}
}

func TestFetchLatestRelease_InvalidAPIURL(t *testing.T) {
	restore := restoreFetchLatestReleaseHTTP("://bad-url", http.DefaultClient)
	defer restore()

	_, _, err := FetchLatestRelease(context.Background(), "v1.2.0")
	require.Error(t, err)
}

func restoreFetchLatestReleaseHTTP(apiURL string, client *http.Client) func() {
	oldURL := latestReleaseAPIURL
	oldClient := latestReleaseHTTPClient

	latestReleaseAPIURL = apiURL
	latestReleaseHTTPClient = client

	return func() {
		latestReleaseAPIURL = oldURL
		latestReleaseHTTPClient = oldClient
	}
}
