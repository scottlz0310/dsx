package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"time"

	"github.com/scottlz0310/dsx/internal/config"
	"github.com/scottlz0310/dsx/internal/runner"
	"github.com/scottlz0310/dsx/internal/updater"
	"github.com/spf13/cobra"
)

var (
	sysDryRun  bool
	sysVerbose bool
	sysJobs    int
	sysTimeout string
	sysTUI     bool
	sysNoTUI   bool
	sysLogFile string
)

// sysCmd はシステム関連コマンドのルートです
var sysCmd = &cobra.Command{
	Use:   "sys",
	Short: "システムパッケージの管理",
	Long:  `システムパッケージの更新・管理を行うサブコマンド群です。`,
}

// sysUpdateCmd はシステムパッケージを更新するコマンドです
var sysUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "システムパッケージを更新します",
	Long: `設定ファイルで有効化されたパッケージマネージャを使用して、
システムパッケージを最新版に更新します。

対応マネージャ:
  - apt       (Debian/Ubuntu)
  - brew      (macOS/Linux Homebrew)
  - go        (Go ツール)
  - npm       (Node.js グローバルパッケージ)
  - pnpm      (Node.js グローバルパッケージ)
  - nvm       (Node.js バージョン管理)
  - snap      (Ubuntu Snap パッケージ)
  - flatpak   (Linux Flatpak)
  - fwupdmgr  (Linux Firmware)
  - pipx      (Python CLI ツール)
  - cargo     (Rust ツール)
  - uv        (Python CLI ツール)
  - rustup    (Rust ツールチェーン)
  - gem       (Ruby Gems)

例:
  dsx sys update           # 設定に基づいて更新
  dsx sys update --dry-run # 更新計画のみ表示
  dsx sys update -v        # 詳細ログを表示
  dsx sys update --jobs 4  # 4並列で更新`,
	RunE: runSysUpdate,
}

// sysListCmd は利用可能なマネージャを一覧表示します
var sysListCmd = &cobra.Command{
	Use:   "list",
	Short: "利用可能なパッケージマネージャを一覧表示します",
	RunE:  runSysList,
}

func init() {
	rootCmd.AddCommand(sysCmd)
	sysCmd.AddCommand(sysUpdateCmd)
	sysCmd.AddCommand(sysListCmd)

	// フラグの定義
	sysUpdateCmd.Flags().BoolVarP(&sysDryRun, "dry-run", "n", false, "実際の更新は行わず、計画のみ表示")
	sysUpdateCmd.Flags().BoolVarP(&sysVerbose, "verbose", "v", false, "詳細なログを出力")
	sysUpdateCmd.Flags().IntVarP(&sysJobs, "jobs", "j", 0, "並列実行数（0以下の場合は設定値または1を使用）")
	sysUpdateCmd.Flags().StringVarP(&sysTimeout, "timeout", "t", "10m", "全体のタイムアウト時間")
	sysUpdateCmd.Flags().BoolVar(&sysTUI, "tui", false, "Bubble Tea の進捗UIを表示（既定値は config.yaml の ui.tui）")
	sysUpdateCmd.Flags().BoolVar(&sysNoTUI, "no-tui", false, "TUI 進捗表示を無効化（設定より優先）")
	sysUpdateCmd.Flags().StringVar(&sysLogFile, "log-file", "", "ジョブ実行ログをファイルに保存")
}

