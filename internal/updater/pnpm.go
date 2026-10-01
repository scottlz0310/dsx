package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/scottlz0310/dsx/internal/config"
	"github.com/scottlz0310/dsx/internal/selfupdate"
)

type pnpmSelfUpdateOutputRunner func(context.Context, ...string) ([]byte, error)
type pnpmSelfUpdateRunner func(context.Context, ...string) error
type pnpmNvmShimModeChecker func(context.Context) (bool, error)
type pnpmNvmCommandRunner func(context.Context, string, ...string) error

// PnpmUpdater は pnpm グローバルパッケージマネージャの実装です。
type PnpmUpdater struct {
	runSelfUpdateOutputStep pnpmSelfUpdateOutputRunner
	runSelfUpdateStep       pnpmSelfUpdateRunner
	detectNvmShimModeStep   pnpmNvmShimModeChecker
	runNvmCommandStep       pnpmNvmCommandRunner
}

const (
	pnpmNoImporterManifestErrorCode = "ERR_PNPM_NO_IMPORTER_MANIFEST_FOUND"
	pnpmGlobalManifestContent       = "{\"name\":\"pnpm-global\",\"private\":true}\n"
)

// 起動時にレジストリへ登録します。
func init() {
	Register(&PnpmUpdater{})
}

func (p *PnpmUpdater) Name() string {
	return "pnpm"
}

func (p *PnpmUpdater) DisplayName() string {
	return "pnpm (Node.js グローバルパッケージ)"
}

func (p *PnpmUpdater) IsAvailable() bool {
	_, err := exec.LookPath("pnpm")
	return err == nil
}

func (p *PnpmUpdater) Configure(cfg config.ManagerConfig) error {
	// 現時点では設定項目なし
	return nil
}

func (p *PnpmUpdater) Check(ctx context.Context) (*CheckResult, error) {
	return p.check(ctx, false)
}

func (p *PnpmUpdater) check(ctx context.Context, allowManifestCreate bool) (*CheckResult, error) {
	output, stderrOutput, err := p.runOutdatedCommand(ctx)

	if isPnpmNoImporterManifestOutput(output, stderrOutput) {
		if allowManifestCreate {
			if ensureErr := p.ensureGlobalManifest(ctx); ensureErr != nil {
				return nil, fmt.Errorf("pnpm グローバル環境の初期化に失敗: %w", ensureErr)
			}

			output, stderrOutput, err = p.runOutdatedCommand(ctx)
		}

		if isPnpmNoImporterManifestOutput(output, stderrOutput) {
			return nil, buildPnpmNoImporterManifestError(output, stderrOutput)
		}
	}

	if err != nil && !isPnpmOutdatedExitErr(err) {
		return nil, fmt.Errorf(
			"pnpm outdated -g --format json の実行に失敗: %w",
			buildCommandOutputErr(err, combineCommandOutputs(output, stderrOutput)),
		)
	}

	packages, parseErr := p.parseOutdatedJSON(output)
	if parseErr != nil {
		return nil, fmt.Errorf(
			"pnpm outdated -g --format json の出力解析に失敗: %w",
			buildCommandOutputErr(parseErr, combineCommandOutputs(output, stderrOutput)),
		)
	}

	return &CheckResult{
		AvailableUpdates: len(packages),
		Packages:         packages,
	}, nil
}

func (p *PnpmUpdater) runOutdatedCommand(ctx context.Context) (stdout, stderr []byte, err error) {
	cmd := exec.CommandContext(ctx, "pnpm", "outdated", "-g", "--format", "json")

	cmd.Env = append(os.Environ(), "LANG=C", "LC_ALL=C")

	var stderrBuf bytes.Buffer

	cmd.Stderr = &stderrBuf

	output, err := cmd.Output()

	return output, stderrBuf.Bytes(), err
}

func (p *PnpmUpdater) Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	checkResult, err := p.check(ctx, !opts.DryRun)
	if err != nil {
		if opts.DryRun && isPnpmNoImporterManifestError(err) {
			return &UpdateResult{
				Message: "pnpm グローバル環境が未初期化のため、DryRun では更新確認をスキップしました（通常更新時は自動初期化して再試行します）",
			}, nil
		}

		return nil, err
	}

	result := &UpdateResult{}

	if checkResult.AvailableUpdates == 0 {
		result.Message = "すべての pnpm グローバルパッケージは最新です"

		return result, nil
	}

	if opts.DryRun {
		result.Message = fmt.Sprintf("%d 件の pnpm グローバルパッケージが更新可能です（DryRunモード）", checkResult.AvailableUpdates)
		result.Packages = checkResult.Packages

		return result, nil
	}

	if err := p.runUpdate(ctx); err != nil {
		result.Errors = append(result.Errors, err)

		return result, fmt.Errorf("pnpm update -g --latest に失敗: %w", err)
	}

	result.UpdatedCount = checkResult.AvailableUpdates
	result.Packages = checkResult.Packages
	result.Message = fmt.Sprintf("%d 件の pnpm グローバルパッケージを更新しました", result.UpdatedCount)

	return result, nil
}

