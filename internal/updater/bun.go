package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/scottlz0310/dsx/internal/config"
	"github.com/scottlz0310/dsx/internal/selfupdate"
)

const (
	bunLatestReleaseAPI = "https://api.github.com/repos/oven-sh/bun/releases/latest"

	bunOwnerSelf     bunInstallOwner = "bun"
	bunOwnerHomebrew bunInstallOwner = "homebrew"
	bunOwnerScoop    bunInstallOwner = "scoop"
	bunOwnerUnknown  bunInstallOwner = "unknown"
)

type bunInstallOwner string

type bunReleaseFetcher func(context.Context, string) (latestVersion, releaseURL string, err error)
type bunOutputRunner func(context.Context, ...string) ([]byte, error)
type bunInteractiveRunner func(context.Context, ...string) error

// BunUpdater は Bun のグローバルパッケージと Bun 本体を更新します。
type BunUpdater struct {
	lookPathStep       func(string) (string, error)
	detectOwnerStep    func(string) bunInstallOwner
	fetchReleaseStep   bunReleaseFetcher
	runOutputStep      bunOutputRunner
	runInteractiveStep bunInteractiveRunner
}

func init() {
	Register(&BunUpdater{})
}

func (b *BunUpdater) Name() string {
	return "bun"
}

func (b *BunUpdater) DisplayName() string {
	return "Bun (JavaScript/TypeScript グローバルパッケージ)"
}

func (b *BunUpdater) IsAvailable() bool {
	_, err := b.lookPath("bun")

	return err == nil
}

func (b *BunUpdater) Configure(config.ManagerConfig) error {
	return nil
}

func (b *BunUpdater) Check(ctx context.Context) (*CheckResult, error) {
	output, err := b.runOutput(ctx, "outdated", "-g")
	if err != nil {
		return nil, fmt.Errorf("bun outdated -g の実行に失敗: %w", err)
	}

	packages, err := parseBunOutdatedOutput(string(output))
	if err != nil {
		return nil, fmt.Errorf("bun outdated -g の出力解析に失敗: %w", err)
	}

	return &CheckResult{
		AvailableUpdates: len(packages),
		Packages:         packages,
	}, nil
}

func (b *BunUpdater) Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	checkResult, err := b.Check(ctx)
	if err != nil {
		return nil, err
	}

	result := &UpdateResult{
		Packages: checkResult.Packages,
	}

	if checkResult.AvailableUpdates == 0 {
		result.Message = "すべての Bun グローバルパッケージは最新です"

		return result, nil
	}

	if opts.DryRun {
		result.Message = fmt.Sprintf("%d 件の Bun グローバルパッケージが更新可能です（DryRunモード）", checkResult.AvailableUpdates)

		return result, nil
	}

	if err := b.runInteractive(ctx, "update", "-g", "--latest"); err != nil {
		result.Errors = append(result.Errors, err)

		return result, fmt.Errorf("bun update -g --latest に失敗: %w", err)
	}

	result.UpdatedCount = checkResult.AvailableUpdates
	result.Message = fmt.Sprintf("%d 件の Bun グローバルパッケージを更新しました", result.UpdatedCount)

	return result, nil
}

func (b *BunUpdater) CheckSelfUpdate(ctx context.Context) (*CheckResult, error) {
	bunPath, err := b.lookPath("bun")
	if err != nil {
		return nil, fmt.Errorf("bun 実行ファイルの確認に失敗: %w", err)
	}

	owner := b.detectOwner(bunPath)
	if owner != bunOwnerSelf {
		return &CheckResult{Message: bunSelfUpdateSkipMessage(owner)}, nil
	}

	currentOutput, err := b.runOutput(ctx, "--version")
	if err != nil {
		return nil, fmt.Errorf("bun --version の実行に失敗: %w", err)
	}

	currentVersion := normalizeBunVersion(string(currentOutput))

	currentCore, ok := selfupdate.ParseSemverCore(currentVersion)
	if !ok {
		return nil, fmt.Errorf("bun の現在バージョンを解析できません: %q", strings.TrimSpace(string(currentOutput)))
	}

	checkCtx, cancel := context.WithTimeout(ctx, selfupdate.CheckTimeout)
	defer cancel()

	latestVersion, _, err := b.fetchRelease(checkCtx, currentVersion)
	if err != nil {
		return nil, fmt.Errorf("bun の最新リリース取得に失敗: %w", err)
	}

	latestVersion = normalizeBunVersion(latestVersion)

	latestCore, ok := selfupdate.ParseSemverCore(latestVersion)
	if !ok {
		return nil, fmt.Errorf("bun の最新バージョンを解析できません: %q", latestVersion)
	}

	if selfupdate.CompareSemverCore(latestCore, currentCore) <= 0 {
		return &CheckResult{Message: "Bun 本体は最新です"}, nil
	}

	return &CheckResult{
		AvailableUpdates: 1,
		Packages: []PackageInfo{
			{
				Name:           "bun",
				CurrentVersion: currentVersion,
				NewVersion:     latestVersion,
			},
		},
		Message: "Bun 本体の更新が可能です",
	}, nil
}