func runSysUpdate(cmd *cobra.Command, args []string) error {
	defer printSelfUpdateNoticeAtEnd()

	// 設定の読み込み
	cfg, opts := loadSysUpdateConfig(cmd)

	// コンテキストの作成（タイムアウト + キャンセル対応）
	ctx, cancel := setupContext()
	defer cancel()

	// 有効なマネージャを取得
	enabledUpdaters, err := updater.GetEnabled(&cfg.Sys)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  %v\n", err)
	}

	configureUpdaterRuntimeVersion(enabledUpdaters, version)

	tuiReq, err := resolveTUIRequest(cfg.UI.TUI, cmd.Flags().Changed("tui"), sysTUI, cmd.Flags().Changed("no-tui"), sysNoTUI)
	if err != nil {
		return err
	}

	useTUI, warning := resolveTUIEnabled(tuiReq)
	printTUIWarning(warning)

	// 有効なマネージャがない場合は利用可能なものを表示
	if len(enabledUpdaters) == 0 {
		printNoTargetTUIMessage(tuiReq, "sys update")
		printNoManagerHelp()

		return nil
	}

	// TUI 使用時は開始メッセージを抑制（TUI が画面を制御するため）
	if !useTUI {
		fmt.Println("🔄 システムパッケージの更新を開始します...")
		fmt.Println()
	}

	if !useTUI {
		printSysUpdateDryRunNotice(opts.DryRun)
	}

	jobs := resolveSysJobs(cfg.Control.Concurrency, sysJobs)

	stats, err := runSysUpdatePhases(ctx, cfg, opts, enabledUpdaters, jobs, useTUI)
	if err != nil {
		return err
	}

	// TUI 使用時は TUI 側で完了サマリーを表示済みのため、テキストサマリーは非 TUI 時のみ出力
	if !useTUI {
		printUpdateSummary(stats)
	}

	// 失敗ジョブのエラー詳細を表示
	printFailedErrors(stats.Errors)

	if len(stats.Errors) > 0 {
		return fmt.Errorf("%d 件のエラーが発生しました", len(stats.Errors))
	}

	if !useTUI {
		fmt.Println()
		fmt.Println("✅ システムパッケージの更新が完了しました")
	}

	return nil
}

// runSysUpdatePhases はマネージャ本体更新・単独実行・並列実行の各フェーズを実行します。
func runSysUpdatePhases(ctx context.Context, cfg *config.Config, opts updater.UpdateOptions, enabledUpdaters []updater.Updater, jobs int, useTUI bool) (updateStats, error) {
	var stats updateStats

	remainingUpdaters, selfUpdateStats := runManagerSelfUpdatePhase(ctx, opts, enabledUpdaters, useTUI)
	mergeUpdateStats(&stats, selfUpdateStats)

	exclusiveUpdaters, parallelUpdaters := splitUpdatersForExecution(remainingUpdaters)

	if len(exclusiveUpdaters) > 0 {
		if err := runExclusivePhase(ctx, cfg, opts, exclusiveUpdaters, useTUI, &stats); err != nil {
			return stats, err
		}
	}

	if len(parallelUpdaters) > 0 {
		if err := runParallelPhase(ctx, cfg, opts, parallelUpdaters, jobs, useTUI, &stats); err != nil {
			return stats, err
		}
	}

	return stats, nil
}

type managerSelfUpdateTarget struct {
	updater updater.Updater
	self    updater.ManagerSelfUpdater
}

func runManagerSelfUpdatePhase(ctx context.Context, opts updater.UpdateOptions, updaters []updater.Updater, useTUI bool) ([]updater.Updater, updateStats) {
	targets := collectManagerSelfUpdateTargets(updaters)
	if len(targets) == 0 {
		return updaters, updateStats{}
	}

	if useTUI {
		return runManagerSelfUpdatePhaseWithTUI(ctx, opts, updaters, targets)
	}

	printManagerSelfUpdatePhaseHeader()

	var stats updateStats

	skipNormalUpdate := make(map[string]bool)

	for _, target := range targets {
		select {
		case <-ctx.Done():
			stats.Errors = append(stats.Errors, fmt.Errorf("マネージャ本体更新フェーズがタイムアウトまたはキャンセルされました"))

			return filterNormalUpdateUpdaters(updaters, skipNormalUpdate), stats
		default:
		}

		printManagerSelfUpdateHeader(target.updater)

		result, err := executeManagerSelfUpdate(ctx, target.self, opts)
		if err != nil {
			recordManagerSelfUpdateError(target.updater, err, &stats)
			fmt.Fprintf(os.Stderr, "❌ エラー: %v\n", err)
			fmt.Println()

			continue
		}

		printUpdaterResult(&result.UpdateResult)
		fmt.Println()

		mergeSelfUpdateResult(&stats, result)

		if !result.ShouldContinueNormalUpdate() {
			skipNormalUpdate[target.updater.Name()] = true
		}
	}

	return filterNormalUpdateUpdaters(updaters, skipNormalUpdate), stats
}

