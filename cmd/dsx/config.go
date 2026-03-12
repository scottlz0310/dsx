package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/scottlz0310/dsx/internal/config"

	"github.com/scottlz0310/dsx/internal/env"
	"github.com/scottlz0310/dsx/internal/updater"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// シェルタイプ定数
const (
	shellPowerShell = "powershell"
	shellZsh        = "zsh"
	shellBash       = "bash"
)

var errConfigInitCanceled = errors.New("config init canceled")

var availableSystemManagers = []string{
	"apt", "brew", "go", "npm", "pnpm", "nvm", "snap", "flatpak", "fwupdmgr", "pipx", "cargo", "uv", "rustup", "gem", "winget", "scoop",
}

// テストで対話入力や外部依存を差し替えるためのフック
var (
	surveyAskOneStep             = survey.AskOne
	getPowerShellProfilePathStep = getPowerShellProfilePath
)

type configInitDefaults struct {
	RepoRoot        string
	GitHubOwner     string
	Concurrency     int
	EnableTUI       bool
	EnabledManagers []string
}

type configInitAnswers struct {
	RepoRoot        string
	GithubOwner     string
	Concurrency     int
	EnableTUI       bool
	EnabledManagers []string
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "設定ファイルの管理",
	Long:  `設定ファイルの作成、編集、表示を行います。`,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "設定ファイルの初期化（対話モード）",
	Long:  `ウィザード形式で設定ファイルを作成します。`,
	RunE:  runConfigInit,
}

var configUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "シェル設定からdsxを削除",
	Long:  `シェルの設定ファイル（.bashrc, .zshrc, PowerShellプロファイル）からdsxのマーカーブロックを削除します。`,
	RunE:  runConfigUninstall,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "現在の設定を表示",
	Long:  `現在の設定（設定ファイル + 環境変数を反映した値）を YAML 形式で表示します。`,
	RunE:  runConfigShow,
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "設定内容を検証",
	Long:  `設定内容の妥当性（パスの存在、値の範囲、未対応値など）を検証します。`,
	RunE:  runConfigValidate,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configUninstallCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configValidateCmd)
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	fmt.Println("dsx 設定ウィザードを開始します...")
	fmt.Println()

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	recommendedManagers := env.GetRecommendedManagers()
	defaultGitHubOwner := resolveGitHubOwnerDefault(cmd.Context(), queryGitHubOwner)
	existingCfg, existingConfigPath, hasExistingConfig := loadExistingConfigForInit()
	defaults := buildConfigInitDefaults(home, recommendedManagers, availableSystemManagers, existingCfg, defaultGitHubOwner)

	printConfigInitDefaultsInfo(defaultGitHubOwner, existingCfg, existingConfigPath, hasExistingConfig)

	answers, err := askConfigInitAnswers(defaults)
	if err != nil {
		return handleConfigInitErr(err)
	}

	repoRoot, err := prepareRepoRoot(answers.RepoRoot, askCreateRepoRoot)
	if err != nil {
		return handleConfigInitErr(err)
	}

	answers.RepoRoot = repoRoot

	printBitwardenGuide()

	cfg := buildConfigFromInitAnswers(answers)

	savePath := filepath.Join(home, ".config", "dsx", "config.yaml")
	if err := confirmAndSaveConfig(cfg, savePath); err != nil {
		return handleConfigInitErr(err)
	}

	fmt.Println("\n✅ 設定ファイルを作成しました！")
	fmt.Println("変更するには `dsx config init` を再実行するか、直接ファイルを編集してください。")

	// シェル初期化スクリプトの生成
	if err := generateShellInit(home); err != nil {
		fmt.Printf("\n⚠️  シェル初期化スクリプトの生成に失敗しました: %v\n", err)
	}

	return nil
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("設定ファイルの読み込みに失敗しました: %w", err)
	}

	exists, path, stateErr := config.ConfigFileExists()
	if stateErr != nil {
		fmt.Fprintf(os.Stderr, "⚠️  設定ファイル状態の確認に失敗しました: %v\n", stateErr)
	}

	fmt.Printf("📋 設定ファイル: %s\n", path)

	switch {
	case stateErr != nil:
		fmt.Println("⚠️  設定ファイル状態は要確認です（設定値は表示します）")
	case exists:
		fmt.Println("✅ 設定ファイルを読み込みました")
	default:
		fmt.Println("⚪ 設定ファイルは未作成です（デフォルト値で表示します）")
	}

	yamlBytes, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("設定の整形に失敗しました: %w", err)
	}

	fmt.Println("\n---")
	fmt.Print(string(yamlBytes))

	return nil
}

