package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/scottlz0310/devsync/internal/config"
)

// AptUpdater は APT パッケージマネージャ (Debian/Ubuntu) の実装です。
type AptUpdater struct {
	useSudo bool
}

// 起動時にレジストリに登録
func init() {
	Register(&AptUpdater{useSudo: true})
}

func (a *AptUpdater) Name() string {
	return "apt"
}

func (a *AptUpdater) DisplayName() string {
	return "APT (Debian/Ubuntu)"
}

func (a *AptUpdater) IsAvailable() bool {
	_, err := exec.LookPath("apt")
	return err == nil
}

func (a *AptUpdater) Configure(cfg config.ManagerConfig) error {
	if cfg == nil {
		return nil
	}
	if useSudo, ok := cfg["use_sudo"].(bool); ok {
		a.useSudo = useSudo
	}
	return nil
}

func (a *AptUpdater) Check(ctx context.Context) (*CheckResult, error) {
	// パッケージリストを更新
	if err := a.runCommand(ctx, "update"); err != nil {
		return nil, fmt.Errorf("apt update に失敗: %w", err)
	}

	// 更新可能なパッケージを取得
	cmd := exec.CommandContext(ctx, "apt", "list", "--upgradable")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("更新可能パッケージの取得に失敗: %w", err)
	}

	packages := a.parseUpgradableList(string(output))
	return &CheckResult{
		AvailableUpdates: len(packages),
		Packages:         packages,
	}, nil
}

func (a *AptUpdater) Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	result := &UpdateResult{}

	// まず更新確認
	checkResult, err := a.Check(ctx)
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
	args := []string{"upgrade", "-y"}
	if err := a.runCommand(ctx, args...); err != nil {
		result.Errors = append(result.Errors, err)
		return result, fmt.Errorf("apt upgrade に失敗: %w", err)
	}

	result.UpdatedCount = checkResult.AvailableUpdates
	result.Packages = checkResult.Packages
	result.Message = fmt.Sprintf("%d 件のパッケージを更新しました", result.UpdatedCount)

	return result, nil
}

// runCommand は apt コマンドを実行します（必要に応じて sudo を使用）
func (a *AptUpdater) runCommand(ctx context.Context, args ...string) error {
	var cmd *exec.Cmd
	if a.useSudo {
		fullArgs := append([]string{"apt"}, args...)
		cmd = exec.CommandContext(ctx, "sudo", fullArgs...)
	} else {
		cmd = exec.CommandContext(ctx, "apt", args...)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

// parseUpgradableList は "apt list --upgradable" の出力をパースします
func (a *AptUpdater) parseUpgradableList(output string) []PackageInfo {
	var packages []PackageInfo
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		// "Listing..." のヘッダー行をスキップ
		if strings.HasPrefix(line, "Listing") || line == "" {
			continue
		}

		// 形式: "package/release version arch [upgradable from: old-version]"
		parts := strings.Split(line, " ")
		if len(parts) < 2 {
			continue
		}

		nameParts := strings.Split(parts[0], "/")
		if len(nameParts) < 1 {
			continue
		}

		pkg := PackageInfo{
			Name:       nameParts[0],
			NewVersion: parts[1],
		}

		// 旧バージョンを取得
		if idx := strings.Index(line, "upgradable from:"); idx != -1 {
			oldVersion := strings.TrimSuffix(line[idx+len("upgradable from:"):], "]")
			pkg.CurrentVersion = strings.TrimSpace(oldVersion)
		}

		packages = append(packages, pkg)
	}

	return packages
}
