package updater

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/scottlz0310/devsync/internal/config"
)

var semverPattern = regexp.MustCompile(`v?(\d+\.\d+\.\d+)`)

const windowsOS = "windows"

// NvmUpdater は nvm (Node.js バージョン管理) の実装です。
type NvmUpdater struct{}

// 起動時にレジストリへ登録します。
func init() {
	Register(&NvmUpdater{})
}

func (n *NvmUpdater) Name() string {
	return "nvm"
}

func (n *NvmUpdater) DisplayName() string {
	return "nvm (Node.js バージョン管理)"
}

func (n *NvmUpdater) IsAvailable() bool {
	if runtime.GOOS == windowsOS {
		_, err := exec.LookPath("nvm")
		return err == nil
	}

	_, _, err := n.resolveUnixNvmRuntime()

	return err == nil
}

func (n *NvmUpdater) Configure(cfg config.ManagerConfig) error {
	// 現時点では設定項目なし
	return nil
}

func (n *NvmUpdater) Check(ctx context.Context) (*CheckResult, error) {
	currentVersion, err := n.currentVersion(ctx)
	if err != nil {
		return nil, err
	}

	latestVersion, err := n.latestVersion(ctx)
	if err != nil {
		return nil, err
	}

	if currentVersion == "" {
		return &CheckResult{
			AvailableUpdates: 1,
			Packages: []PackageInfo{
				{
					Name:       "node",
					NewVersion: latestVersion,
				},
			},
			Message: "現在の Node.js バージョンを検出できなかったため、最新バージョンの導入を提案します",
		}, nil
	}

	needsUpdate, cmpErr := isSemverLess(currentVersion, latestVersion)
	if cmpErr != nil {
		return nil, fmt.Errorf("nvm バージョン比較に失敗: %w", cmpErr)
	}

	if !needsUpdate {
		return &CheckResult{
			AvailableUpdates: 0,
			Packages:         []PackageInfo{},
		}, nil
	}

	return &CheckResult{
		AvailableUpdates: 1,
		Packages: []PackageInfo{
			{
				Name:           "node",
				CurrentVersion: currentVersion,
				NewVersion:     latestVersion,
			},
		},
	}, nil
}

func (n *NvmUpdater) Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	checkResult, err := n.Check(ctx)
	if err != nil {
		return nil, err
	}

	result := &UpdateResult{}

	if checkResult.AvailableUpdates == 0 {
		result.Message = "nvm 管理下の Node.js は最新です"

		return result, nil
	}

	if opts.DryRun {
		result.Message = fmt.Sprintf("%d 件の Node.js バージョン更新が可能です（DryRunモード）", checkResult.AvailableUpdates)
		result.Packages = checkResult.Packages

		return result, nil
	}

	targetVersion := checkResult.Packages[0].NewVersion

	cmd, err := n.buildCommand(ctx, "install", targetVersion)
	if err != nil {
		result.Errors = append(result.Errors, err)

		return result, fmt.Errorf("nvm install %s の準備に失敗: %w", targetVersion, err)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		result.Errors = append(result.Errors, err)

		return result, fmt.Errorf("nvm install %s に失敗: %w", targetVersion, err)
	}

	result.UpdatedCount = checkResult.AvailableUpdates
	result.Packages = checkResult.Packages
	result.Message = fmt.Sprintf("Node.js %s をインストールしました", targetVersion)

	return result, nil
}

func (n *NvmUpdater) currentVersion(ctx context.Context) (string, error) {
	output, err := n.runCommandOutput(ctx, "current")
	if err != nil {
		return "", fmt.Errorf("nvm current の実行に失敗: %w", err)
	}

	version, parseErr := parseNvmCurrentVersion(output)
	if parseErr != nil {
		return "", fmt.Errorf("nvm current の出力解析に失敗: %w", parseErr)
	}

	return version, nil
}

func (n *NvmUpdater) latestVersion(ctx context.Context) (string, error) {
	candidates := [][]string{
		{"list", "available"},
		{"ls-remote", "--no-colors", "--lts"},
		{"ls-remote", "--no-colors"},
	}

	errs := make([]string, 0, len(candidates))

	for _, args := range candidates {
		output, err := n.runCommandOutput(ctx, args...)
		if err != nil {
			errs = append(errs, fmt.Sprintf("nvm %s: %v", strings.Join(args, " "), err))
			continue
		}

		version := parseLatestNodeVersion(output)
		if version != "" {
			return version, nil
		}

		errs = append(errs, fmt.Sprintf("nvm %s: バージョンを検出できませんでした", strings.Join(args, " ")))
	}

	return "", fmt.Errorf("最新 Node.js バージョンの取得に失敗: %s", strings.Join(errs, " / "))
}

func (n *NvmUpdater) runCommandOutput(ctx context.Context, args ...string) (string, error) {
	cmd, err := n.buildCommand(ctx, args...)
	if err != nil {
		return "", err
	}

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		return "", buildCommandOutputErr(err, combineCommandOutputs(output, stderr.Bytes()))
	}

	return string(output), nil
}