func runConfigValidate(cmd *cobra.Command, args []string) error {
	fmt.Println("🔍 設定の検証を開始します...")
	fmt.Println()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("設定ファイルの読み込みに失敗しました: %w", err)
	}

	exists, path, stateErr := config.ConfigFileExists()
	if stateErr != nil {
		fmt.Fprintf(os.Stderr, "⚠️  設定ファイル状態の確認に失敗しました: %v\n", stateErr)
	}

	fmt.Printf("📋 設定ファイル: %s\n", path)

	switch {
	case stateErr != nil:
		fmt.Println("⚠️  設定ファイル状態は要確認です（設定値は検証します）")
	case exists:
		fmt.Println("✅ 設定ファイルを読み込みました")
	default:
		fmt.Println("⚪ 設定ファイルは未作成です（デフォルト値で検証します）")
	}

	knownManagers := make(map[string]struct{})
	for _, u := range updater.All() {
		knownManagers[u.Name()] = struct{}{}
	}

	result := config.Validate(cfg, config.ValidateOptions{
		KnownSysManagers: knownManagers,
	})

	if len(result.Warnings) > 0 {
		fmt.Println("\n⚠️  警告:")

		for _, w := range result.Warnings {
			fmt.Printf("  - %s\n", w.String())
		}
	}

	if len(result.Errors) > 0 {
		fmt.Println("\n❌ エラー:")

		for _, e := range result.Errors {
			fmt.Printf("  - %s\n", e.String())
		}

		fmt.Println("\n修正後に `dsx config init` を再実行するか、設定ファイルを編集してから再実行してください。")

		return fmt.Errorf("設定にエラーがあります（%d件）", len(result.Errors))
	}

	fmt.Println("\n✅ 設定の検証に成功しました。")

	if !exists {
		fmt.Println("   まだ設定ファイルを作成していない場合は `dsx config init` の実行を推奨します。")
	}

	return nil
}

func printConfigInitDefaultsInfo(defaultGitHubOwner string, existingCfg *config.Config, existingConfigPath string, hasExistingConfig bool) {
	if hasExistingConfig {
		fmt.Printf("🧩 既存設定を初期値として読み込みました: %s\n\n", existingConfigPath)
	}

	if defaultGitHubOwner != "" && (existingCfg == nil || strings.TrimSpace(existingCfg.Repo.GitHub.Owner) == "") {
		fmt.Printf("🔎 gh auth から GitHub オーナー名を自動入力しました: %s\n\n", defaultGitHubOwner)
	}
}

func askConfigInitAnswers(defaults configInitDefaults) (configInitAnswers, error) {
	questions := []*survey.Question{
		{
			Name: "RepoRoot",
			Prompt: &survey.Input{
				Message: "リポジトリのルートディレクトリ:",
				Default: defaults.RepoRoot,
			},
		},
		{
			Name: "GithubOwner",
			Prompt: &survey.Input{
				Message: "GitHubのオーナー名 (ユーザー名または組織名):",
				Default: defaults.GitHubOwner,
				Help:    "gh auth login 済みなら自動入力されます。必要に応じて組織名へ変更してください。",
			},
		},
		{
			Name: "Concurrency",
			Prompt: &survey.Input{
				Message: "並列実行数:",
				Default: strconv.Itoa(defaults.Concurrency),
			},
		},
		{
			Name: "EnableTUI",
			Prompt: &survey.Confirm{
				Message: "進捗表示をTUI (Bubble Tea) で表示しますか？",
				Default: defaults.EnableTUI,
			},
		},
		{
			Name: "EnabledManagers",
			Prompt: &survey.MultiSelect{
				Message: "有効にするシステムマネージャ:",
				Options: availableSystemManagers,
				Default: defaults.EnabledManagers,
				Help:    "環境に合わせて自動検出された推奨値が選択されています。",
			},
		},
	}

	answers := configInitAnswers{}
	if err := survey.Ask(questions, &answers); err != nil {
		if errors.Is(err, terminal.InterruptErr) {
			return configInitAnswers{}, errConfigInitCanceled
		}

		return configInitAnswers{}, err
	}

	return answers, nil
}

func printBitwardenGuide() {
	fmt.Println()
	fmt.Println("📝 Bitwarden連携について:")
	fmt.Println("   環境変数は 'env:' プレフィックス付きの項目から自動的に読み込まれます。")
	fmt.Println("   各項目に 'value' カスタムフィールドを設定してください。")
	fmt.Println("   例: 項目名='env:GPAT', カスタムフィールド='value'に値を設定")
	fmt.Println()
}

func buildConfigFromInitAnswers(answers configInitAnswers) *config.Config {
	cfg := &config.Config{
		Version: 1,
		Control: config.ControlConfig{
			Concurrency: answers.Concurrency,
			Timeout:     "10m",
			DryRun:      false,
		},
		UI: config.UIConfig{
			TUI: answers.EnableTUI,
		},
		Repo: config.RepoConfig{
			Root: answers.RepoRoot,
			GitHub: config.GitHubConfig{
				Owner:    answers.GithubOwner,
				Protocol: "https",
			},
			Sync: config.RepoSyncConfig{
				AutoStash:       true,
				Prune:           true,
				SubmoduleUpdate: true,
			},
			Cleanup: config.RepoCleanupConfig{
				Enabled:         true,
				Target:          []string{"merged", "squashed"},
				ExcludeBranches: []string{"main", "master", "develop"},
			},
		},
		Sys: config.SysConfig{
			Enable:   answers.EnabledManagers,
			Managers: make(map[string]config.ManagerConfig),
		},
		Secrets: config.SecretsConfig{
			Enabled:  true, // 常に有効（env:プレフィックスで自動検索）
			Provider: "bitwarden",
		},
	}

	for _, mgr := range answers.EnabledManagers {
		if mgr == "apt" || mgr == "snap" {
			cfg.Sys.Managers[mgr] = config.ManagerConfig{"use_sudo": true}
		}
	}

	return cfg
}

