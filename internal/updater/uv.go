package updater

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/scottlz0310/devsync/internal/config"
)

// UVUpdater は uv tool (Python CLI ツール) の実装です。
type UVUpdater struct{}

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

	cmd := exec.CommandContext(ctx, "uv", "tool", "upgrade", "--all")
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
