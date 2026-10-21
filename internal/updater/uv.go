package updater

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/scottlz0310/dsx/internal/config"
)

// UVUpdater は uv tool (Python CLI ツール) の実装です。
type UVUpdater struct {
	// listRunning は dir にある names の実行ファイルで動作中のプロセスを返します。nil のときは実際のプロセスを調べます（テスト用に差し替え可能）。
	listRunning func(dir string, names []string) ([]string, error)
}

// uvExecutableNames は `uv self update` が置き換える実行ファイルです。
var uvExecutableNames = []string{"uv.exe", "uvx.exe", "uvw.exe"}

var uvSelfUpdatePattern = regexp.MustCompile(`(?i)would update uv from v?(\S+) to v?(\S+)`)

const uvSelfUpdateUnavailableMessage = "uv 本体の self update はこのインストール経路では利用できないためスキップします"

// 起動時にレジストリに登録
func init() {
	Register(&UVUpdater{})
}

func (u *UVUpdater) Name() string {
	return "uv"
}

func (u *UVUpdater) DisplayName() string {
	return "uv tool (Python CLI ツール)"
}

func (u *UVUpdater) IsAvailable() bool {
	_, err := exec.LookPath("uv")
	return err == nil
}

func (u *UVUpdater) Configure(cfg config.ManagerConfig) error {
	// 現時点では設定項目なし
	return nil
}

func (u *UVUpdater) Check(ctx context.Context) (*CheckResult, error) {
	cmd := exec.CommandContext(ctx, "uv", "tool", "list")

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf(
			"uv tool list の実行に失敗: %w",
			buildCommandOutputErr(err, combineCommandOutputs(output, stderr.Bytes())),
		)
	}

	packages := u.parseToolListOutput(string(output))

	return &CheckResult{
		AvailableUpdates: 0,
		Packages:         packages,
		Message:          fmt.Sprintf("%d 件のインストール済みツールを確認（更新可否は実行時に判定）", len(packages)),
	}, nil
}

func (u *UVUpdater) CheckSelfUpdate(ctx context.Context) (*CheckResult, error) {
	output, err := u.runSelfUpdateDryRun(ctx)
	if err != nil {
		return nil, err
	}

	if isUVSelfUpdateUnsupportedOutput(output) {
		return &CheckResult{
			Message: uvSelfUpdateUnavailableMessage,
		}, nil
	}

	packages := u.parseSelfUpdateDryRunOutput(output)
	if len(packages) == 0 {
		return &CheckResult{
			Message: "uv 本体は最新です",
		}, nil
	}

	return &CheckResult{
		AvailableUpdates: len(packages),
		Packages:         packages,
		Message:          "uv 本体の更新が可能です",
	}, nil
}

func (u *UVUpdater) SelfUpdate(ctx context.Context, opts UpdateOptions) (*SelfUpdateResult, error) {
	checkResult, err := u.CheckSelfUpdate(ctx)
	if err != nil {
		return nil, err
	}

	result := &SelfUpdateResult{
		Continuation: ContinueNormalUpdate,
	}

	if checkResult.AvailableUpdates == 0 {
		result.Message = checkResult.Message
		if result.Message == "" {
			result.Message = "uv 本体は最新です"
		}

		return result, nil
	}

	if opts.DryRun {
		result.Packages = checkResult.Packages
		result.Message = "uv 本体の更新が可能です（DryRunモード）"

		return result, nil
	}

	running, err := u.runningUVProcesses()
	if err != nil {
		return nil, err
	}

	if len(running) > 0 {
		result.InUse = true
		result.Packages = checkResult.Packages
		result.Message = fmt.Sprintf(
			"uv / uvx が実行中のため uv 本体の更新を見送りました（%s）。実行中のプロセス: %s。終了してから再実行してください",
			describeUVVersionChange(checkResult.Packages),
			strings.Join(running, ", "),
		)

		return result, nil
	}

	cmd := exec.CommandContext(ctx, "uv", "self", "update")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		result.Errors = append(result.Errors, err)

		return result, fmt.Errorf("uv self update に失敗: %w", err)
	}

	result.UpdatedCount = checkResult.AvailableUpdates
	result.Packages = checkResult.Packages
	result.Message = "uv 本体を更新しました"

	return result, nil
}