func (p *PnpmUpdater) CheckSelfUpdate(ctx context.Context) (*CheckResult, error) {
	currentOutput, err := p.runSelfUpdateOutput(ctx, "--version")
	if err != nil {
		return nil, fmt.Errorf("pnpm --version の実行に失敗: %w", err)
	}

	currentVersion, err := parsePnpmVersionOutput(currentOutput)
	if err != nil {
		return nil, fmt.Errorf("pnpm --version の出力解析に失敗: %w", err)
	}

	latestOutput, err := p.runSelfUpdateOutput(ctx, "view", "pnpm", "version", "--json")
	if err != nil {
		return nil, fmt.Errorf("pnpm view pnpm version --json の実行に失敗: %w", err)
	}

	latestVersion, err := parsePnpmVersionOutput(latestOutput)
	if err != nil {
		return nil, fmt.Errorf("pnpm view pnpm version --json の出力解析に失敗: %w", err)
	}

	if comparePnpmVersions(latestVersion, currentVersion) <= 0 {
		return &CheckResult{Message: "pnpm 本体は最新です"}, nil
	}

	return &CheckResult{
		AvailableUpdates: 1,
		Packages: []PackageInfo{
			{
				Name:           "pnpm",
				CurrentVersion: currentVersion,
				NewVersion:     latestVersion,
			},
		},
		Message: "pnpm 本体の更新が可能です",
	}, nil
}

func (p *PnpmUpdater) SelfUpdate(ctx context.Context, opts UpdateOptions) (*SelfUpdateResult, error) {
	checkResult, err := p.CheckSelfUpdate(ctx)
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
		result.Message = "pnpm 本体の更新が可能です（DryRunモード）"

		return result, nil
	}

	nvmShimMode, err := p.detectNvmShimMode(ctx)
	if err != nil {
		result.Errors = append(result.Errors, err)

		return result, fmt.Errorf("NVM for Windows の動作モード確認に失敗: %w", err)
	}

	if nvmShimMode {
		targetVersion := checkResult.Packages[0].NewVersion
		if err := p.runNvmShimSelfUpdate(ctx, targetVersion); err != nil {
			result.Errors = append(result.Errors, err)

			return result, fmt.Errorf("NVM for Windows Shim 経由の pnpm 本体更新に失敗: %w", err)
		}

		versionOutput, err := p.runSelfUpdateOutput(ctx, "--version")
		if err != nil {
			result.Errors = append(result.Errors, err)

			return result, fmt.Errorf("NVM for Windows Shim 更新後の pnpm バージョン確認に失敗: %w", err)
		}

		updatedVersion, err := parsePnpmVersionOutput(versionOutput)
		if err != nil {
			result.Errors = append(result.Errors, err)

			return result, fmt.Errorf("NVM for Windows Shim 更新後の pnpm バージョン解析に失敗: %w", err)
		}

		if updatedVersion != targetVersion {
			err := fmt.Errorf("pnpm の更新後バージョンが一致しません（期待値 %s、実際 %s）", targetVersion, updatedVersion)
			result.Errors = append(result.Errors, err)

			return result, err
		}
	} else if err := p.runSelfUpdate(ctx, "self-update"); err != nil {
		result.Errors = append(result.Errors, err)

		return result, fmt.Errorf("pnpm self-update の実行に失敗: %w", err)
	}

	result.UpdatedCount = 1
	result.Message = "pnpm 本体を更新しました"

	return result, nil
}

func (p *PnpmUpdater) detectNvmShimMode(ctx context.Context) (bool, error) {
	if p.detectNvmShimModeStep != nil {
		return p.detectNvmShimModeStep(ctx)
	}

	return detectNvmWindowsShimMode(ctx)
}

