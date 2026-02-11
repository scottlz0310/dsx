package updater

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/scottlz0310/devsync/internal/config"
)

// FlatpakUpdater は Flatpak パッケージマネージャの実装です。
type FlatpakUpdater struct {
	useUser bool
}

// 起動時にレジストリへ登録します。
func init() {
	Register(&FlatpakUpdater{useUser: false})
}

func (f *FlatpakUpdater) Name() string {
	return "flatpak"
}

func (f *FlatpakUpdater) DisplayName() string {
	return "Flatpak"
}

func (f *FlatpakUpdater) IsAvailable() bool {
	_, err := exec.LookPath("flatpak")
	return err == nil
}

func (f *FlatpakUpdater) Configure(cfg config.ManagerConfig) error {
	if cfg == nil {
		return nil
	}

	if useUser, ok := cfg["use_user"].(bool); ok {
		f.useUser = useUser
		return nil
	}

	// 旧キー `user` との後方互換
	if useUser, ok := cfg["user"].(bool); ok {
		f.useUser = useUser
	}

	return nil
}

func (f *FlatpakUpdater) Check(ctx context.Context) (*CheckResult, error) {
	args := f.buildCommandArgs("remote-ls", "--updates", "--columns=application,version")
	cmd := exec.CommandContext(ctx, "flatpak", args...)

	cmd.Env = append(os.Environ(), "LANG=C", "LC_ALL=C")

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf(
			"flatpak remote-ls --updates の実行に失敗: %w",
			buildCommandOutputErr(err, combineCommandOutputs(output, stderr.Bytes())),
		)
	}

	packages := f.parseRemoteLSOutput(string(output))

	return &CheckResult{
		AvailableUpdates: len(packages),
		Packages:         packages,
	}, nil
}

func (f *FlatpakUpdater) Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	result := &UpdateResult{}

	checkResult, err := f.Check(ctx)
	if err != nil {
		return nil, err
	}

	if checkResult.AvailableUpdates == 0 {
		result.Message = "すべての Flatpak パッケージは最新です"
		return result, nil
	}

	if opts.DryRun {
		result.Message = fmt.Sprintf("%d 件の Flatpak パッケージが更新可能です（DryRunモード）", checkResult.AvailableUpdates)
		result.Packages = checkResult.Packages

		return result, nil
	}

	args := f.buildCommandArgs("update", "-y", "--noninteractive")
	cmd := exec.CommandContext(ctx, "flatpak", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		result.Errors = append(result.Errors, err)
		return result, fmt.Errorf("flatpak update に失敗: %w", err)
	}

	result.UpdatedCount = checkResult.AvailableUpdates
	result.Packages = checkResult.Packages
	result.Message = fmt.Sprintf("%d 件の Flatpak パッケージを更新しました", result.UpdatedCount)

	return result, nil
}

func (f *FlatpakUpdater) buildCommandArgs(args ...string) []string {
	if !f.useUser {
		return append([]string(nil), args...)
	}

	result := make([]string, 0, 1+len(args))
	result = append(result, "--user")
	result = append(result, args...)

	return result
}

func (f *FlatpakUpdater) parseRemoteLSOutput(output string) []PackageInfo {
	lines := strings.Split(output, "\n")
	packages := make([]PackageInfo, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "application id") {
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}

		pkg := PackageInfo{
			Name: fields[0],
		}
		if len(fields) > 1 {
			pkg.NewVersion = fields[1]
		}

		packages = append(packages, pkg)
	}

	return packages
}
