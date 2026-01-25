package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/scottlz0310/devsync/internal/config"
)

// NpmUpdater は npm グローバルパッケージマネージャの実装です。
type NpmUpdater struct {
	// 将来的な拡張のための設定フィールド
}

// 起動時にレジストリに登録
func init() {
	Register(&NpmUpdater{})
}

func (n *NpmUpdater) Name() string {
	return "npm"
}

func (n *NpmUpdater) DisplayName() string {
	return "npm (Node.js グローバルパッケージ)"
}

func (n *NpmUpdater) IsAvailable() bool {
	_, err := exec.LookPath("npm")
	return err == nil
}

func (n *NpmUpdater) Configure(cfg config.ManagerConfig) error {
	// 現時点では設定項目なし
	return nil
}

func (n *NpmUpdater) Check(ctx context.Context) (*CheckResult, error) {
	// npm outdated -g --json で更新可能なパッケージを取得
	cmd := exec.CommandContext(ctx, "npm", "outdated", "-g", "--json")
	output, err := cmd.Output()
	
	// npm outdated は更新可能なパッケージがある場合に exit code 1 を返すため、
	// エラーを無視して output の内容を確認
	if err != nil && len(output) == 0 {
		// 実際のエラー（コマンド実行失敗など）
		return nil, fmt.Errorf("npm outdated の実行に失敗: %w", err)
	}

	packages := n.parseOutdatedJSON(output)
	return &CheckResult{
		AvailableUpdates: len(packages),
		Packages:         packages,
	}, nil
}

func (n *NpmUpdater) Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	result := &UpdateResult{}

	// まず更新確認
	checkResult, err := n.Check(ctx)
	if err != nil {
		return nil, err
	}

	if checkResult.AvailableUpdates == 0 {
		result.Message = "すべてのパッケージは最新です"
		return result, nil
	}

	if opts.DryRun {
		result.Message = fmt.Sprintf("%d 件のパッケージが更新可能です（DryRunモード）", checkResult.AvailableUpdates)
		result.Packages = checkResult.Packages
		return result, nil
	}

	// 実際の更新を実行
	cmd := exec.CommandContext(ctx, "npm", "update", "-g")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		result.Errors = append(result.Errors, err)
		return result, fmt.Errorf("npm update -g に失敗: %w", err)
	}

	result.UpdatedCount = checkResult.AvailableUpdates
	result.Packages = checkResult.Packages
	result.Message = fmt.Sprintf("%d 件のパッケージを更新しました", result.UpdatedCount)

	return result, nil
}

// parseOutdatedJSON は "npm outdated -g --json" の出力をパースします
// JSON 形式: { "package-name": { "current": "1.0.0", "wanted": "1.1.0", "latest": "2.0.0", "location": "..." }, ... }
func (n *NpmUpdater) parseOutdatedJSON(output []byte) []PackageInfo {
	var packages []PackageInfo
	
	if len(output) == 0 {
		return packages
	}

	var outdated map[string]struct {
		Current  string `json:"current"`
		Wanted   string `json:"wanted"`
		Latest   string `json:"latest"`
		Location string `json:"location"`
	}

	if err := json.Unmarshal(output, &outdated); err != nil {
		// JSON パースエラーは無視して空リストを返す
		return packages
	}

	for name, info := range outdated {
		pkg := PackageInfo{
			Name:           name,
			CurrentVersion: info.Current,
			NewVersion:     info.Latest,
		}
		packages = append(packages, pkg)
	}

	return packages
}
