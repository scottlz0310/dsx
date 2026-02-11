package updater

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/scottlz0310/devsync/internal/config"
)

// GemUpdater は gem (Ruby Gems) の実装です。
type GemUpdater struct{}

// 起動時にレジストリに登録
func init() {
	Register(&GemUpdater{})
}

func (g *GemUpdater) Name() string {
	return "gem"
}

func (g *GemUpdater) DisplayName() string {
	return "gem (Ruby Gems)"
}

func (g *GemUpdater) IsAvailable() bool {
	_, err := exec.LookPath("gem")
	return err == nil
}

func (g *GemUpdater) Configure(cfg config.ManagerConfig) error {
	// 現時点では設定項目なし
	return nil
}

func (g *GemUpdater) Check(ctx context.Context) (*CheckResult, error) {
	output, err := runCommandOutputWithLocaleC(ctx, "gem", []string{"outdated"}, "gem outdated の実行に失敗: %w")
	if err != nil {
		return nil, err
	}

	packages := g.parseOutdatedOutput(string(output))

	return &CheckResult{
		AvailableUpdates: len(packages),
		Packages:         packages,
	}, nil
}

func (g *GemUpdater) Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	checkResult, err := g.Check(ctx)
	if err != nil {
		return nil, err
	}

	return runCountBasedUpdate(
		ctx,
		opts,
		checkResult,
		"gem で更新可能なパッケージはありません",
		func(count int) string {
			return fmt.Sprintf("%d 件の gem パッケージが更新可能です（DryRunモード）", count)
		},
		"gem",
		[]string{"update"},
		"gem update に失敗: %w",
		func(count int) string {
			return fmt.Sprintf("%d 件の gem パッケージを更新しました", count)
		},
	)
}

func (g *GemUpdater) parseOutdatedOutput(output string) []PackageInfo {
	lines := strings.Split(output, "\n")
	packages := make([]PackageInfo, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		name, current, next, ok := parseGemOutdatedLine(trimmed)
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

func parseGemOutdatedLine(line string) (name, current, next string, ok bool) {
	openIdx := strings.Index(line, "(")

	closeIdx := strings.LastIndex(line, ")")
	if openIdx <= 0 || closeIdx <= openIdx {
		return "", "", "", false
	}

	name = strings.TrimSpace(line[:openIdx])
	if name == "" {
		return "", "", "", false
	}

	body := strings.TrimSpace(line[openIdx+1 : closeIdx])

	parts := strings.SplitN(body, "<", 2)
	if len(parts) != 2 {
		return "", "", "", false
	}

	current = normalizeGemVersion(parts[0])
	next = normalizeGemVersion(parts[1])

	if current == "" || next == "" {
		return "", "", "", false
	}

	return name, current, next, true
}

func normalizeGemVersion(value string) string {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return ""
	}

	segments := strings.Split(cleaned, ",")
	for _, segment := range segments {
		token := strings.TrimSpace(segment)
		if token == "" {
			continue
		}

		if strings.Contains(token, ":") {
			fields := strings.SplitN(token, ":", 2)
			if len(fields) == 2 {
				token = strings.TrimSpace(fields[1])
			}
		}

		token = strings.TrimPrefix(token, "v")
		if looksLikeVersionToken(token) {
			return token
		}
	}

	return ""
}