func detectNvmWindowsShimMode(ctx context.Context) (bool, error) {
	if runtime.GOOS != windowsOS {
		return false, nil
	}

	nvmPath, err := exec.LookPath("nvm")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return false, nil
		}

		return false, fmt.Errorf("nvm コマンドの検索に失敗: %w", err)
	}

	versionOutput, err := exec.CommandContext(ctx, nvmPath, "version").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("nvm version の実行に失敗: %w", buildCommandOutputErr(err, versionOutput))
	}

	version := extractSemver(string(versionOutput))
	if version == "" {
		return false, fmt.Errorf("nvm version の出力を解析できません: %q", string(versionOutput))
	}

	versionParts, err := parseSemver(version)
	if err != nil {
		return false, fmt.Errorf("nvm version の出力を解析できません: %w", err)
	}

	if versionParts[0] < 2 {
		return false, nil
	}

	modeOutput, err := exec.CommandContext(ctx, nvmPath, "config", "get", "mode").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("nvm config get mode の実行に失敗: %w", buildCommandOutputErr(err, modeOutput))
	}

	switch strings.ToLower(strings.TrimSpace(string(modeOutput))) {
	case "shim":
		return true, nil
	case "link":
		return false, nil
	default:
		return false, fmt.Errorf("nvm config get mode の出力を解析できません: %q", string(modeOutput))
	}
}

func (p *PnpmUpdater) runNvmShimSelfUpdate(ctx context.Context, version string) error {
	if err := p.runNvmCommand(ctx, "npm", "install", "--global", "pnpm@"+version); err != nil {
		return fmt.Errorf("npm install --global pnpm@%s の実行に失敗: %w", version, err)
	}

	if err := p.runNvmCommand(ctx, "nvm", "reshim"); err != nil {
		return fmt.Errorf("nvm reshim の実行に失敗: %w", err)
	}

	return nil
}

func (p *PnpmUpdater) runNvmCommand(ctx context.Context, name string, args ...string) error {
	if p.runNvmCommandStep != nil {
		return p.runNvmCommandStep(ctx, name, args...)
	}

	cmd := exec.CommandContext(ctx, name, args...)

	cmd.Env = append(pnpmSelfUpdateEnv(), "CI=true")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func (p *PnpmUpdater) runSelfUpdateOutput(ctx context.Context, args ...string) ([]byte, error) {
	args = pnpmSelfUpdateArgs(args...)
	if p.runSelfUpdateOutputStep != nil {
		return p.runSelfUpdateOutputStep(ctx, args...)
	}

	cmd := exec.CommandContext(ctx, "pnpm", args...)
	cmd.Env = pnpmSelfUpdateEnv()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, buildCommandOutputErr(err, output)
	}

	return output, nil
}

func (p *PnpmUpdater) runSelfUpdate(ctx context.Context, args ...string) error {
	args = pnpmSelfUpdateArgs(args...)
	if p.runSelfUpdateStep != nil {
		return p.runSelfUpdateStep(ctx, args...)
	}

	cmd := exec.CommandContext(ctx, "pnpm", args...)

	cmd.Env = append(pnpmSelfUpdateEnv(), "CI=true")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func pnpmSelfUpdateArgs(args ...string) []string {
	// pnpm v10 以前と v11 以降では packageManager pin を無視する設定名が異なります。
	result := make([]string, 0, 2+len(args))
	result = append(result, "--config.managePackageManagerVersions=false", "--config.pmOnFail=ignore")

	return append(result, args...)
}

func pnpmSelfUpdateEnv() []string {
	env := os.Environ()

	result := make([]string, 0, len(env)+3)

	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, "COREPACK_ENABLE_PROJECT_SPEC") {
			continue
		}

		result = append(result, entry)
	}

	return append(result, "COREPACK_ENABLE_PROJECT_SPEC=0", "LANG=C", "LC_ALL=C")
}

func parsePnpmVersionOutput(output []byte) (string, error) {
	var versionLine string
	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "[WARN]") {
			continue
		}

		if versionLine != "" {
			return "", fmt.Errorf("複数のバージョン行があります: %q", string(output))
		}

		versionLine = trimmed
	}

	var jsonVersion string
	if err := json.Unmarshal([]byte(versionLine), &jsonVersion); err == nil {
		versionLine = strings.TrimSpace(jsonVersion)
	}

	versionLine = strings.TrimPrefix(versionLine, "v")
	if _, ok := selfupdate.ParseSemverCore(versionLine); !ok {
		return "", fmt.Errorf("semver 形式のバージョンではありません: %q", versionLine)
	}

	return versionLine, nil
}

