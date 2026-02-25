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
	"strings"

	"github.com/scottlz0310/dsx/internal/config"
)

// PnpmUpdater は pnpm グローバルパッケージマネージャの実装です。
type PnpmUpdater struct{}

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