func confirmAndSaveConfig(cfg *config.Config, savePath string) error {
	fmt.Printf("\n以下のパスに設定ファイルを保存します:\n%s\n", savePath)

	confirm := false
	prompt := &survey.Confirm{
		Message: "保存してよろしいですか？",
		Default: true,
	}

	if err := survey.AskOne(prompt, &confirm); err != nil {
		if errors.Is(err, terminal.InterruptErr) {
			return errConfigInitCanceled
		}

		return err
	}

	if !confirm {
		return errConfigInitCanceled
	}

	if err := config.Save(cfg, savePath); err != nil {
		return fmt.Errorf("設定ファイルの保存に失敗しました: %w", err)
	}

	return nil
}

func handleConfigInitErr(err error) error {
	if errors.Is(err, errConfigInitCanceled) {
		fmt.Println("キャンセルしました。")
		return nil
	}

	return err
}

func askCreateRepoRoot(path string) (bool, error) {
	createDir := false
	prompt := &survey.Confirm{
		Message: fmt.Sprintf("指定したリポジトリルートが存在しません。作成しますか？\n%s", path),
		Default: true,
	}

	if err := survey.AskOne(prompt, &createDir); err != nil {
		if errors.Is(err, terminal.InterruptErr) {
			return false, errConfigInitCanceled
		}

		return false, err
	}

	return createDir, nil
}

func resolveGitHubOwnerDefault(baseCtx context.Context, lookup func(context.Context) (string, error)) string {
	if lookup == nil {
		return ""
	}

	owner, err := lookup(baseCtx)
	if err != nil {
		return ""
	}

	trimmed := strings.TrimSpace(owner)
	if trimmed == "" {
		return ""
	}

	return trimmed
}

func loadExistingConfigForInit() (cfg *config.Config, configPath string, ok bool) {
	exists, path, stateErr := config.ConfigFileExists()
	if stateErr != nil {
		fmt.Fprintf(os.Stderr, "⚠️  設定ファイル状態の確認に失敗: %v\n", stateErr)
		return nil, path, false
	}

	if !exists {
		return nil, path, false
	}

	loadedCfg, loadErr := config.Load()
	if loadErr != nil {
		if strings.TrimSpace(path) != "" {
			fmt.Fprintf(os.Stderr, "⚠️  既存設定の読み込みに失敗 (%s): %v\n", path, loadErr)
		} else {
			fmt.Fprintf(os.Stderr, "⚠️  既存設定の読み込みに失敗: %v\n", loadErr)
		}

		return nil, path, false
	}

	return loadedCfg, path, true
}

func buildConfigInitDefaults(
	home string,
	recommendedManagers []string,
	promptOptions []string,
	existingCfg *config.Config,
	autoGitHubOwner string,
) configInitDefaults {
	defaults := configInitDefaults{
		RepoRoot:        filepath.Join(home, "src"),
		GitHubOwner:     strings.TrimSpace(autoGitHubOwner),
		Concurrency:     8,
		EnableTUI:       false,
		EnabledManagers: append([]string(nil), recommendedManagers...),
	}

	if existingCfg != nil {
		if existingRoot := strings.TrimSpace(existingCfg.Repo.Root); existingRoot != "" {
			defaults.RepoRoot = existingRoot
		}

		if existingOwner := strings.TrimSpace(existingCfg.Repo.GitHub.Owner); existingOwner != "" {
			defaults.GitHubOwner = existingOwner
		}

		if existingCfg.Control.Concurrency > 0 {
			defaults.Concurrency = existingCfg.Control.Concurrency
		}

		defaults.EnableTUI = existingCfg.UI.TUI

		if len(existingCfg.Sys.Enable) > 0 {
			defaults.EnabledManagers = append([]string(nil), existingCfg.Sys.Enable...)
		}
	}

	defaults.EnabledManagers = resolvePromptDefaultManagers(defaults.EnabledManagers, promptOptions, recommendedManagers)

	return defaults
}

func resolvePromptDefaultManagers(candidates, promptOptions, recommendedManagers []string) []string {
	allowed := make(map[string]struct{}, len(promptOptions))
	for _, manager := range promptOptions {
		allowed[manager] = struct{}{}
	}

	filtered := filterManagersByAllowedSet(candidates, allowed)
	if len(filtered) > 0 {
		return filtered
	}

	filtered = filterManagersByAllowedSet(recommendedManagers, allowed)
	if len(filtered) > 0 {
		return filtered
	}

	return append([]string(nil), promptOptions...)
}

