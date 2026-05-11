package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/scottlz0310/dsx/internal/selfupdate"
	"github.com/spf13/cobra"
)

type selfUpdateInfo = selfupdate.Info
type semverCore = selfupdate.SemverCore

var (
	selfUpdateCheckOnly bool

	selfUpdateCheckStep        = checkSelfUpdateAvailable
	selfUpdateApplyStep        = applySelfUpdate
	selfUpdateFetchReleaseStep = func(ctx context.Context) (string, string, error) {
		return selfupdate.FetchLatestRelease(ctx, version)
	}
)

func selfUpdateInstallTarget(version string) string {
	return selfupdate.InstallTarget(version)
}

var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "dsx 本体を更新します",
	Long: `dsx 本体の更新確認と更新適用を行います。

既定では更新確認後に更新を実行します。
確認のみ行う場合は --check を指定してください。`,
	RunE: runSelfUpdate,
}

func init() {
	rootCmd.AddCommand(selfUpdateCmd)
	selfUpdateCmd.Flags().BoolVar(&selfUpdateCheckOnly, "check", false, "更新確認のみ行う（更新は適用しない）")
}

func runSelfUpdate(cmd *cobra.Command, args []string) error {
	info, err := selfUpdateCheckStep(context.Background(), version)
	if err != nil {
		return fmt.Errorf("更新確認に失敗しました: %w", err)
	}

	if info == nil {
		if isDevelopmentBuildVersion(version) {
			fmt.Printf("ℹ️  開発版（%s）のため更新比較をスキップしました。\n", version)
		} else {
			fmt.Printf("✅ すでに最新です（%s）\n", version)
		}

		return nil
	}

	fmt.Printf("🆕 新しいバージョン %s が利用可能です（現在: %s）\n", info.LatestVersion, info.CurrentVersion)

	if info.ReleaseURL != "" {
		fmt.Printf("   リリース情報: %s\n", info.ReleaseURL)
	}

	if selfUpdateCheckOnly {
		return nil
	}

	fmt.Println("🔄 self-update を実行します...")

	applyCtx := cmd.Context()
	if applyCtx == nil {
		applyCtx = context.Background()
	}

	if err := selfUpdateApplyStep(applyCtx, info.LatestVersion); err != nil {
		return err
	}

	fmt.Println("✅ self-update が完了しました。")
	fmt.Println("💡 新しいシェルで `dsx --version` を確認してください。")

	return nil
}

func printSelfUpdateNoticeAtEnd() {
	info, err := selfUpdateCheckStep(context.Background(), version)
	if err != nil || info == nil {
		return
	}

	fmt.Println()
	fmt.Printf("🆕 dsx の新しいバージョン %s が利用可能です（現在: %s）\n", info.LatestVersion, info.CurrentVersion)
	fmt.Println("   更新コマンド: dsx self-update")

	if info.ReleaseURL != "" {
		fmt.Printf("   リリース情報: %s\n", info.ReleaseURL)
	}
}

func checkSelfUpdateAvailable(ctx context.Context, currentVersion string) (*selfUpdateInfo, error) {
	return selfupdate.CheckAvailable(ctx, currentVersion, selfUpdateFetchReleaseStep)
}

func applySelfUpdate(ctx context.Context, version string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	target := selfUpdateInstallTarget(version)
	cmd := exec.CommandContext(ctx, "go", "install", target)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		if runtime.GOOS == "windows" {
			return fmt.Errorf(
				"self-update に失敗しました（実行中バイナリの置換競合の可能性があります）。別シェルで `go install %s` を実行してください: %w",
				target,
				err,
			)
		}

		return fmt.Errorf("self-update に失敗しました: %w", err)
	}

	return nil
}

func isDevelopmentBuildVersion(v string) bool {
	return selfupdate.IsDevelopmentBuildVersion(v)
}

func parseSemverCore(v string) (semverCore, bool) {
	return selfupdate.ParseSemverCore(v)
}

func compareSemverCore(left, right semverCore) int {
	return selfupdate.CompareSemverCore(left, right)
}