func (u *UVUpdater) Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	result := &UpdateResult{}

	checkResult, err := u.Check(ctx)
	if err != nil {
		return nil, err
	}

	if len(checkResult.Packages) == 0 {
		result.Message = "uv tool でインストールされたツールがありません"
		return result, nil
	}

	if opts.DryRun {
		result.Message = fmt.Sprintf("%d 件のインストール済みツールについて更新を確認します（DryRunモード）", len(checkResult.Packages))
		result.Packages = checkResult.Packages

		return result, nil
	}

	cmd := exec.CommandContext(ctx, "uv", "tool", upgradeCommand, "--all")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		result.Errors = append(result.Errors, err)
		return result, fmt.Errorf("uv tool upgrade --all に失敗: %w", err)
	}

	result.UpdatedCount = len(checkResult.Packages)
	result.Packages = checkResult.Packages
	result.Message = fmt.Sprintf("%d 件のツールを確認・更新しました", result.UpdatedCount)

	return result, nil
}

// runningUVProcesses は更新で置き換える uv の実行ファイルを使っているプロセスを返します。
// Windows は実行中の実行ファイルを置き換えられず、`uv self update` が exit 2 で失敗するため、更新の前に確認します。
// 例: `uvx windows-mcp serve` のように、他のツールが常駐させている uv / uvx。
func (u *UVUpdater) runningUVProcesses() ([]string, error) {
	list := u.listRunning
	if list == nil {
		list = listRunningExecutables
	}

	path, err := exec.LookPath("uv")
	if err != nil {
		return nil, fmt.Errorf("uv の場所を特定できません: %w", err)
	}

	if resolved, evalErr := filepath.EvalSymlinks(path); evalErr == nil {
		path = resolved
	}

	running, err := list(filepath.Dir(path), uvExecutableNames)
	if err != nil {
		return nil, fmt.Errorf("uv の実行中のプロセスを確認できません: %w", err)
	}

	return running, nil
}

func describeUVVersionChange(packages []PackageInfo) string {
	if len(packages) == 0 {
		return "更新あり"
	}

	return fmt.Sprintf("v%s → v%s", packages[0].CurrentVersion, packages[0].NewVersion)
}

func (u *UVUpdater) runSelfUpdateDryRun(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "uv", "self", "update", "--dry-run")

	output, err := cmd.CombinedOutput()
	if err != nil {
		outputText := string(output)
		if isUVSelfUpdateUnsupportedOutput(outputText) {
			return outputText, nil
		}

		return "", fmt.Errorf(
			"uv self update --dry-run の実行に失敗: %w",
			buildCommandOutputErr(err, output),
		)
	}

	return string(output), nil
}

func isUVSelfUpdateUnsupportedOutput(output string) bool {
	lower := strings.ToLower(output)

	return strings.Contains(lower, "self-update is only available") ||
		strings.Contains(lower, "self-updates are disabled")
}

func (u *UVUpdater) parseToolListOutput(output string) []PackageInfo {
	lines := strings.Split(output, "\n")
	packages := make([]PackageInfo, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "no tools installed") {
			continue
		}

		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "• ") {
			continue
		}

		name, version, ok := parseToolLine(trimmed)
		if !ok {
			continue
		}

		packages = append(packages, PackageInfo{
			Name:           name,
			CurrentVersion: version,
		})
	}

	return packages
}

func (u *UVUpdater) parseSelfUpdateDryRunOutput(output string) []PackageInfo {
	matches := uvSelfUpdatePattern.FindStringSubmatch(output)
	if len(matches) != 3 {
		return []PackageInfo{}
	}

	return []PackageInfo{
		{
			Name:           "uv",
			CurrentVersion: strings.TrimPrefix(matches[1], "v"),
			NewVersion:     strings.TrimPrefix(matches[2], "v"),
		},
	}
}

func parseToolLine(line string) (name, version string, ok bool) {
	normalized := strings.TrimSuffix(line, ":")
	fields := strings.Fields(normalized)

	if len(fields) == 0 {
		return "", "", false
	}

	name = fields[0]
	if name == "-" {
		return "", "", false
	}

	for _, field := range fields[1:] {
		token := strings.Trim(field, "()")
		if token == "" {
			continue
		}

		token = strings.TrimPrefix(token, "v")
		if !looksLikeVersionToken(token) {
			continue
		}

		return name, token, true
	}

	return name, version, true
}