func filterManagersByAllowedSet(candidates []string, allowed map[string]struct{}) []string {
	filtered := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))

	for _, manager := range candidates {
		if _, ok := allowed[manager]; !ok {
			continue
		}

		if _, exists := seen[manager]; exists {
			continue
		}

		filtered = append(filtered, manager)
		seen[manager] = struct{}{}
	}

	return filtered
}

func queryGitHubOwner(baseCtx context.Context) (string, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "", err
	}

	if baseCtx == nil {
		baseCtx = context.Background()
	}

	output, err := exec.CommandContext(baseCtx, "gh", "api", "user", "--jq", ".login").Output()
	if err != nil {
		return "", err
	}

	owner := strings.TrimSpace(string(output))
	if owner == "" {
		return "", fmt.Errorf("GitHubオーナー名を取得できませんでした")
	}

	return owner, nil
}

func prepareRepoRoot(input string, confirmCreate func(path string) (bool, error)) (string, error) {
	root, err := normalizeRepoRoot(input)
	if err != nil {
		return "", err
	}

	if err := ensureRepoRoot(root, confirmCreate); err != nil {
		return "", err
	}

	return root, nil
}

func normalizeRepoRoot(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", fmt.Errorf("リポジトリルートが空です")
	}

	expanded, err := expandHomePath(trimmed)
	if err != nil {
		return "", err
	}

	return filepath.Clean(expanded), nil
}

func expandHomePath(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("ホームディレクトリの取得に失敗: %w", err)
	}

	if path == "~" {
		return home, nil
	}

	return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
}

func ensureRepoRoot(path string, confirmCreate func(path string) (bool, error)) error {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("指定されたリポジトリルートはディレクトリではありません: %s", path)
		}

		return nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("リポジトリルートの確認に失敗: %w", err)
	}

	createDir, err := confirmCreate(path)
	if err != nil {
		return err
	}

	if !createDir {
		return errConfigInitCanceled
	}

	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("リポジトリルートの作成に失敗: %w", err)
	}

	fmt.Printf("\n✅ リポジトリルートを作成しました: %s\n", path)

	return nil
}

// generateShellInit は検出されたシェルに応じた初期化スクリプトを生成します
func generateShellInit(home string) error {
	shell := detectShell()
	configDir := filepath.Join(home, ".config", "dsx")

	exePath, err := resolveExecutablePath()
	if err != nil {
		return err
	}

	scriptPath, scriptContent := resolveInitScript(shell, configDir, exePath)

	// スクリプトを保存
	writeErr := os.WriteFile(scriptPath, []byte(scriptContent), 0o644)
	if writeErr != nil {
		return fmt.Errorf("スクリプトファイルの保存に失敗: %w", writeErr)
	}

	fmt.Printf("\n📝 シェル初期化スクリプトを生成しました: %s\n", scriptPath)

	rcFilePath, sourceCommand, supported, err := resolveShellRcFile(shell, home, scriptPath)
	if !supported {
		// sh の場合は dot コマンド (.) を使う
		fmt.Printf("\n次のコマンドをシェルの設定ファイルに追加してください:\n")
		fmt.Printf("\n  %s %s\n", getSourceKeyword(shell), quoteForPosixShell(scriptPath))

		return nil
	}

	if err != nil {
		fmt.Printf("\n⚠️  PowerShell プロファイルパスの取得に失敗しました: %v\n", err)
		fmt.Printf("次のコマンドを PowerShell のプロファイル ($PROFILE) に手動で追加してください:\n")
		fmt.Printf("\n  . %s\n", quoteForPowerShell(scriptPath))

		return nil
	}

	addToRc, err := confirmAddToRc(rcFilePath)
	if err != nil {
		return err
	}

	if !addToRc {
		fmt.Printf("\n次のコマンドを %s に手動で追加してください:\n", rcFilePath)
		fmt.Printf("\n  %s\n", sourceCommand)

		return nil
	}

	// rcファイルに追加
	if err := appendToRcFile(rcFilePath, sourceCommand); err != nil {
		return fmt.Errorf("rcファイルへの追加に失敗: %w", err)
	}

	fmt.Printf("\n✅ %s に設定を追加しました！\n", rcFilePath)
	fmt.Println("次回シェル起動時から自動的に dsx が利用可能になります。")

	fmt.Printf("\n現在のシェルに反映するには: %s\n", buildReloadCommand(shell, rcFilePath))

	return nil
}

func resolveExecutablePath() (string, error) {
	// 現在の実行ファイルのパスを取得
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("実行ファイルのパス取得に失敗: %w", err)
	}

	// シンボリックリンクを解決
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return "", fmt.Errorf("シンボリックリンクの解決に失敗: %w", err)
	}

	return exePath, nil
}

func resolveInitScript(shell, configDir, exePath string) (scriptPath, scriptContent string) {
	switch shell {
	case shellPowerShell, "pwsh":
		return filepath.Join(configDir, "init.ps1"), getPowerShellScript(exePath)
	case shellZsh:
		return filepath.Join(configDir, "init.zsh"), getZshScript(exePath)
	case shellBash:
		return filepath.Join(configDir, "init.bash"), getBashScript(exePath)
	default:
		return filepath.Join(configDir, "init.sh"), getShScript(exePath)
	}
}

