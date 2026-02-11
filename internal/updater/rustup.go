package updater

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/scottlz0310/devsync/internal/config"
)

// RustupUpdater は rustup (Rust ツールチェーン) の実装です。
type RustupUpdater struct{}

// 起動時にレジストリに登録
func init() {
	Register(&RustupUpdater{})
}

func (r *RustupUpdater) Name() string {
	return "rustup"
}

func (r *RustupUpdater) DisplayName() string {
	return "rustup (Rust ツールチェーン)"
}

func (r *RustupUpdater) IsAvailable() bool {
	_, err := exec.LookPath("rustup")
	return err == nil
}

func (r *RustupUpdater) Configure(cfg config.ManagerConfig) error {
	// 現時点では設定項目なし
	return nil
}

func (r *RustupUpdater) Check(ctx context.Context) (*CheckResult, error) {
	output, err := runCommandOutputWithLocaleC(ctx, "rustup", []string{"check"}, "rustup check の実行に失敗: %w")
	if err != nil {
		return nil, err
	}

	packages := r.parseCheckOutput(string(output))

	return &CheckResult{
		AvailableUpdates: len(packages),
		Packages:         packages,
	}, nil
}

func (r *RustupUpdater) Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	checkResult, err := r.Check(ctx)
	if err != nil {
		return nil, err
	}

	return runCountBasedUpdate(
		ctx,
		opts,
		checkResult,
		"rustup で更新可能なツールチェーンはありません",
		func(count int) string {
			return fmt.Sprintf("%d 件の Rust ツールチェーン更新が可能です（DryRunモード）", count)
		},
		"rustup",
		[]string{"update"},
		"rustup update に失敗: %w",
		func(count int) string {
			return fmt.Sprintf("%d 件の Rust ツールチェーンを更新しました", count)
		},
	)
}

func (r *RustupUpdater) parseCheckOutput(output string) []PackageInfo {
	lines := strings.Split(output, "\n")
	packages := make([]PackageInfo, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if !strings.Contains(strings.ToLower(trimmed), "update available") {
			continue
		}

		name, current, next, ok := parseRustupUpdateLine(trimmed)
		if !ok {
			continue
		}

		packages = append(packages, PackageInfo{
			Name:           name,
			CurrentVersion: current,
			NewVersion:     next,
		})
	}

	return packages
}

func parseRustupUpdateLine(line string) (name, current, next string, ok bool) {
	parts := strings.SplitN(line, " - ", 2)
	if len(parts) != 2 {
		return "", "", "", false
	}

	name = strings.TrimSpace(parts[0])
	if name == "" {
		return "", "", "", false
	}

	arrowParts := strings.SplitN(parts[1], "->", 2)
	if len(arrowParts) != 2 {
		return "", "", "", false
	}

	left := arrowParts[0]
	right := arrowParts[1]

	colonParts := strings.SplitN(left, ":", 2)
	if len(colonParts) != 2 {
		return "", "", "", false
	}

	current = normalizeRustupVersion(colonParts[1])
	next = normalizeRustupVersion(right)

	if current == "" || next == "" {
		return "", "", "", false
	}

	return name, current, next, true
}

func normalizeRustupVersion(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return ""
	}

	return strings.TrimPrefix(fields[0], "v")
}
