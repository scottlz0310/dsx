package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	selfUpdateLatestReleaseAPI = "https://api.github.com/repos/scottlz0310/dsx/releases/latest"
	selfUpdateGoInstallTarget  = "github.com/scottlz0310/dsx/cmd/dsx@latest"
	selfUpdateCheckTimeout     = 2 * time.Second
)

type selfUpdateInfo struct {
	CurrentVersion string
	LatestVersion  string
	ReleaseURL     string
}

type semverCore struct {
	Major int
	Minor int
	Patch int
}

type latestReleaseResponse struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

var (
	selfUpdateCheckOnly bool

	selfUpdateCheckStep        = checkSelfUpdateAvailable
	selfUpdateApplyStep        = applySelfUpdate
	selfUpdateFetchReleaseStep = fetchLatestRelease
)

var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "dsx 本体を更新します",
	Long: `dsx 本体の更新確認と更新適用を行います。

既定では更新確認後に更新を実行します。
確認のみ行う場合は --check を指定してください。`,
	RunE: runSelfUpdate,
}

func init() {
	rootCmd.AddCommand(selfUpdateCmd)
	selfUpdateCmd.Flags().BoolVar(&selfUpdateCheckOnly, "check", false, "更新確認のみ行う（更新は適用しない）")
}

func runSelfUpdate(cmd *cobra.Command, args []string) error {
	info, err := selfUpdateCheckStep(context.Background(), version)
	if err != nil {
		return fmt.Errorf("更新確認に失敗しました: %w", err)
	}

	if info == nil {
		if isDevelopmentBuildVersion(version) {
			fmt.Printf("ℹ️  開発版（%s）のため更新比較をスキップしました。\n", version)
		} else {
			fmt.Printf("✅ すでに最新です（%s）\n", version)
		}

		return nil
	}

	fmt.Printf("🆕 新しいバージョン %s が利用可能です（現在: %s）\n", info.LatestVersion, info.CurrentVersion)

	if info.ReleaseURL != "" {
		fmt.Printf("   リリース情報: %s\n", info.ReleaseURL)
	}

	if selfUpdateCheckOnly {
		return nil
	}

	fmt.Println("🔄 self-update を実行します...")

	applyCtx := cmd.Context()
	if applyCtx == nil {
		applyCtx = context.Background()
	}

	if err := selfUpdateApplyStep(applyCtx); err != nil {
		return err
	}

	fmt.Println("✅ self-update が完了しました。")
	fmt.Println("💡 新しいシェルで `dsx --version` を確認してください。")

	return nil
}

func printSelfUpdateNoticeAtEnd() {
	info, err := selfUpdateCheckStep(context.Background(), version)
	if err != nil || info == nil {
		return
	}

	fmt.Println()
	fmt.Printf("🆕 dsx の新しいバージョン %s が利用可能です（現在: %s）\n", info.LatestVersion, info.CurrentVersion)
	fmt.Println("   更新コマンド: dsx self-update")

	if info.ReleaseURL != "" {
		fmt.Printf("   リリース情報: %s\n", info.ReleaseURL)
	}
}

func checkSelfUpdateAvailable(ctx context.Context, currentVersion string) (*selfUpdateInfo, error) {
	if isDevelopmentBuildVersion(currentVersion) {
		return nil, nil
	}

	currentCore, ok := parseSemverCore(currentVersion)
	if !ok {
		return nil, nil
	}

	checkCtx, cancel := context.WithTimeout(ctx, selfUpdateCheckTimeout)
	defer cancel()

	latestVersion, releaseURL, err := selfUpdateFetchReleaseStep(checkCtx)
	if err != nil {
		return nil, err
	}

	latestCore, ok := parseSemverCore(latestVersion)
	if !ok {
		return nil, nil
	}

	if compareSemverCore(latestCore, currentCore) <= 0 {
		return nil, nil
	}

	return &selfUpdateInfo{
		CurrentVersion: strings.TrimSpace(currentVersion),
		LatestVersion:  strings.TrimSpace(latestVersion),
		ReleaseURL:     strings.TrimSpace(releaseURL),
	}, nil
}

func fetchLatestRelease(ctx context.Context) (latestVersion, releaseURL string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, selfUpdateLatestReleaseAPI, http.NoBody)
	if err != nil {
		return "", "", err
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "dsx/"+strings.TrimSpace(version))

	client := &http.Client{}

	//nolint:gosec // 固定URL（selfUpdateLatestReleaseAPI）のみへアクセスする
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

func applySelfUpdate(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	cmd := exec.CommandContext(ctx, "go", "install", selfUpdateGoInstallTarget)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		if runtime.GOOS == "windows" {
			return fmt.Errorf(
				"self-update に失敗しました（実行中バイナリの置換競合の可能性があります）。別シェルで `go install %s` を実行してください: %w",
				selfUpdateGoInstallTarget,
				err,
			)
		}

		return fmt.Errorf("self-update に失敗しました: %w", err)
	}

	return nil
}

func isDevelopmentBuildVersion(v string) bool {
	trimmed := strings.TrimSpace(v)
	return trimmed == "" || trimmed == "dev" || trimmed == "(devel)"
}

func parseSemverCore(v string) (semverCore, bool) {
	trimmed := strings.TrimSpace(v)
	trimmed = strings.TrimPrefix(trimmed, "v")
	parts := strings.SplitN(trimmed, "-", 2)
	coreAndBuild := strings.SplitN(parts[0], "+", 2)
	core := strings.TrimSpace(coreAndBuild[0])

	segments := strings.Split(core, ".")
	if len(segments) != 3 {
		return semverCore{}, false
	}

	major, err := strconv.Atoi(strings.TrimSpace(segments[0]))
	if err != nil {
		return semverCore{}, false
	}

	minor, err := strconv.Atoi(strings.TrimSpace(segments[1]))
	if err != nil {
		return semverCore{}, false
	}

	patch, err := strconv.Atoi(strings.TrimSpace(segments[2]))
	if err != nil {
		return semverCore{}, false
	}

	return semverCore{
		Major: major,
		Minor: minor,
		Patch: patch,
	}, true
}

func compareSemverCore(left, right semverCore) int {
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