func runManagerSelfUpdatePhaseWithTUI(ctx context.Context, opts updater.UpdateOptions, updaters []updater.Updater, targets []managerSelfUpdateTarget) ([]updater.Updater, updateStats) {
	var (
		stats            updateStats
		statsMu          sync.Mutex
		skipNormalUpdate = make(map[string]bool)
	)

	jobs := make([]runner.Job, 0, len(targets))
	for _, item := range targets {
		target := item

		jobs = append(jobs, runner.Job{
			Name: target.updater.Name() + "-self-update",
			Run: func(jobCtx context.Context) error {
				result, err := executeManagerSelfUpdate(jobCtx, target.self, opts)
				if err != nil {
					if isContextCancellation(err) {
						return err
					}

					statsMu.Lock()
					recordManagerSelfUpdateError(target.updater, err, &stats)
					statsMu.Unlock()

					return err
				}

				statsMu.Lock()
				mergeSelfUpdateResult(&stats, result)

				if !result.ShouldContinueNormalUpdate() {
					skipNormalUpdate[target.updater.Name()] = true
				}
				statsMu.Unlock()

				return nil
			},
		})
	}

	summary := runJobsWithOptionalTUI(ctx, "マネージャ本体更新 進捗", 1, jobs, true, sysLogFile)
	if summary.Skipped > 0 {
		stats.Errors = append(stats.Errors, fmt.Errorf("キャンセルまたはタイムアウトによりマネージャ本体更新 %d 件をスキップしました", summary.Skipped))
	}

	return filterNormalUpdateUpdaters(updaters, skipNormalUpdate), stats
}

func collectManagerSelfUpdateTargets(updaters []updater.Updater) []managerSelfUpdateTarget {
	targets := make([]managerSelfUpdateTarget, 0, len(updaters))

	for _, u := range updaters {
		self, ok := u.(updater.ManagerSelfUpdater)
		if !ok {
			continue
		}

		targets = append(targets, managerSelfUpdateTarget{updater: u, self: self})
	}

	return targets
}

func executeManagerSelfUpdate(ctx context.Context, self updater.ManagerSelfUpdater, opts updater.UpdateOptions) (*updater.SelfUpdateResult, error) {
	if !opts.DryRun {
		result, err := self.SelfUpdate(ctx, opts)
		if result == nil {
			result = &updater.SelfUpdateResult{Continuation: updater.ContinueNormalUpdate}
		}

		return result, err
	}

	checkResult, err := self.CheckSelfUpdate(ctx)
	if err != nil {
		return nil, err
	}

	return buildDryRunSelfUpdateResult(checkResult), nil
}

func buildDryRunSelfUpdateResult(checkResult *updater.CheckResult) *updater.SelfUpdateResult {
	result := &updater.SelfUpdateResult{
		Continuation: updater.ContinueNormalUpdate,
	}

	if checkResult == nil {
		result.Message = "マネージャ本体更新を確認しました（DryRunモード）"

		return result
	}

	result.Packages = checkResult.Packages
	if checkResult.AvailableUpdates > 0 {
		result.Message = fmt.Sprintf("%d 件のマネージャ本体更新が可能です（DryRunモード）", checkResult.AvailableUpdates)
		if checkResult.Message != "" {
			result.Message = checkResult.Message + "（DryRunモード）"
		}

		return result
	}

	if checkResult.Message != "" {
		result.Message = checkResult.Message + "（DryRunモード）"
	} else {
		result.Message = "マネージャ本体は最新です（DryRunモード）"
	}

	return result
}