func resolveShellRcFile(shell, home, scriptPath string) (rcFilePath, sourceCommand string, supported bool, err error) {
	switch shell {
	case shellPowerShell, "pwsh":
		profilePath, err := getPowerShellProfilePathStep(shell)
		if err != nil {
			return "", "", true, err
		}

		// PowerShell ではパスにスペースが含まれる可能性があるため、文字列として正しく解釈される形で引用符を付けます。
		return profilePath, fmt.Sprintf(". %s", quoteForPowerShell(scriptPath)), true, nil
	case shellZsh:
		rcFilePath := filepath.Join(home, ".zshrc")
		return rcFilePath, fmt.Sprintf("source %s", quoteForPosixShell(scriptPath)), true, nil
	case shellBash:
		rcFilePath := filepath.Join(home, ".bashrc")
		return rcFilePath, fmt.Sprintf("source %s", quoteForPosixShell(scriptPath)), true, nil
	default:
		return "", "", false, nil
	}
}

func confirmAddToRc(rcFilePath string) (bool, error) {
	addToRc := false
	prompt := &survey.Confirm{
		Message: fmt.Sprintf("%s に自動的に読み込む設定を追加しますか？", rcFilePath),
		Default: true,
	}

	if err := surveyAskOneStep(prompt, &addToRc); err != nil {
		return false, err
	}

	return addToRc, nil
}

// getSourceKeyword はシェルに応じたファイル読み込みコマンドを返します。
// sh の場合は POSIX 標準の dot コマンド (.) を、その他は source を返します。
func getSourceKeyword(shell string) string {
	if shell == "sh" {
		return "."
	}
	return "source"
}

func buildReloadCommand(shell, rcFilePath string) string {
	if shell == shellPowerShell || shell == "pwsh" {
		return ". $PROFILE"
	}

	return fmt.Sprintf("%s %s", getSourceKeyword(shell), quoteForPosixShell(rcFilePath))
}

