package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/scottlz0310/devsync/internal/config"
)

// PnpmUpdater は pnpm グローバルパッケージマネージャの実装です。
type PnpmUpdater struct{}

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
	cmd := exec.CommandContext(ctx, "pnpm", "outdated", "-g", "--format", "json")

	cmd.Env = append(os.Environ(), "LANG=C", "LC_ALL=C")

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil && !isPnpmOutdatedExitErr(err) {
		return nil, fmt.Errorf(
			"pnpm outdated -g --format json の実行に失敗: %w",
			buildCommandOutputErr(err, combineCommandOutputs(output, stderr.Bytes())),
		)
	}

	packages, parseErr := p.parseOutdatedJSON(output)
	if parseErr != nil {
		return nil, fmt.Errorf(
			"pnpm outdated -g --format json の出力解析に失敗: %w",
			buildCommandOutputErr(parseErr, combineCommandOutputs(output, stderr.Bytes())),
		)
	}

	return &CheckResult{
		AvailableUpdates: len(packages),
		Packages:         packages,
	}, nil
}

func (p *PnpmUpdater) Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	checkResult, err := p.Check(ctx)
	if err != nil {
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

		return result, fmt.Errorf("pnpm update -g に失敗: %w", err)
	}

	result.UpdatedCount = checkResult.AvailableUpdates
	result.Packages = checkResult.Packages
	result.Message = fmt.Sprintf("%d 件の pnpm グローバルパッケージを更新しました", result.UpdatedCount)

	return result, nil
}

func (p *PnpmUpdater) runUpdate(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "pnpm", "update", "-g")
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

func (p *PnpmUpdater) parseOutdatedJSON(output []byte) ([]PackageInfo, error) {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return []PackageInfo{}, nil
	}

	packages := p.parseOutdatedArrayJSON([]byte(trimmed))
	if packages != nil {
		return packages, nil
	}

	return p.parseOutdatedMapJSON([]byte(trimmed))
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