func mergeSelfUpdateResult(stats *updateStats, result *updater.SelfUpdateResult) {
	if result == nil {
		return
	}

	stats.Updated += result.UpdatedCount
	stats.Failed += result.FailedCount
	stats.Errors = append(stats.Errors, result.Errors...)
}

func recordManagerSelfUpdateError(u updater.Updater, err error, stats *updateStats) {
	stats.Errors = append(stats.Errors, fmt.Errorf("%s 本体更新: %w", u.Name(), err))
	stats.Failed++
}

func filterNormalUpdateUpdaters(updaters []updater.Updater, skip map[string]bool) []updater.Updater {
	if len(skip) == 0 {
		return updaters
	}

	filtered := make([]updater.Updater, 0, len(updaters))
	for _, u := range updaters {
		if skip[u.Name()] {
			continue
		}

		filtered = append(filtered, u)
	}

	return filtered
}

func runExclusivePhase(ctx context.Context, cfg *config.Config, opts updater.UpdateOptions, updaters []updater.Updater, useTUI bool, stats *updateStats) error {
	if phaseRequiresSudo(updaters, cfg.Sys.Managers) {
		if err := ensureSudoAuthentication(ctx, "単独実行フェーズ", useTUI); err != nil {
			return err
		}

		if !useTUI {
			fmt.Println()
		}
	}

	if !useTUI {
		fmt.Println("🔒 依存関係の都合で単独実行するマネージャがあります（apt）。")
		fmt.Println()
	}

	if useTUI {
		mergeUpdateStats(stats, executeUpdatesParallel(ctx, updaters, opts, 1, true))
	} else {
		mergeUpdateStats(stats, executeUpdates(ctx, updaters, opts))
	}

	return nil
}

func runParallelPhase(ctx context.Context, cfg *config.Config, opts updater.UpdateOptions, updaters []updater.Updater, jobs int, useTUI bool, stats *updateStats) error {
	if phaseRequiresSudo(updaters, cfg.Sys.Managers) {
		if err := ensureSudoAuthentication(ctx, "並列実行フェーズ", useTUI); err != nil {
			return err
		}

		if !useTUI {
			fmt.Println()
		}
	}

	mergeUpdateStats(stats, executeParallelUpdaters(ctx, updaters, opts, jobs, useTUI))

	return nil
}

func printSysUpdateDryRunNotice(dryRun bool) {
	if !dryRun {
		return
	}

	fmt.Println("📋 DryRun モード: 実際の更新は行いません")
	fmt.Println()
}

func printManagerSelfUpdatePhaseHeader() {
	fmt.Println("🧰 マネージャ本体更新フェーズを開始します...")
	fmt.Println()
}

func printManagerSelfUpdateHeader(u updater.Updater) {
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("🧰 %s 本体更新 (%s)\n", u.DisplayName(), u.Name())
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
}

// updateStats は更新処理の統計情報を保持します。
type updateStats struct {
	Updated int
	Failed  int
	Errors  []error
}

// loadSysUpdateConfig は設定とオプションを読み込みます。
func loadSysUpdateConfig(cmd *cobra.Command) (*config.Config, updater.UpdateOptions) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  設定ファイルの読み込みに失敗（デフォルト設定を使用）: %v\n", err)

		cfg = config.Default()
	}

	// DryRun フラグの適用（コマンドラインが優先）
	if cmd.Flags().Changed("dry-run") {
		cfg.Control.DryRun = sysDryRun
	}

	opts := updater.UpdateOptions{
		DryRun:         cfg.Control.DryRun,
		Verbose:        sysVerbose,
		CurrentVersion: version,
	}

	return cfg, opts
}