func quoteForPosixShell(path string) string {
	if path == "" {
		return "''"
	}

	// POSIX シェルの単一引用符: ' を含む場合は  '\'' でエスケープします。
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

func quoteForPowerShell(path string) string {
	if path == "" {
		return "''"
	}

	// PowerShell の単一引用符: ' は '' に置換します。
	return "'" + strings.ReplaceAll(path, "'", "''") + "'"
}

// detectShell は現在のシェルを検出します
func detectShell() string {
	// Windowsの場合、まず pwsh (PowerShell 7+) が存在するか確認
	if os.Getenv("PSModulePath") != "" {
		// pwsh (PowerShell Core / PowerShell 7+) の存在確認
		if _, err := exec.LookPath("pwsh"); err == nil {
			return "pwsh"
		}
		// 従来の powershell (Windows PowerShell 5.x)
		return "powershell"
	}

	// SHELL 環境変数から検出 (Linux/macOS)
	shell := os.Getenv("SHELL")
	if shell != "" {
		if filepath.Base(shell) == "zsh" {
			return "zsh"
		}

		if filepath.Base(shell) == "bash" {
			return "bash"
		}
	}

	// デフォルト
	return "sh"
}

// getPowerShellProfilePath は PowerShell のプロファイルパスを取得します
func getPowerShellProfilePath(shell string) (string, error) {
	return getPowerShellProfilePathWithOutput(shell, func(command string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(context.Background(), command, args...)
		return cmd.Output()
	})
}

type commandOutputFunc func(command string, args ...string) ([]byte, error)

func getPowerShellProfilePathWithOutput(shell string, commandOutput commandOutputFunc) (string, error) {
	command := "powershell"
	if shell == "pwsh" {
		command = "pwsh"
	}

	// PowerShell の標準出力は環境によって文字コードが変わるため、パス文字列を Base64(UTF-8) で受け取ります。
	// これにより、日本語を含むパスでも文字化けせず安全に復元できます。
	psCommand := "[System.Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($PROFILE))"

	output, err := commandOutput(command, "-NoProfile", "-NonInteractive", "-Command", psCommand)
	if err != nil {
		return "", err
	}

	encoded := strings.TrimSpace(decodePowerShellTextOutput(output))
	if encoded == "" {
		return "", fmt.Errorf("プロファイルパスの取得に失敗しました（出力が空です）")
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("プロファイルパスのデコードに失敗しました: %w", err)
	}

	profilePath := strings.TrimSpace(string(decoded))
	if profilePath == "" {
		return "", fmt.Errorf("プロファイルパスが空です")
	}

	// プロファイルの親ディレクトリが存在しない場合は作成
	profileDir := filepath.Dir(profilePath)
	if _, err := os.Stat(profileDir); os.IsNotExist(err) {
		if err := os.MkdirAll(profileDir, 0o755); err != nil {
			return "", fmt.Errorf("プロファイルディレクトリの作成に失敗: %w", err)
		}
	}

	return profilePath, nil
}

func decodePowerShellTextOutput(output []byte) string {
	if len(output) == 0 {
		return ""
	}

	// BOM がある場合はそれを優先して扱う
	if len(output) >= 2 && output[0] == 0xFF && output[1] == 0xFE {
		return decodeUTF16(output[2:], binary.LittleEndian)
	}

	if len(output) >= 2 && output[0] == 0xFE && output[1] == 0xFF {
		return decodeUTF16(output[2:], binary.BigEndian)
	}

	// BOM が無くても UTF-16LE のケースがあるため、NUL バイトを含む場合は UTF-16LE とみなします。
	// Windows の標準出力で遭遇するパターンを優先します。
	if bytes.IndexByte(output, 0) != -1 {
		return decodeUTF16(output, binary.LittleEndian)
	}

	// それ以外は（Base64 の ASCII も含め）そのまま扱う
	return string(output)
}

func decodeUTF16(data []byte, order binary.ByteOrder) string {
	if len(data) == 0 {
		return ""
	}

	// UTF-16 は 2 バイト単位。奇数長は末尾を切り捨てます。
	if len(data)%2 == 1 {
		data = data[:len(data)-1]
	}

	u16 := make([]uint16, 0, len(data)/2)
	for i := 0; i < len(data); i += 2 {
		u16 = append(u16, order.Uint16(data[i:i+2]))
	}

	return string(utf16.Decode(u16))
}

// appendToRcFile はrcファイルにsource行を追加します（マーカー付きで冪等性を保証）
func appendToRcFile(rcFilePath, sourceCommand string) error {
	const (
		markerBegin = "# >>> dsx >>>"
		markerEnd   = "# <<< dsx <<<"
	)

	// rcファイルが存在するか確認
	content, err := os.ReadFile(rcFilePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	contentStr := string(content)

	// 既にマーカーブロックが存在するかチェック
	if strings.Contains(contentStr, markerBegin) {
		fmt.Println("\n⚠️  既に設定が追加されています。スキップします。")
		return nil
	}

	// 追加する内容（マーカー付き）
	addition := fmt.Sprintf("\n%s\n%s\n%s\n", markerBegin, sourceCommand, markerEnd)

	// ファイルに追記
	f, err := os.OpenFile(rcFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	if _, err := f.WriteString(addition); err != nil {
		return err
	}

	return nil
}

// getZshScript はzsh用の初期化スクリプトを返します
func getZshScript(exePath string) string {
	return fmt.Sprintf(`# dsx shell integration for zsh
# Generated by: dsx config init

# dsx 実行ファイルのパス
DSX_PATH="%s"
if [[ ! -x "$DSX_PATH" ]] && command -v dsx >/dev/null 2>&1; then
  DSX_PATH="$(command -v dsx)"
fi

# Bitwarden をこのシェルでアンロック（単独使用）
dsx-unlock() {
  if ! command -v bw >/dev/null 2>&1; then
    echo "bw コマンドが見つかりません" >&2
    return 1
  fi

  if [[ -n "${BW_SESSION-}" ]]; then
    local status_json
    status_json="$(bw status 2>/dev/null)"
    if [[ "$status_json" == *'"status":"unlocked"'* ]]; then
      echo "このシェルでは既に BW_SESSION が設定されています。"
      return 0
    fi
    unset BW_SESSION
  fi

  if ! bw login --check >/dev/null 2>&1; then
    echo "Bitwarden CLI にログインしていません。まず bw login を実行してください。" >&2
    return 1
  fi

  local token
  token="$(bw unlock --raw)"
  local status=$?
  if [[ $status -ne 0 || -z "$token" ]]; then
    echo "Bitwarden のアンロックに失敗しました。" >&2
    return 1
  fi

  export BW_SESSION="$token"
  echo "✅ このシェルで Bitwarden をアンロックしました。"
}

# 環境変数を注入する（必要に応じて自動でアンロック）
dsx-env() {
  if [[ -z "${BW_SESSION-}" ]]; then
    echo "🔐 Bitwarden をアンロック中..."
    dsx-unlock || return 1
  fi

  echo "🔑 環境変数をシェルへ読み込み中..."
  local env_output
  env_output="$("$DSX_PATH" env export)"
  if [[ $? -ne 0 ]]; then
    return 1
  fi
  eval "$env_output" || return 1
  export DSX_ENV_LOADED=1
}

# システム更新（環境変数注入 → sys update）
dsx-sys() {
  dsx-env || return 1
  "$DSX_PATH" sys update "$@"
}

# リポジトリ更新（環境変数注入 → repo update）
dsx-repo() {
  dsx-env || return 1
  "$DSX_PATH" repo update "$@"
}

# 全部実行（環境変数注入 → dsx run）
dsx-run() {
  dsx-env || return 1
  echo "🚀 dsx run を実行します..."
  "$DSX_PATH" run "$@"
}

# dsx の補完を自動ロード（オプション）
# autoload -U compinit && compinit
`, exePath)
}

// getBashScript はbash用の初期化スクリプトを返します
func getBashScript(exePath string) string {
	return fmt.Sprintf(`# dsx shell integration for bash
# Generated by: dsx config init

# dsx 実行ファイルのパス
DSX_PATH="%s"
if [[ ! -x "$DSX_PATH" ]] && command -v dsx >/dev/null 2>&1; then
  DSX_PATH="$(command -v dsx)"
fi

# Bitwarden をこのシェルでアンロック（単独使用）
dsx-unlock() {
  if ! command -v bw >/dev/null 2>&1; then
    echo "bw コマンドが見つかりません" >&2
    return 1
  fi

  if [ -n "${BW_SESSION-}" ]; then
    local status_json
    status_json="$(bw status 2>/dev/null)"
    case "$status_json" in
      *'"status":"unlocked"'*)
        echo "このシェルでは既に BW_SESSION が設定されています。"
        return 0
        ;;
    esac
    unset BW_SESSION
  fi

  if ! bw login --check >/dev/null 2>&1; then
    echo "Bitwarden CLI にログインしていません。まず bw login を実行してください。" >&2
    return 1
  fi

  local token
  token="$(bw unlock --raw)"
  local status=$?
  if [ $status -ne 0 ] || [ -z "$token" ]; then
    echo "Bitwarden のアンロックに失敗しました。" >&2
    return 1
  fi

  export BW_SESSION="$token"
  echo "✅ このシェルで Bitwarden をアンロックしました。"
}

# 環境変数を注入する（必要に応じて自動でアンロック）
dsx-env() {
  if [ -z "${BW_SESSION-}" ]; then
    echo "🔐 Bitwarden をアンロック中..."
    dsx-unlock || return 1
  fi

  echo "🔑 環境変数をシェルへ読み込み中..."
  local env_output
  env_output="$("$DSX_PATH" env export)"
  if [ $? -ne 0 ]; then
    return 1
  fi
  eval "$env_output" || return 1
  export DSX_ENV_LOADED=1
}

# システム更新（環境変数注入 → sys update）
dsx-sys() {
  dsx-env || return 1
  "$DSX_PATH" sys update "$@"
}

# リポジトリ更新（環境変数注入 → repo update）
dsx-repo() {
  dsx-env || return 1
  "$DSX_PATH" repo update "$@"
}

# 全部実行（環境変数注入 → dsx run）
dsx-run() {
  dsx-env || return 1
  echo "🚀 dsx run を実行します..."
  "$DSX_PATH" run "$@"
}
`, exePath)
}

// getShScript は汎用sh用の初期化スクリプトを返します
func getShScript(exePath string) string {
	return getBashScript(exePath)
}

// getPowerShellScript はPowerShell用の初期化スクリプトを返します
func getPowerShellScript(exePath string) string {
	return fmt.Sprintf(`# dsx shell integration for PowerShell
# Generated by: dsx config init

# dsx 実行ファイルのパス
$DSX_PATH = "%s"
if (-not (Test-Path $DSX_PATH)) {
  $resolved = Get-Command dsx -ErrorAction SilentlyContinue
  if ($resolved) {
    $DSX_PATH = $resolved.Source
  }
}

# Bitwarden をこのシェルでアンロック（単独使用）
function dsx-unlock {
  $bw = Get-Command bw -ErrorAction SilentlyContinue
  if (-not $bw) {
    Write-Error "bw コマンドが見つかりません"
    return $false
  }

  if ($env:BW_SESSION) {
    $statusJson = & bw status 2>$null
    if ($statusJson -match '"status":"unlocked"') {
      Write-Host "このシェルでは既に BW_SESSION が設定されています。"
      return $true
    }
    Remove-Item Env:BW_SESSION -ErrorAction SilentlyContinue
  }

  $null = & bw login --check 2>$null
  if ($LASTEXITCODE -ne 0) {
    Write-Error "Bitwarden CLI にログインしていません。まず bw login を実行してください。"
    return $false
  }

  $token = & bw unlock --raw
  if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($token)) {
    Write-Error "Bitwarden のアンロックに失敗しました。"
    return $false
  }

  $env:BW_SESSION = $token.Trim()
  Write-Host "✅ このシェルで Bitwarden をアンロックしました。"
  return $true
}

# 環境変数を注入する（必要に応じて自動でアンロック）
function dsx-env {
  if (-not $env:BW_SESSION) {
    Write-Host "🔐 Bitwarden をアンロック中..." -ForegroundColor Cyan
    if (-not (dsx-unlock)) { return $false }
  }

  Write-Host "🔑 環境変数をシェルへ読み込み中..." -ForegroundColor Cyan
  $envExports = & $DSX_PATH env export
  if ($LASTEXITCODE -ne 0) { return $false }

  $commandText = @($envExports) -join [Environment]::NewLine
  if ([string]::IsNullOrWhiteSpace($commandText)) {
    Write-Error "読み込む環境変数が見つかりませんでした。"
    return $false
  }

  try {
    Invoke-Expression -Command $commandText -ErrorAction Stop
  } catch {
    Write-Error "環境変数の読み込み中にエラーが発生しました: $_"
    return $false
  }

  $env:DSX_ENV_LOADED = "1"
  return $true
}

# システム更新（環境変数注入 → sys update）
function dsx-sys {
  if (-not (dsx-env)) { return 1 }
  & $DSX_PATH sys update @args
  return $LASTEXITCODE
}

# リポジトリ更新（環境変数注入 → repo update）
function dsx-repo {
  if (-not (dsx-env)) { return 1 }
  & $DSX_PATH repo update @args
  return $LASTEXITCODE
}

# 全部実行（環境変数注入 → dsx run）
function dsx-run {
  if (-not (dsx-env)) { return 1 }
  Write-Host "🚀 dsx run を実行します..." -ForegroundColor Cyan
  & $DSX_PATH run @args
  return $LASTEXITCODE
}
`, exePath)
}

// runConfigUninstall はシェル設定からdsxを削除します
func runConfigUninstall(cmd *cobra.Command, args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	shell := detectShell()

	var rcFilePath string

	switch shell {
	case shellPowerShell, "pwsh":
		profilePath, profileErr := getPowerShellProfilePath(shell)
		if profileErr != nil {
			fmt.Printf("⚠️  PowerShell プロファイルパスの取得に失敗しました: %v\n", profileErr)
			return nil
		}

		rcFilePath = profilePath
	case shellZsh:
		rcFilePath = filepath.Join(home, ".zshrc")
	case shellBash:
		rcFilePath = filepath.Join(home, ".bashrc")
	default:
		return fmt.Errorf("未対応のシェル: %s", shell)
	}

	// ファイルが存在するか確認
	if _, statErr := os.Stat(rcFilePath); os.IsNotExist(statErr) {
		fmt.Printf("設定ファイルが見つかりません: %s\n", rcFilePath)
		return nil
	}

	// マーカーブロックを削除
	removed, err := removeDsxBlock(home, rcFilePath)
	if err != nil {
		return fmt.Errorf("設定の削除に失敗しました: %w", err)
	}

	if removed {
		fmt.Printf("✅ %s からdsxの設定を削除しました。\n", rcFilePath)
	} else {
		fmt.Printf("ℹ️  %s にdsxの設定が見つかりませんでした。\n", rcFilePath)
	}

	return nil
}

// removeDsxBlock はrcファイルからdsxのマーカーブロックを削除します。
// homeDir はユーザーのホームディレクトリパスで、rcFilePath の実体パスがその配下にあることを
// シンボリックリンクを解決した上で検証します。
func removeDsxBlock(homeDir, rcFilePath string) (bool, error) {
	const (
		markerBegin = "# >>> dsx >>>"
		markerEnd   = "# <<< dsx <<<"
	)

	// パスを正規化して相対要素（.. など）を除去し、絶対パスであることを確認する
	rcFilePath = filepath.Clean(rcFilePath)

	if !filepath.IsAbs(rcFilePath) {
		return false, fmt.Errorf("絶対パスが必要です: %s", rcFilePath)
	}

	// シンボリックリンクを解決して実体パスを取得する（シンボリックリンク経由のホーム外アクセスを防止）
	realPath, err := filepath.EvalSymlinks(rcFilePath)
	if err != nil {
		return false, fmt.Errorf("パスの解決に失敗しました: %w", err)
	}

	// ホームディレクトリ側もシンボリックリンクを解決する
	realHome, err := filepath.EvalSymlinks(homeDir)
	if err != nil {
		return false, fmt.Errorf("ホームディレクトリの解決に失敗しました: %w", err)
	}

	// パストラバーサル防止: 実体パスがホームディレクトリ配下であることを確認する
	// filepath.Rel を使うことで Windows の大文字小文字の差異にも対応する
	rel, err := filepath.Rel(realHome, realPath)
	if err != nil {
		return false, fmt.Errorf("相対パス解決に失敗しました: %w", err)
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, fmt.Errorf("ファイルパスがホームディレクトリ外です: %s", rcFilePath)
	}

	// ファイルの既存パーミッションを取得して書き戻し時に保持する
	info, err := os.Stat(realPath)
	if err != nil {
		return false, err
	}

	originalMode := info.Mode()

	content, err := os.ReadFile(realPath)
	if err != nil {
		return false, err
	}

	contentStr := string(content)

	// マーカーブロックが存在しない場合
	if !strings.Contains(contentStr, markerBegin) {
		return false, nil
	}

	// マーカーブロックを削除
	lines := strings.Split(contentStr, "\n")

	var newLines []string

	inBlock := false
	removed := false

	for _, line := range lines {
		if strings.Contains(line, markerBegin) {
			inBlock = true
			removed = true

			continue
		}

		if strings.Contains(line, markerEnd) {
			inBlock = false
			continue
		}

		if !inBlock {
			newLines = append(newLines, line)
		}
	}

	// ファイルに書き戻す（元のパーミッションを保持）
	// realPath は filepath.EvalSymlinks で解決済みだが、filepath.Clean で明示的に正規化する
	newContent := strings.Join(newLines, "\n")
	cleanPath := filepath.Clean(realPath)

	if err := os.WriteFile(cleanPath, []byte(newContent), originalMode); err != nil {
		return false, err
	}

	return removed, nil
}
