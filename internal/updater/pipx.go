package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/scottlz0310/devsync/internal/config"
)

// PipxUpdater は pipx (Python CLI ツール) の実装です。
type PipxUpdater struct {
	// 将来的な拡張のための設定フィールド
}

// 起動時にレジストリに登録
func init() {
	Register(&PipxUpdater{})
}

func (p *PipxUpdater) Name() string {
	return "pipx"
}

func (p *PipxUpdater) DisplayName() string {
	return "pipx (Python CLI ツール)"
}

func (p *PipxUpdater) IsAvailable() bool {
	_, err := exec.LookPath("pipx")
	return err == nil
}

func (p *PipxUpdater) Configure(cfg config.ManagerConfig) error {
	// 現時点では設定項目なし
	return nil
}

func (p *PipxUpdater) Check(ctx context.Context) (*CheckResult, error) {
	// pipx list --json でインストール済みパッケージを取得
	cmd := exec.CommandContext(ctx, "pipx", "list", "--json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("pipx list の実行に失敗: %w", err)
	}

	packages := p.parsePipxListJSON(output)
	
	// pipx は個別の outdated チェックがないため、
	// AvailableUpdates は 0 とし、インストール済みパッケージのみ返す
	// 実際の更新可否は upgrade-all 実行時に判定される
	return &CheckResult{
		AvailableUpdates: 0,
		Packages:         packages,
		Message:          fmt.Sprintf("%d 件のインストール済みパッケージを確認（更新可否は実行時に判定）", len(packages)),
	}, nil
}

func (p *PipxUpdater) Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	result := &UpdateResult{}

	// まず更新確認
	checkResult, err := p.Check(ctx)
	if err != nil {
		return nil, err
	}

	if len(checkResult.Packages) == 0 {
		result.Message = "pipx でインストールされたパッケージがありません"
		return result, nil
	}

	if opts.DryRun {
		result.Message = fmt.Sprintf("%d 件のインストール済みパッケージについて更新を確認します（DryRunモード）", len(checkResult.Packages))
		result.Packages = checkResult.Packages
		return result, nil
	}

	// 実際の更新を実行
	cmd := exec.CommandContext(ctx, "pipx", "upgrade-all")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		result.Errors = append(result.Errors, err)
		return result, fmt.Errorf("pipx upgrade-all に失敗: %w", err)
	}

	// pipx upgrade-all は個別のパッケージ結果を返さないため、
	// 処理したパッケージ数を UpdatedCount とする
	result.UpdatedCount = len(checkResult.Packages)
	result.Packages = checkResult.Packages
	result.Message = fmt.Sprintf("%d 件のパッケージを確認・更新しました", result.UpdatedCount)

	return result, nil
}

// parsePipxListJSON は "pipx list --json" の出力をパースします
// JSON 形式: { "venvs": { "package-name": { "metadata": { "main_package": { "package_version": "1.0.0" } } } } }
func (p *PipxUpdater) parsePipxListJSON(output []byte) []PackageInfo {
	var packages []PackageInfo
	
	if len(output) == 0 {
		return packages
	}

	var listResult struct {
		Venvs map[string]struct {
			Metadata struct {
				MainPackage struct {
					PackageVersion string `json:"package_version"`
				} `json:"main_package"`
			} `json:"metadata"`
		} `json:"venvs"`
	}

	if err := json.Unmarshal(output, &listResult); err != nil {
		// JSON パースエラーは無視して空リストを返す
		return packages
	}

	for name, venv := range listResult.Venvs {
		pkg := PackageInfo{
			Name:           name,
			CurrentVersion: venv.Metadata.MainPackage.PackageVersion,
			NewVersion:     "", // pipx は事前に新バージョンを知る手段がない
		}
		packages = append(packages, pkg)
	}

	return packages
}
