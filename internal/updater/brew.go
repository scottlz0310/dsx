package updater

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/scottlz0310/devsync/internal/config"
)

// BrewUpdater は Homebrew パッケージマネージャの実装です。
type BrewUpdater struct {
	// cleanup が true の場合、更新後に古いバージョンを削除
	cleanup bool
	// greedy が true の場合、auto_updates が有効な Cask も更新対象に含める
	greedy bool
}

// 起動時にレジストリに登録
func init() {
	Register(&BrewUpdater{cleanup: true, greedy: false})
}

func (b *BrewUpdater) Name() string {
	return "brew"
}

func (b *BrewUpdater) DisplayName() string {
	return "Homebrew"
}

func (b *BrewUpdater) IsAvailable() bool {
	_, err := exec.LookPath("brew")
	return err == nil
}

func (b *BrewUpdater) Configure(cfg config.ManagerConfig) error {
	if cfg == nil {
		return nil
	}
	if cleanup, ok := cfg["cleanup"].(bool); ok {
		b.cleanup = cleanup
	}
	if greedy, ok := cfg["greedy"].(bool); ok {
		b.greedy = greedy
	}
	return nil
}

func (b *BrewUpdater) Check(ctx context.Context) (*CheckResult, error) {
	// brew update でフォーミュラ情報を更新
	updateCmd := exec.CommandContext(ctx, "brew", "update")
	updateCmd.Stdout = os.Stdout
	updateCmd.Stderr = os.Stderr
	if err := updateCmd.Run(); err != nil {
		return nil, fmt.Errorf("brew update に失敗: %w", err)
	}

	// 更新可能なフォーミュラを取得
	outdatedCmd := exec.CommandContext(ctx, "brew", "outdated", "--verbose")
	output, err := outdatedCmd.Output()
	if err != nil {
		// outdated は更新がない場合も exit 0 だが、念のためエラーハンドリング
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("brew outdated に失敗: %s", string(exitErr.Stderr))
		}
	}

	packages := b.parseOutdatedList(string(output))

	return &CheckResult{
		AvailableUpdates: len(packages),
		Packages:         packages,
	}, nil
}

func (b *BrewUpdater) Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	result := &UpdateResult{}

	// まず更新確認
	checkResult, err := b.Check(ctx)
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

	// フォーミュラの更新
	upgradeCmd := exec.CommandContext(ctx, "brew", "upgrade")
	upgradeCmd.Stdout = os.Stdout
	upgradeCmd.Stderr = os.Stderr
	if err := upgradeCmd.Run(); err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("brew upgrade に失敗: %w", err))
	}

	// Cask の更新（greedy オプション考慮）
	caskArgs := []string{"upgrade", "--cask"}
	if b.greedy {
		caskArgs = append(caskArgs, "--greedy")
	}
	caskCmd := exec.CommandContext(ctx, "brew", caskArgs...)
	caskCmd.Stdout = os.Stdout
	caskCmd.Stderr = os.Stderr
	if err := caskCmd.Run(); err != nil {
		// Cask がない環境もあるため、エラーは警告として記録
		result.Errors = append(result.Errors, fmt.Errorf("brew upgrade --cask: %w", err))
	}

	// クリーンアップ
	if b.cleanup {
		cleanupCmd := exec.CommandContext(ctx, "brew", "cleanup")
		cleanupCmd.Stdout = os.Stdout
		cleanupCmd.Stderr = os.Stderr
		if err := cleanupCmd.Run(); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("brew cleanup: %w", err))
		}
	}

	result.UpdatedCount = checkResult.AvailableUpdates
	result.Packages = checkResult.Packages
	result.Message = fmt.Sprintf("%d 件のパッケージを更新しました", result.UpdatedCount)

	return result, nil
}

// parseOutdatedList は "brew outdated --verbose" の出力をパースします
func (b *BrewUpdater) parseOutdatedList(output string) []PackageInfo {
	var packages []PackageInfo
	scanner := bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// 形式: "package (current_version) < new_version" または "package (current_version) != new_version"
		// または単純に: "package (current_version)"
		pkg := PackageInfo{}

		// パッケージ名を取得
		if idx := strings.Index(line, " "); idx != -1 {
			pkg.Name = line[:idx]
			rest := line[idx+1:]

			// バージョン情報をパース
			if strings.HasPrefix(rest, "(") {
				if endIdx := strings.Index(rest, ")"); endIdx != -1 {
					pkg.CurrentVersion = rest[1:endIdx]
					rest = strings.TrimSpace(rest[endIdx+1:])
				}
			}

			// 新バージョン（< または != の後）
			if idx := strings.IndexAny(rest, "<!="); idx != -1 {
				pkg.NewVersion = strings.TrimSpace(rest[idx+1:])
			}
		} else {
			pkg.Name = line
		}

		packages = append(packages, pkg)
	}

	return packages
}
