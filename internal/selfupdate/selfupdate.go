// Package selfupdate は dsx 本体の更新確認に関する共通ロジックを提供します。
package selfupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// LatestReleaseAPI は dsx の最新 GitHub Release を取得する API です。
	LatestReleaseAPI = "https://api.github.com/repos/scottlz0310/dsx/releases/latest"
	// GoInstallPackage は dsx 本体を go install する際の package path です。
	GoInstallPackage = "github.com/scottlz0310/dsx/cmd/dsx"
	CheckTimeout     = 2 * time.Second
	DevelVersion     = "(devel)"
)

// Info は self-update の更新確認結果を保持します。
type Info struct {
	CurrentVersion string
	LatestVersion  string
	ReleaseURL     string
}

// SemverCore は比較に使う major/minor/patch だけを保持します。
type SemverCore struct {
	Major int
	Minor int
	Patch int
}

type latestReleaseResponse struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// ReleaseFetcher は最新リリース情報を取得する関数です。
type ReleaseFetcher func(context.Context) (latestVersion, releaseURL string, err error)

// NormalizeVersion は v prefix を揃えたバージョン文字列を返します。
func NormalizeVersion(version string) string {
	normalized := strings.TrimSpace(version)
	if normalized == "" {
		return normalized
	}

	if !strings.HasPrefix(normalized, "v") {
		normalized = "v" + normalized
	}

	return normalized
}

// InstallTarget は dsx 本体の go install target を返します。
func InstallTarget(version string) string {
	return GoInstallPackage + "@" + NormalizeVersion(version)
}

// CheckAvailable は現在バージョンと最新リリースを比較し、更新がある場合のみ Info を返します。
func CheckAvailable(ctx context.Context, currentVersion string, fetch ReleaseFetcher) (*Info, error) {
	if IsDevelopmentBuildVersion(currentVersion) {
		return nil, nil
	}

	currentCore, ok := ParseSemverCore(currentVersion)
	if !ok {
		return nil, nil
	}

	checkCtx, cancel := context.WithTimeout(ctx, CheckTimeout)
	defer cancel()

	if fetch == nil {
		fetch = func(ctx context.Context) (string, string, error) {
			return FetchLatestRelease(ctx, currentVersion)
		}
	}

	latestVersion, releaseURL, err := fetch(checkCtx)
	if err != nil {
		return nil, err
	}

	latestCore, ok := ParseSemverCore(latestVersion)
	if !ok {
		return nil, nil
	}

	if CompareSemverCore(latestCore, currentCore) <= 0 {
		return nil, nil
	}

	return &Info{
		CurrentVersion: strings.TrimSpace(currentVersion),
		LatestVersion:  strings.TrimSpace(latestVersion),
		ReleaseURL:     strings.TrimSpace(releaseURL),
	}, nil
}

// FetchLatestRelease は GitHub Releases API から最新リリースタグを取得します。
func FetchLatestRelease(ctx context.Context, userAgentVersion string) (latestVersion, releaseURL string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, LatestReleaseAPI, http.NoBody)
	if err != nil {
		return "", "", err
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "dsx/"+strings.TrimSpace(userAgentVersion))

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			return
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("最新リリース取得に失敗しました: status=%s", resp.Status)
	}

	var payload latestReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", err
	}

	tag := strings.TrimSpace(payload.TagName)
	if tag == "" {
		return "", "", errors.New("最新リリースのタグが取得できませんでした")
	}

	return tag, strings.TrimSpace(payload.HTMLURL), nil
}

// IsDevelopmentBuildVersion は更新比較できない開発版バージョンかどうかを返します。
func IsDevelopmentBuildVersion(v string) bool {
	trimmed := strings.TrimSpace(v)
	return trimmed == "" || trimmed == "dev" || trimmed == DevelVersion
}

// ParseSemverCore は semver 文字列から major/minor/patch を取り出します。
func ParseSemverCore(v string) (SemverCore, bool) {
	trimmed := strings.TrimSpace(v)
	trimmed = strings.TrimPrefix(trimmed, "v")
	parts := strings.SplitN(trimmed, "-", 2)
	coreAndBuild := strings.SplitN(parts[0], "+", 2)
	core := strings.TrimSpace(coreAndBuild[0])

	segments := strings.Split(core, ".")
	if len(segments) != 3 {
		return SemverCore{}, false
	}

	major, err := strconv.Atoi(strings.TrimSpace(segments[0]))
	if err != nil {
		return SemverCore{}, false
	}

	minor, err := strconv.Atoi(strings.TrimSpace(segments[1]))
	if err != nil {
		return SemverCore{}, false
	}

	patch, err := strconv.Atoi(strings.TrimSpace(segments[2]))
	if err != nil {
		return SemverCore{}, false
	}

	return SemverCore{
		Major: major,
		Minor: minor,
		Patch: patch,
	}, true
}

// CompareSemverCore は left と right を比較し、left が大きければ 1、小さければ -1、同値なら 0 を返します。
func CompareSemverCore(left, right SemverCore) int {
	if left.Major != right.Major {
		if left.Major > right.Major {
			return 1
		}

		return -1
	}

	if left.Minor != right.Minor {
		if left.Minor > right.Minor {
			return 1
		}

		return -1
	}

	if left.Patch != right.Patch {
		if left.Patch > right.Patch {
			return 1
		}

		return -1
	}

	return 0
}