func (n *NvmUpdater) buildCommand(ctx context.Context, args ...string) (*exec.Cmd, error) {
	if runtime.GOOS == windowsOS {
		cmd := exec.CommandContext(ctx, "nvm", args...)

		cmd.Env = append(os.Environ(), "LANG=C", "LC_ALL=C")

		return cmd, nil
	}

	shell, nvmScript, err := n.resolveUnixNvmRuntime()
	if err != nil {
		return nil, err
	}

	commandText := buildUnixNvmCommand(nvmScript, args...)
	cmd := exec.CommandContext(ctx, shell, "-lc", commandText)

	cmd.Env = append(os.Environ(), "LANG=C", "LC_ALL=C")

	return cmd, nil
}

func (n *NvmUpdater) resolveUnixNvmRuntime() (shell, nvmScript string, err error) {
	shell, err = lookupFirstExecutable("bash", "zsh", "sh")
	if err != nil {
		return "", "", fmt.Errorf("nvm 実行用シェルが見つかりません: %w", err)
	}

	nvmScript, err = resolveNvmScriptPath()
	if err != nil {
		return "", "", err
	}

	return shell, nvmScript, nil
}

func lookupFirstExecutable(candidates ...string) (string, error) {
	for _, candidate := range candidates {
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("利用可能な実行ファイルがありません: %v", candidates)
}

func resolveNvmScriptPath() (string, error) {
	candidates := make([]string, 0, 2)

	if nvmDir := strings.TrimSpace(os.Getenv("NVM_DIR")); nvmDir != "" {
		candidates = append(candidates, filepath.Join(nvmDir, "nvm.sh"))
	}

	home, err := os.UserHomeDir()
	if err == nil && strings.TrimSpace(home) != "" {
		candidates = append(candidates, filepath.Join(home, ".nvm", "nvm.sh"))
	}

	for _, candidate := range candidates {
		info, statErr := os.Stat(candidate)
		if statErr != nil {
			continue
		}

		if info.IsDir() {
			continue
		}

		return candidate, nil
	}

	return "", fmt.Errorf("nvm.sh が見つかりません（NVM_DIR または ~/.nvm）")
}

func buildUnixNvmCommand(nvmScript string, args ...string) string {
	quotedArgs := make([]string, 0, len(args))
	for _, arg := range args {
		quotedArgs = append(quotedArgs, quotePosixShellArg(arg))
	}

	command := "nvm"
	if len(quotedArgs) > 0 {
		command += " " + strings.Join(quotedArgs, " ")
	}

	return fmt.Sprintf(". %s >/dev/null 2>&1 && %s", quotePosixShellArg(nvmScript), command)
}

func quotePosixShellArg(value string) string {
	if value == "" {
		return "''"
	}

	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func parseNvmCurrentVersion(output string) (string, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return "", nil
	}

	lower := strings.ToLower(trimmed)
	if lower == "none" || lower == "n/a" || strings.Contains(lower, "system") {
		return "", nil
	}

	version := extractSemver(trimmed)
	if version == "" {
		return "", fmt.Errorf("バージョン形式を解釈できません: %s", trimmed)
	}

	return version, nil
}

func parseLatestNodeVersion(output string) string {
	lines := strings.Split(output, "\n")
	versions := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.Contains(strings.ToLower(trimmed), "iojs") {
			continue
		}

		matches := semverPattern.FindAllStringSubmatch(trimmed, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}

			versions = append(versions, match[1])
		}
	}

	if len(versions) == 0 {
		return ""
	}

	latest := versions[0]
	for _, version := range versions[1:] {
		less, err := isSemverLess(latest, version)
		if err != nil {
			continue
		}

		if less {
			latest = version
		}
	}

	return latest
}

func extractSemver(text string) string {
	match := semverPattern.FindStringSubmatch(text)
	if len(match) < 2 {
		return ""
	}

	return match[1]
}

func isSemverLess(left, right string) (bool, error) {
	leftParts, err := parseSemver(left)
	if err != nil {
		return false, err
	}

	rightParts, err := parseSemver(right)
	if err != nil {
		return false, err
	}

	for i := 0; i < len(leftParts); i++ {
		switch {
		case leftParts[i] < rightParts[i]:
			return true, nil
		case leftParts[i] > rightParts[i]:
			return false, nil
		}
	}

	return false, nil
}

func parseSemver(value string) ([3]int, error) {
	normalized := strings.TrimPrefix(strings.TrimSpace(value), "v")

	parts := strings.Split(normalized, ".")
	if len(parts) != 3 {
		return [3]int{}, fmt.Errorf("不正な semver 形式: %q", value)
	}

	var result [3]int

	for i := 0; i < len(parts); i++ {
		num, err := strconv.Atoi(parts[i])
		if err != nil {
			return [3]int{}, fmt.Errorf("不正な semver 要素: %q", value)
		}

		result[i] = num
	}

	return result, nil
}