// setupContext はタイムアウトとシグナルハンドリング付きのコンテキストを作成します。
func setupContext() (context.Context, context.CancelFunc) {
	timeout, err := time.ParseDuration(sysTimeout)
	if err != nil {
		timeout = 10 * time.Minute
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	// Ctrl+C でキャンセル可能に
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		<-sigCh
		fmt.Println("\n⚠️  中断シグナルを受信しました。処理を終了します...")
		cancel()
	}()

	return ctx, cancel
}

// printNoManagerHelp はマネージャが未設定の場合のヘルプを表示します。
func printNoManagerHelp() {
	fmt.Println("📝 有効化されたマネージャがありません。")
	fmt.Println()
	fmt.Println("利用可能なマネージャ:")

	for _, u := range updater.Available() {
		fmt.Printf("  - %s (%s)\n", u.Name(), u.DisplayName())
	}

	fmt.Println()
	fmt.Println("💡 config.yaml の sys.enable で使用するマネージャを指定してください。")
	fmt.Println("   例: enable: [\"apt\", \"go\"]")
}

// executeUpdates は各マネージャで更新を実行し、統計を返します。
func executeUpdates(ctx context.Context, updaters []updater.Updater, opts updater.UpdateOptions) updateStats {
	var stats updateStats

	for _, u := range updaters {
		select {
		case <-ctx.Done():
			stats.Errors = append(stats.Errors, fmt.Errorf("タイムアウトまたはキャンセルされました"))

			return stats
		default:
		}

		printUpdaterHeader(u)

		result, err := u.Update(ctx, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ エラー: %v\n", err)
			stats.Errors = append(stats.Errors, fmt.Errorf("%s: %w", u.Name(), err))
			stats.Failed++

			continue
		}

		printUpdaterResult(result)
		stats.Updated += result.UpdatedCount
		stats.Failed += result.FailedCount
		stats.Errors = append(stats.Errors, result.Errors...)

		fmt.Println()
	}

	return stats
}

func executeParallelUpdaters(ctx context.Context, updaters []updater.Updater, opts updater.UpdateOptions, jobs int, useTUI bool) updateStats {
	switch {
	case useTUI:
		parallelJobs := jobs
		if parallelJobs <= 0 {
			parallelJobs = 1
		}

		return executeUpdatesParallel(ctx, updaters, opts, parallelJobs, true)
	case jobs > 1:
		fmt.Printf("⚡ %d 並列で更新します。\n", jobs)
		fmt.Println()
		return executeUpdatesParallel(ctx, updaters, opts, jobs, false)
	default:
		return executeUpdates(ctx, updaters, opts)
	}
}

// executeUpdatesParallel はマネージャ更新を並列実行し、統計を返します。
func executeUpdatesParallel(ctx context.Context, updaters []updater.Updater, opts updater.UpdateOptions, jobs int, useTUI bool) updateStats {
	var (
		stats    updateStats
		statsMu  sync.Mutex
		outputMu sync.Mutex
	)

	execJobs := buildUpdaterJobs(updaters, opts, useTUI, &stats, &statsMu, &outputMu)
	summary := runJobsWithOptionalTUI(ctx, "sys update 進捗", jobs, execJobs, useTUI, sysLogFile)

	if summary.Skipped > 0 {
		stats.Errors = append(stats.Errors, fmt.Errorf("キャンセルまたはタイムアウトにより %d 件をスキップしました", summary.Skipped))
	}

	return stats
}

func buildUpdaterJobs(updaters []updater.Updater, opts updater.UpdateOptions, useTUI bool, stats *updateStats, statsMu, outputMu *sync.Mutex) []runner.Job {
	execJobs := make([]runner.Job, 0, len(updaters))

	for _, updaterItem := range updaters {
		u := updaterItem

		execJobs = append(execJobs, runner.Job{
			Name: u.Name(),
			Run: func(jobCtx context.Context) error {
				return runUpdaterJob(jobCtx, u, opts, useTUI, stats, statsMu, outputMu)
			},
		})
	}

	return execJobs
}