func comparePnpmVersions(left, right string) int {
	leftCore, _ := selfupdate.ParseSemverCore(left)

	rightCore, _ := selfupdate.ParseSemverCore(right)
	if comparison := selfupdate.CompareSemverCore(leftCore, rightCore); comparison != 0 {
		return comparison
	}

	return comparePnpmPrerelease(left, right)
}

func comparePnpmPrerelease(left, right string) int {
	leftIdentifiers := pnpmPrereleaseIdentifiers(left)

	rightIdentifiers := pnpmPrereleaseIdentifiers(right)
	if len(leftIdentifiers) == 0 && len(rightIdentifiers) == 0 {
		return 0
	}

	if len(leftIdentifiers) == 0 {
		return 1
	}

	if len(rightIdentifiers) == 0 {
		return -1
	}

	for i := 0; i < len(leftIdentifiers) && i < len(rightIdentifiers); i++ {
		leftIdentifier := leftIdentifiers[i]

		rightIdentifier := rightIdentifiers[i]

		if comparison := comparePnpmPrereleaseIdentifier(leftIdentifier, rightIdentifier); comparison != 0 {
			return comparison
		}
	}

	return comparePnpmIdentifierCount(len(leftIdentifiers), len(rightIdentifiers))
}

func comparePnpmPrereleaseIdentifier(left, right string) int {
	leftNumeric := isPnpmNumericIdentifier(left)

	rightNumeric := isPnpmNumericIdentifier(right)
	if leftNumeric && rightNumeric {
		return comparePnpmNumericIdentifiers(left, right)
	}

	if leftNumeric {
		return -1
	}

	if rightNumeric {
		return 1
	}

	return strings.Compare(left, right)
}

func comparePnpmIdentifierCount(left, right int) int {
	if left < right {
		return -1
	}

	if left > right {
		return 1
	}

	return 0
}

func pnpmPrereleaseIdentifiers(version string) []string {
	versionWithoutBuild, _, _ := strings.Cut(version, "+")

	_, prerelease, hasPrerelease := strings.Cut(versionWithoutBuild, "-")
	if !hasPrerelease {
		return nil
	}

	return strings.Split(prerelease, ".")
}

func isPnpmNumericIdentifier(identifier string) bool {
	if identifier == "" {
		return false
	}

	for _, char := range identifier {
		if char < '0' || char > '9' {
			return false
		}
	}

	return true
}

func comparePnpmNumericIdentifiers(left, right string) int {
	left = strings.TrimLeft(left, "0")

	right = strings.TrimLeft(right, "0")

	if left == "" {
		left = "0"
	}

	if right == "" {
		right = "0"
	}

	if len(left) < len(right) {
		return -1
	}

	if len(left) > len(right) {
		return 1
	}

	return strings.Compare(left, right)
}

func (p *PnpmUpdater) runUpdate(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "pnpm", "update", "-g", "--latest", "--no-interactive")

	cmd.Env = append(os.Environ(), "CI=true")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

func isPnpmOutdatedExitErr(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}

	// pnpm outdated は更新対象がある場合に 1 を返します。
	return exitErr.ExitCode() == 1
}

func buildPnpmNoImporterManifestError(output, stderr []byte) error {
	return fmt.Errorf(
		"pnpm outdated -g --format json の実行に失敗: %w",
		buildCommandOutputErr(
			errors.New(pnpmNoImporterManifestErrorCode),
			combineCommandOutputs(output, stderr),
		),
	)
}

func isPnpmNoImporterManifestOutput(output, stderr []byte) bool {
	return strings.Contains(string(combineCommandOutputs(output, stderr)), pnpmNoImporterManifestErrorCode)
}

func isPnpmNoImporterManifestError(err error) bool {
	return err != nil && strings.Contains(err.Error(), pnpmNoImporterManifestErrorCode)
}