func (b *BunUpdater) SelfUpdate(ctx context.Context, opts UpdateOptions) (*SelfUpdateResult, error) {
	checkResult, err := b.CheckSelfUpdate(ctx)
	if err != nil {
		return nil, err
	}

	result := &SelfUpdateResult{
		Continuation: ContinueNormalUpdate,
		UpdateResult: UpdateResult{
			Packages: checkResult.Packages,
			Message:  checkResult.Message,
		},
	}

	if checkResult.AvailableUpdates == 0 {
		return result, nil
	}

	if opts.DryRun {
		result.Message = "Bun 本体の更新が可能です（DryRunモード）"

		return result, nil
	}

	if err := b.runInteractive(ctx, upgradeCommand); err != nil {
		result.Errors = append(result.Errors, err)

		return result, fmt.Errorf("bun upgrade に失敗: %w", err)
	}

	result.UpdatedCount = 1
	result.Message = "Bun 本体を更新しました"

	return result, nil
}

func parseBunOutdatedOutput(output string) ([]PackageInfo, error) {
	lines := strings.Split(output, "\n")
	packages := make([]PackageInfo, 0)
	headerFound := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "bun outdated v") || isBunTableBorder(trimmed) {
			continue
		}

		columns := splitBunTableRow(trimmed)
		if !headerFound {
			if isBunOutdatedHeader(columns) {
				headerFound = true
				continue
			}

			return nil, fmt.Errorf("ヘッダーを認識できません: %q", trimmed)
		}

		if len(columns) != 4 {
			return nil, fmt.Errorf("列数が不正です: %q", trimmed)
		}

		if columns[0] == "" || columns[1] == "" || columns[3] == "" {
			return nil, fmt.Errorf("必須列が空です: %q", trimmed)
		}

		packages = append(packages, PackageInfo{
			Name:           columns[0],
			CurrentVersion: columns[1],
			NewVersion:     columns[3],
		})
	}

	return packages, nil
}

func splitBunTableRow(line string) []string {
	normalized := strings.ReplaceAll(line, "│", "|")
	if !strings.HasPrefix(normalized, "|") || !strings.HasSuffix(normalized, "|") {
		return nil
	}

	rawColumns := strings.Split(strings.Trim(normalized, "|"), "|")
	columns := make([]string, 0, len(rawColumns))

	for _, column := range rawColumns {
		columns = append(columns, strings.TrimSpace(column))
	}

	return columns
}

func isBunOutdatedHeader(columns []string) bool {
	return len(columns) == 4 &&
		columns[0] == "Package" &&
		columns[1] == "Current" &&
		columns[2] == "Update" &&
		columns[3] == "Latest"
}

func isBunTableBorder(line string) bool {
	return strings.TrimFunc(line, func(r rune) bool {
		switch r {
		case '|', '-', '+', ' ', '┌', '┐', '└', '┘', '├', '┤', '┬', '┴', '┼', '─', '│':
			return true
		default:
			return false
		}
	}) == ""
}

func normalizeBunVersion(version string) string {
	normalized := strings.TrimSpace(version)
	normalized = strings.TrimPrefix(normalized, "bun-")

	return strings.TrimPrefix(normalized, "v")
}

func bunSelfUpdateSkipMessage(owner bunInstallOwner) string {
	switch owner {
	case bunOwnerHomebrew:
		return "Bun 本体は Homebrew 管理のため bun upgrade をスキップします"
	case bunOwnerScoop:
		return "Bun 本体は Scoop 管理のため bun upgrade をスキップします"
	default:
		return "Bun 本体のインストール経路を安全に判定できないため bun upgrade をスキップします"
	}
}

