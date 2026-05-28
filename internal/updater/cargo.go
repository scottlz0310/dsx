package updater

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/scottlz0310/dsx/internal/config"
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

	if opts.DryRun {
		checkResult, err := c.Check(ctx)
		if err != nil {
			return nil, err
		}

		if len(checkResult.Packages) == 0 {
			result.Message = "cargo でインストールされたパッケージがありません"
			return result, nil
		}

		result.Message = fmt.Sprintf("%d 件のインストール済みパッケージについて更新を確認します（DryRunモード）", len(checkResult.Packages))
		result.Packages = checkResult.Packages

		return result, nil
	}

	// cargo-update がなければ自動インストール、フルパスを取得
	updateBin, err := c.ensureCargoUpdate(ctx)
	if err != nil {
		return result, err
	}

	// フルパスで直接実行（PATH に ~/.cargo/bin がない環境でも動作）
	// v20 以降はサブコマンド形式: cargo-install-update install-update -a
	var buf bytes.Buffer

	cmd := exec.CommandContext(ctx, updateBin, "install-update", "-a")
	cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		result.Errors = append(result.Errors, err)
		return result, fmt.Errorf("cargo install-update -a に失敗: %w", err)
	}

	result.UpdatedCount = c.parseUpdateCount(buf.String())

	if result.UpdatedCount > 0 {
		result.Message = fmt.Sprintf("%d 件のパッケージを更新しました", result.UpdatedCount)
	} else {
		result.Message = "cargo パッケージを確認しました（更新なし）"
	}

	return result, nil
}

// ensureCargoUpdate は cargo-update がなければ自動インストールし、
// cargo-install-update のフルパスを返します。
func (c *CargoUpdater) ensureCargoUpdate(ctx context.Context) (string, error) {
	if path, err := exec.LookPath("cargo-install-update"); err == nil {
		return path, nil
	}

	fmt.Println("ℹ️  cargo-update が見つかりません。自動インストールします...")

	cmd := exec.CommandContext(ctx, "cargo", "install", "cargo-update")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("cargo-update のインストールに失敗: %w", err)
	}

	fmt.Println("✅ cargo-update のインストールが完了しました。")

	// PATH への反映を再確認
	if path, err := exec.LookPath("cargo-install-update"); err == nil {
		return path, nil
	}

	// PATH に反映されていない場合は CARGO_HOME/bin を直接確認
	path, err := cargoInstallUpdateBinPath()
	if err != nil {
		return "", fmt.Errorf(
			"cargo-update のインストールは成功しましたが、cargo-install-update が PATH に見つかりません。"+
				"~/.cargo/bin を PATH に追加してから再実行してください: %w", err)
	}

	return path, nil
}

// cargoInstallUpdateBinPath は CARGO_HOME/bin の cargo-install-update のパスを返します。
// PATH に ~/.cargo/bin が含まれていない環境での自動インストール後のフォールバックに使用します。
func cargoInstallUpdateBinPath() (string, error) {
	cargoHome := os.Getenv("CARGO_HOME")
	if cargoHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}

		cargoHome = filepath.Join(home, ".cargo")
	}

	binDir := filepath.Join(cargoHome, "bin")

	// 実行可能ファイルの候補（Windows では .exe/.cmd/.bat も確認）
	candidates := []string{"cargo-install-update"}
	if runtime.GOOS == windowsOS {
		candidates = []string{
			"cargo-install-update.exe",
			"cargo-install-update.cmd",
			"cargo-install-update.bat",
		}
	}

	for _, name := range candidates {
		path := filepath.Join(binDir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("%s に cargo-install-update が見つかりません", binDir)
}

// parseUpdateCount は "cargo install-update -a" の出力から実際に更新されたパッケージ数を返します。
// v20+: "Overall updated N package(s)." / v19-: "Updated N package(s)." のサマリ行を解析します。
func (c *CargoUpdater) parseUpdateCount(output string) int {
	output = strings.ReplaceAll(output, "\r\n", "\n")

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)

		// v20+: "Overall updated N package(s)."
		if strings.HasPrefix(line, "Overall updated ") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				if n, err := strconv.Atoi(parts[2]); err == nil {
					return n
				}
			}
		}

		// v19-: "Updated N package(s)."
		if strings.HasPrefix(line, "Updated ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				if n, err := strconv.Atoi(parts[1]); err == nil {
					return n
				}
			}
		}
	}

	return 0
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
