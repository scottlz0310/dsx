package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/scottlz0310/devsync/internal/config"
)

// CargoUpdater は cargo (Rust パッケージ) の実装です。
type CargoUpdater struct {
	// 将来的な拡張のための設定フィールド
}

// 起動時にレジストリに登録
func init() {
	Register(&CargoUpdater{})
}

func (c *CargoUpdater) Name() string {
	return "cargo"
}

func (c *CargoUpdater) DisplayName() string {
	return "cargo (Rust パッケージ)"
}

func (c *CargoUpdater) IsAvailable() bool {
	_, err := exec.LookPath("cargo")
	return err == nil
}

func (c *CargoUpdater) Configure(cfg config.ManagerConfig) error {
	// 現時点では設定項目なし
	return nil
}

func (c *CargoUpdater) Check(ctx context.Context) (*CheckResult, error) {
	// cargo install --list でインストール済みパッケージを取得
	cmd := exec.CommandContext(ctx, "cargo", "install", "--list")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("cargo install --list の実行に失敗: %w", err)
	}

	packages := c.parseInstallList(string(output))

	// cargo は個別の outdated チェックがないため、
	// AvailableUpdates は 0 とし、インストール済みパッケージのみ返す
	// 実際の更新可否は update 実行時に判定される
	return &CheckResult{
		AvailableUpdates: 0,
		Packages:         packages,
		Message:          fmt.Sprintf("%d 件のインストール済みパッケージを確認（更新可否は実行時に判定）", len(packages)),
	}, nil
}

func (c *CargoUpdater) Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	result := &UpdateResult{}

	// まず更新確認
	checkResult, err := c.Check(ctx)
	if err != nil {
		return nil, err
	}

	if len(checkResult.Packages) == 0 {
		result.Message = "cargo でインストールされたパッケージがありません"
		return result, nil
	}

	if opts.DryRun {
		result.Message = fmt.Sprintf("%d 件のインストール済みパッケージについて更新を確認します（DryRunモード）", len(checkResult.Packages))
		result.Packages = checkResult.Packages

		return result, nil
	}

	// cargo-update がインストールされているか確認
	// cargo-update は cargo のサブコマンドとして動作するため、
	// cargo install-update --help で確認
	checkCmd := exec.CommandContext(ctx, "cargo", "install-update", "--help")
	if err := checkCmd.Run(); err == nil {
		// cargo-update を使用（推奨）
		cmd := exec.CommandContext(ctx, "cargo", "install-update", "-a")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin

		if err := cmd.Run(); err != nil {
			result.Errors = append(result.Errors, err)
			return result, fmt.Errorf("cargo install-update -a に失敗: %w", err)
		}
	} else {
		// cargo-update がない場合は個別に再インストール
		for _, pkg := range checkResult.Packages {
			cmd := exec.CommandContext(ctx, "cargo", "install", "--force", pkg.Name)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin

			if err := cmd.Run(); err != nil {
				result.FailedCount++
				result.Errors = append(result.Errors, fmt.Errorf("%s: %w", pkg.Name, err))

				continue
			}

			result.UpdatedCount++
		}

		if len(result.Errors) > 0 {
			result.Packages = checkResult.Packages
			result.Message = fmt.Sprintf("%d 件更新、%d 件失敗", result.UpdatedCount, result.FailedCount)

			return result, fmt.Errorf("一部のパッケージ更新に失敗しました")
		}
	}

	result.Packages = checkResult.Packages
	result.Message = fmt.Sprintf("%d 件のパッケージを確認・更新しました", result.UpdatedCount)

	return result, nil
}

// parseInstallList は "cargo install --list" の出力をパースします
// 形式:
// package-name v1.0.0:
//
//	binary1
//	binary2
//
// another-package v2.0.0:
//
//	binary3
func (c *CargoUpdater) parseInstallList(output string) []PackageInfo {
	output = strings.ReplaceAll(output, "\r\n", "\n")
	lines := strings.Split(output, "\n")
	packages := make([]PackageInfo, 0, len(lines))

	for _, line := range lines {
		// インデントされた行（バイナリ名）をスキップ
		if strings.HasPrefix(line, "    ") || line == "" {
			continue
		}

		// "package-name v1.0.0:" 形式をパース
		// コロンで終わることを確認
		if !strings.HasSuffix(line, ":") {
			continue
		}

		line = strings.TrimSuffix(line, ":")

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		name := parts[0]
		version := strings.TrimPrefix(parts[1], "v")

		pkg := PackageInfo{
			Name:           name,
			CurrentVersion: version,
			NewVersion:     "", // cargo は事前に新バージョンを知る手段がない
		}
		packages = append(packages, pkg)
	}

	return packages
}