func detectBunInstallOwner(bunPath string) bunInstallOwner {
	resolvedPath, err := filepath.EvalSymlinks(bunPath)
	if err != nil {
		resolvedPath = bunPath
	}

	homeDir, homeErr := os.UserHomeDir()
	if homeErr != nil {
		homeDir = ""
	}

	return classifyBunInstallOwner(
		bunPath,
		resolvedPath,
		os.Getenv("BUN_INSTALL"),
		homeDir,
		os.Getenv("SCOOP"),
	)
}

func classifyBunInstallOwner(bunPath, resolvedPath, bunInstallDir, homeDir, scoopDir string) bunInstallOwner {
	paths := []string{normalizePortablePath(bunPath), normalizePortablePath(resolvedPath)}

	for _, candidate := range paths {
		if isHomebrewBunPath(candidate) {
			return bunOwnerHomebrew
		}

		if isScoopBunPath(candidate, scoopDir, homeDir) {
			return bunOwnerScoop
		}
	}

	for _, candidate := range paths {
		if isBunManagedPath(candidate, bunInstallDir, homeDir) {
			return bunOwnerSelf
		}
	}

	return bunOwnerUnknown
}

func isHomebrewBunPath(candidate string) bool {
	return strings.Contains(candidate, "/cellar/bun/") ||
		strings.HasSuffix(candidate, "/homebrew/bin/bun") ||
		strings.HasSuffix(candidate, "/homebrew/bin/bun.exe") ||
		strings.HasSuffix(candidate, "/.linuxbrew/bin/bun") ||
		strings.HasSuffix(candidate, "/.linuxbrew/bin/bun.exe")
}

func isScoopBunPath(candidate, scoopDir, homeDir string) bool {
	if pathWithinPortable(candidate, normalizePortablePath(scoopDir)) {
		return true
	}

	defaultRoot := path.Join(normalizePortablePath(homeDir), "scoop")
	if pathWithinPortable(candidate, defaultRoot) {
		return true
	}

	return strings.Contains(candidate, "/scoop/apps/bun/") ||
		strings.HasSuffix(candidate, "/scoop/shims/bun") ||
		strings.HasSuffix(candidate, "/scoop/shims/bun.exe")
}

func isBunManagedPath(candidate, bunInstallDir, homeDir string) bool {
	if pathWithinPortable(candidate, path.Join(normalizePortablePath(bunInstallDir), "bin")) {
		return true
	}

	return pathWithinPortable(candidate, path.Join(normalizePortablePath(homeDir), ".bun", "bin"))
}

func normalizePortablePath(value string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if normalized == "" {
		return ""
	}

	return strings.ToLower(path.Clean(normalized))
}

func pathWithinPortable(candidate, root string) bool {
	if candidate == "" || root == "" || root == "." || root == "/" {
		return false
	}

	return candidate == root || strings.HasPrefix(candidate, strings.TrimSuffix(root, "/")+"/")
}

func (b *BunUpdater) lookPath(name string) (string, error) {
	if b.lookPathStep != nil {
		return b.lookPathStep(name)
	}

	return exec.LookPath(name)
}

func (b *BunUpdater) detectOwner(bunPath string) bunInstallOwner {
	if b.detectOwnerStep != nil {
		return b.detectOwnerStep(bunPath)
	}

	return detectBunInstallOwner(bunPath)
}

func (b *BunUpdater) fetchRelease(ctx context.Context, currentVersion string) (latestVersion, releaseURL string, err error) {
	if b.fetchReleaseStep != nil {
		return b.fetchReleaseStep(ctx, currentVersion)
	}

	return selfupdate.FetchGitHubLatestRelease(ctx, bunLatestReleaseAPI, "dsx-bun-updater/"+currentVersion)
}

func (b *BunUpdater) runOutput(ctx context.Context, args ...string) ([]byte, error) {
	if b.runOutputStep != nil {
		return b.runOutputStep(ctx, args...)
	}

	cmd := exec.CommandContext(ctx, "bun", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, buildCommandOutputErr(err, output)
	}

	return output, nil
}

func (b *BunUpdater) runInteractive(ctx context.Context, args ...string) error {
	if b.runInteractiveStep != nil {
		return b.runInteractiveStep(ctx, args...)
	}

	cmd := exec.CommandContext(ctx, "bun", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

var _ Updater = (*BunUpdater)(nil)
var _ ManagerSelfUpdater = (*BunUpdater)(nil)