func runUpdaterJob(jobCtx context.Context, u updater.Updater, opts updater.UpdateOptions, useTUI bool, stats *updateStats, statsMu, outputMu *sync.Mutex) error {
	printUpdaterHeaderIfNeeded(u, useTUI, outputMu)

	result, err := u.Update(jobCtx, opts)
	if err != nil {
		return handleUpdaterError(u, err, useTUI, stats, statsMu, outputMu)
	}

	printUpdaterResultIfNeeded(result, useTUI, outputMu)
	mergeUpdaterResult(stats, statsMu, result)

	return nil
}

func printUpdaterHeaderIfNeeded(u updater.Updater, useTUI bool, outputMu *sync.Mutex) {
	if useTUI {
		return
	}

	outputMu.Lock()
	printUpdaterHeader(u)
	outputMu.Unlock()
}

func handleUpdaterError(u updater.Updater, err error, useTUI bool, stats *updateStats, statsMu, outputMu *sync.Mutex) error {
	if isContextCancellation(err) {
		return err
	}

	if !useTUI {
		outputMu.Lock()
		fmt.Fprintf(os.Stderr, "❌ エラー: %v\n", err)
		outputMu.Unlock()
	}

	statsMu.Lock()

	stats.Errors = append(stats.Errors, fmt.Errorf("%s: %w", u.Name(), err))
	stats.Failed++

	statsMu.Unlock()

	return err
}

func printUpdaterResultIfNeeded(result *updater.UpdateResult, useTUI bool, outputMu *sync.Mutex) {
	if useTUI {
		return
	}

	outputMu.Lock()
	printUpdaterResult(result)
	fmt.Println()
	outputMu.Unlock()
}

func mergeUpdaterResult(stats *updateStats, statsMu *sync.Mutex, result *updater.UpdateResult) {
	statsMu.Lock()

	stats.Updated += result.UpdatedCount
	stats.Failed += result.FailedCount
	stats.Errors = append(stats.Errors, result.Errors...)

	statsMu.Unlock()
}

func resolveSysJobs(configJobs, flagJobs int) int {
	if flagJobs > 0 {
		return flagJobs
	}

	if configJobs > 0 {
		return configJobs
	}

	return 1
}

type runtimeVersionConfigurer interface {
	ConfigureRuntimeVersion(string)
}

func configureUpdaterRuntimeVersion(updaters []updater.Updater, currentVersion string) {
	for _, u := range updaters {
		configurer, ok := u.(runtimeVersionConfigurer)
		if !ok {
			continue
		}

		configurer.ConfigureRuntimeVersion(currentVersion)
	}
}

func splitUpdatersForExecution(updaters []updater.Updater) (exclusive, parallel []updater.Updater) {
	exclusive = make([]updater.Updater, 0, len(updaters))
	parallel = make([]updater.Updater, 0, len(updaters))

	for _, u := range updaters {
		if mustRunExclusively(u) {
			exclusive = append(exclusive, u)
			continue
		}

		parallel = append(parallel, u)
	}

	return exclusive, parallel
}