func (p *PnpmUpdater) ensureGlobalManifest(ctx context.Context) error {
	globalDir, err := p.resolveGlobalDir(ctx)
	if err != nil {
		return err
	}

	manifestPath := filepath.Join(globalDir, "package.json")
	if _, statErr := os.Stat(manifestPath); statErr == nil {
		return nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("pnpm グローバル manifest の状態確認に失敗: %w", statErr)
	}

	if mkdirErr := os.MkdirAll(globalDir, 0o755); mkdirErr != nil {
		return fmt.Errorf("pnpm グローバルディレクトリの作成に失敗: %w", mkdirErr)
	}

	file, openErr := os.OpenFile(manifestPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if openErr != nil {
		if errors.Is(openErr, os.ErrExist) {
			return nil
		}

		return fmt.Errorf("pnpm グローバル manifest の作成に失敗: %w", openErr)
	}

	if _, writeErr := file.WriteString(pnpmGlobalManifestContent); writeErr != nil {
		closeErr := file.Close()
		if closeErr != nil {
			return fmt.Errorf("pnpm グローバル manifest への書き込みに失敗: %s（さらにクローズにも失敗: %w）", writeErr.Error(), closeErr)
		}

		return fmt.Errorf("pnpm グローバル manifest への書き込みに失敗: %w", writeErr)
	}

	if closeErr := file.Close(); closeErr != nil {
		return fmt.Errorf("pnpm グローバル manifest のクローズに失敗: %w", closeErr)
	}

	return nil
}

func (p *PnpmUpdater) resolveGlobalDir(ctx context.Context) (string, error) {
	output, err := runCommandOutputWithLocaleC(
		ctx,
		"pnpm",
		[]string{"root", "-g"},
		"pnpm root -g の実行に失敗: %w",
	)
	if err != nil {
		return "", err
	}

	globalRoot := filepath.Clean(strings.TrimSpace(string(output)))
	if globalRoot == "" || globalRoot == "." {
		return "", errors.New("pnpm root -g の出力が空です")
	}

	if filepath.Base(globalRoot) == "node_modules" {
		return filepath.Dir(globalRoot), nil
	}

	return globalRoot, nil
}

func (p *PnpmUpdater) parseOutdatedJSON(output []byte) ([]PackageInfo, error) {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return []PackageInfo{}, nil
	}

	// pnpm v11 では stdout に [WARN] / [ERR] / [ERROR] の診断行が混入するため、
	// これらのタグで始まる行を除外して JSON 部分のみを抽出する。
	var jsonLines []string
	for _, line := range strings.Split(trimmed, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[WARN]") || strings.HasPrefix(t, "[ERR]") || strings.HasPrefix(t, "[ERROR]") {
			continue
		}

		jsonLines = append(jsonLines, line)
	}

	jsonStr := strings.TrimSpace(strings.Join(jsonLines, "\n"))
	if jsonStr == "" {
		return nil, fmt.Errorf("JSON が見つかりません")
	}

	packages := p.parseOutdatedArrayJSON([]byte(jsonStr))
	if packages != nil {
		return packages, nil
	}

	return p.parseOutdatedMapJSON([]byte(jsonStr))
}

func (p *PnpmUpdater) parseOutdatedArrayJSON(output []byte) []PackageInfo {
	var outdated []struct {
		Name        string `json:"name"`
		PackageName string `json:"packageName"`
		Current     string `json:"current"`
		Latest      string `json:"latest"`
		Wanted      string `json:"wanted"`
	}

	if err := json.Unmarshal(output, &outdated); err != nil {
		return nil
	}

	packages := make([]PackageInfo, 0, len(outdated))

	for _, item := range outdated {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = strings.TrimSpace(item.PackageName)
		}

		if name == "" {
			continue
		}

		newVersion := strings.TrimSpace(item.Latest)
		if newVersion == "" {
			newVersion = strings.TrimSpace(item.Wanted)
		}

		packages = append(packages, PackageInfo{
			Name:           name,
			CurrentVersion: strings.TrimSpace(item.Current),
			NewVersion:     newVersion,
		})
	}

	return packages
}

func (p *PnpmUpdater) parseOutdatedMapJSON(output []byte) ([]PackageInfo, error) {
	var outdated map[string]struct {
		Current string `json:"current"`
		Latest  string `json:"latest"`
		Wanted  string `json:"wanted"`
	}

	if err := json.Unmarshal(output, &outdated); err != nil {
		return nil, fmt.Errorf("JSON の解析に失敗: %w", err)
	}

	packages := make([]PackageInfo, 0, len(outdated))

	for name, item := range outdated {
		newVersion := strings.TrimSpace(item.Latest)
		if newVersion == "" {
			newVersion = strings.TrimSpace(item.Wanted)
		}

		packages = append(packages, PackageInfo{
			Name:           strings.TrimSpace(name),
			CurrentVersion: strings.TrimSpace(item.Current),
			NewVersion:     newVersion,
		})
	}

	return packages, nil
}

var _ ManagerSelfUpdater = (*PnpmUpdater)(nil)
