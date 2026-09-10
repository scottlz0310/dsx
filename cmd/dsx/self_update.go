package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/scottlz0310/dsx/internal/msix"
	"github.com/scottlz0310/dsx/internal/selfupdate"
	"github.com/spf13/cobra"
)

type selfUpdateInfo = selfupdate.Info
type semverCore = selfupdate.SemverCore

// selfUpdateMethod は実行環境に応じた dsx 本体の更新方法です。
type selfUpdateMethod int

const (
	// selfUpdateByGoInstall は go install で更新します（Windows 以外）。
	selfUpdateByGoInstall selfUpdateMethod = iota
	// selfUpdateByAppInstaller は MSIX 版で、.appinstaller による自動更新に任せます。
	selfUpdateByAppInstaller
	// selfUpdateUnsupported は Windows の MSIX 版以外です。Windows では go install による更新をサポートしません。
	selfUpdateUnsupported
)

var (
	selfUpdateCheckOnly bool

	selfUpdateCheckStep        = checkSelfUpdateAvailable
	selfUpdateApplyStep        = applySelfUpdate
	selfUpdateFetchReleaseStep = func(ctx context.Context) (string, string, error) {
		return selfupdate.FetchLatestRelease(ctx, version)
	}
	selfUpdateGOOS         = runtime.GOOS
	selfUpdatePackagedStep = msix.IsPackaged
)

func selfUpdateInstallTarget(version string) string {
	return selfupdate.InstallTarget(version)
}

var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "dsx 本体を更新します",
	Long: `dsx 本体の更新確認と更新適用を行います。

既定では更新確認後に更新を実行します。
確認のみ行う場合は --check を指定してください。

Windows では MSIX 版のみをサポートし、更新は .appinstaller による自動更新で行います。
go install で導入した dsx は、MSIX 版への移行を案内します。`,
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

	method, err := resolveSelfUpdateMethod()
	if err != nil {
		return err
	}

	switch method {
	case selfUpdateByAppInstaller:
		fmt.Println("ℹ️  MSIX 版は .appinstaller により自動更新されます。")
		fmt.Printf("   すぐに更新する場合は PowerShell で次を実行してください:\n   %s\n", msix.InstallCommand)

		return nil
	case selfUpdateUnsupported:
		return fmt.Errorf("go install による self-update は Windows ではサポートしていません。PowerShell で次を実行して MSIX 版へ移行してください: %s", msix.InstallCommand)
	case selfUpdateByGoInstall:
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

	// 通知は補助情報のため、判定に失敗した場合は更新方法の案内だけを省略する
	if method, err := resolveSelfUpdateMethod(); err == nil {
		fmt.Printf("   %s\n", selfUpdateHint(method))
	}

	if info.ReleaseURL != "" {
		fmt.Printf("   リリース情報: %s\n", info.ReleaseURL)
	}
}

func resolveSelfUpdateMethod() (selfUpdateMethod, error) {
	if selfUpdateGOOS != "windows" {
		return selfUpdateByGoInstall, nil
	}

	packaged, err := selfUpdatePackagedStep()
	if err != nil {
		return selfUpdateUnsupported, err
	}

	if packaged {
		return selfUpdateByAppInstaller, nil
	}

	return selfUpdateUnsupported, nil
}

func selfUpdateHint(method selfUpdateMethod) string {
	switch method {
	case selfUpdateByAppInstaller:
		return "MSIX 版は自動更新されます（すぐに更新する場合: " + msix.InstallCommand + "）"
	case selfUpdateUnsupported:
		return "MSIX 版への移行が必要です: " + msix.InstallCommand
	default:
		return "更新コマンド: dsx self-update"
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