func isContextCancellation(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func mustRunExclusively(u updater.Updater) bool {
	return u.Name() == "apt"
}

func mergeUpdateStats(dst *updateStats, src updateStats) {
	dst.Updated += src.Updated
	dst.Failed += src.Failed
	dst.Errors = append(dst.Errors, src.Errors...)
}

func phaseRequiresSudo(updaters []updater.Updater, managers map[string]config.ManagerConfig) bool {
	for _, u := range updaters {
		if updaterRequiresSudo(u.Name(), managers) {
			return true
		}
	}

	return false
}

func updaterRequiresSudo(name string, managers map[string]config.ManagerConfig) bool {
	if !isSudoManagedUpdater(name) {
		return false
	}

	useSudo, configured := resolveManagerUseSudo(name, managers)
	if configured {
		return useSudo
	}

	return true
}

func isSudoManagedUpdater(name string) bool {
	return name == "apt" || name == "snap"
}

func resolveManagerUseSudo(name string, managers map[string]config.ManagerConfig) (useSudo, configured bool) {
	if managers == nil {
		return false, false
	}

	managerCfg, ok := managers[name]
	if !ok {
		return false, false
	}

	if value, ok := managerCfg["use_sudo"].(bool); ok {
		return value, true
	}

	if value, ok := managerCfg["sudo"].(bool); ok {
		return value, true
	}

	return false, false
}

func ensureSudoAuthentication(ctx context.Context, phase string, suppressOutput bool) error {
	if !suppressOutput {
		fmt.Printf("🔐 sudo 認証を確認します（%s）...\n", phase)
	}

	cmd := exec.CommandContext(ctx, "sudo", "-v")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo 認証に失敗しました（%s）: %w", phase, err)
	}

	if !suppressOutput {
		fmt.Println("✅ sudo 認証を確認しました。")
	}

	return nil
}

// printUpdaterHeader はマネージャのヘッダーを表示します。
func printUpdaterHeader(u updater.Updater) {
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("📦 %s (%s)\n", u.DisplayName(), u.Name())
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
}

// printUpdaterResult は更新結果を表示します。
func printUpdaterResult(result *updater.UpdateResult) {
	if result.Message != "" {
		fmt.Printf("✅ %s\n", result.Message)
	}

	if sysVerbose && len(result.Packages) > 0 {
		fmt.Println("  更新パッケージ:")

		for _, pkg := range result.Packages {
			if pkg.CurrentVersion != "" {
				fmt.Printf("    - %s: %s → %s\n", pkg.Name, pkg.CurrentVersion, pkg.NewVersion)
			} else {
				fmt.Printf("    - %s %s\n", pkg.Name, pkg.NewVersion)
			}
		}
	}

	if len(result.Errors) > 0 {
		for _, e := range result.Errors {
			fmt.Fprintf(os.Stderr, "  ⚠️  %v\n", e)
		}
	}
}

// printUpdateSummary は更新サマリーを表示します。
func printUpdateSummary(stats updateStats) {
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("📊 更新サマリー")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  更新成功: %d 件\n", stats.Updated)

	if stats.Failed > 0 {
		fmt.Printf("  失敗: %d 件\n", stats.Failed)
	}

	if len(stats.Errors) > 0 {
		fmt.Printf("  エラー数: %d\n", len(stats.Errors))
	}
}

func runSysList(cmd *cobra.Command, args []string) error {
	fmt.Println("📋 パッケージマネージャ一覧")
	fmt.Println()

	// 設定の読み込み
	cfg, err := config.Load()
	if err != nil {
		cfg = config.Default()
	}

	enabledSet := make(map[string]bool)
	for _, name := range cfg.Sys.Enable {
		enabledSet[name] = true
	}

	// 登録されている全マネージャを表示
	allUpdaters := updater.All()
	if len(allUpdaters) == 0 {
		fmt.Println("  (登録されているマネージャがありません)")
		return nil
	}

	fmt.Println("名前       | 表示名                    | 利用可能 | 有効")
	fmt.Println("-----------|---------------------------|----------|------")

	for _, u := range allUpdaters {
		available := "❌"
		if u.IsAvailable() {
			available = "✅"
		}

		enabled := enabledMark(enabledSet[u.Name()])

		fmt.Printf("%-10s | %-25s | %s       | %s\n",
			u.Name(), u.DisplayName(), available, enabled)
	}

	fmt.Println()
	fmt.Println("💡 マネージャを有効化するには config.yaml の sys.enable を編集してください。")

	return nil
}

func enabledMark(enabled bool) string {
	if enabled {
		return "✅"
	}

	return "❌"
}
