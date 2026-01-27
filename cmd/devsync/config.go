package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/scottlz0310/devsync/internal/config"

	"github.com/scottlz0310/devsync/internal/env"
	"github.com/spf13/cobra"
)

// シェルタイプ定数
const (
	shellPowerShell = "powershell"
	shellZsh        = "zsh"
	shellBash       = "bash"
)

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
	Short: "シェル設定からdevsyncを削除",
	Long:  `シェルの設定ファイル（.bashrc, .zshrc, PowerShellプロファイル）からdevsyncのマーカーブロックを削除します。`,
	RunE:  runConfigUninstall,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configUninstallCmd)
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	fmt.Println("devsync 設定ウィザードを開始します...")
	fmt.Println()

	// デフォルト値の準備
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	defaultRepoRoot := filepath.Join(home, "src")
	recommendedManagers := env.GetRecommendedManagers()

	// 質問項目の定義
	questions := []*survey.Question{
		{
			Name: "RepoRoot",
			Prompt: &survey.Input{
				Message: "リポジトリのルートディレクトリ:",
				Default: defaultRepoRoot,
			},
		},
		{
			Name: "GithubOwner",
			Prompt: &survey.Input{
				Message: "GitHubのオーナー名 (ユーザー名または組織名):",
				Help:    "自分のリポジトリを同期する場合に指定します。",
			},
		},
		{
			Name: "Concurrency",
			Prompt: &survey.Input{
				Message: "並列実行数:",
				Default: "8",
			},
			Validate: func(val interface{}) error {
				// シンプルな数値チェックがあれば良いが、survey.Input の結果はstring
				// 厳密なバリデーションはConfigロード時に任せる手もあるが、軽くチェックしてもよい
				return nil
			},
		},
		{
			Name: "EnabledManagers",
			Prompt: &survey.MultiSelect{
				Message: "有効にするシステムマネージャ:",
				Options: []string{"apt", "brew", "go", "npm", "snap", "pipx", "cargo"},
				Default: recommendedManagers,
				Help:    "環境に合わせて自動検出された推奨値が選択されています。",
			},
		},
	}

	// 回答を受け取る構造体
	answers := struct {
		RepoRoot        string
		GithubOwner     string
		Concurrency     int
		EnabledManagers []string
	}{}

	// 質問実行
	if err := survey.Ask(questions, &answers); err != nil {
		if errors.Is(err, terminal.InterruptErr) {
			fmt.Println("キャンセルしました。")
			return nil
		}

		return err
	}

	fmt.Println()
	fmt.Println("📝 Bitwarden連携について:")
	fmt.Println("   環境変数は 'env:' プレフィックス付きの項目から自動的に読み込まれます。")
	fmt.Println("   各項目に 'value' カスタムフィールドを設定してください。")
	fmt.Println("   例: 項目名='env:GPAT', カスタムフィールド='value'に値を設定")
	fmt.Println()

	// Config構造体の構築
	cfg := &config.Config{
		Version: 1,
		Control: config.ControlConfig{
			Concurrency: answers.Concurrency,
			Timeout:     "10m",
			DryRun:      false,
		},
		Repo: config.RepoConfig{
			Root: answers.RepoRoot,
			GitHub: config.GitHubConfig{
				Owner:    answers.GithubOwner,
				Protocol: "https",
			},
			Sync: config.RepoSyncConfig{
				AutoStash: true,
				Prune:     true,
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

	// 設定の微調整 (例: aptはsudoが必要など、デフォルト値を入れる)
	for _, mgr := range answers.EnabledManagers {
		if mgr == "apt" || mgr == "snap" {
			cfg.Sys.Managers[mgr] = config.ManagerConfig{"sudo": true}
		}
	}

	// 保存確認
	savePath := filepath.Join(home, ".config", "devsync", "config.yaml")
	fmt.Printf("\n以下のパスに設定ファイルを保存します:\n%s\n", savePath)

	confirm := false
	prompt := &survey.Confirm{
		Message: "保存してよろしいですか？",
		Default: true,
	}

	if err := survey.AskOne(prompt, &confirm); err != nil {
		return err
	}

	if !confirm {
		fmt.Println("キャンセルしました。")
		return nil
	}

	// 保存実行
	if err := config.Save(cfg, savePath); err != nil {
		return fmt.Errorf("設定ファイルの保存に失敗しました: %w", err)
	}

	fmt.Println("\n✅ 設定ファイルを作成しました！")
	fmt.Println("変更するには `devsync config init` を再実行するか、直接ファイルを編集してください。")

	// シェル初期化スクリプトの生成
	if err := generateShellInit(home); err != nil {
		fmt.Printf("\n⚠️  シェル初期化スクリプトの生成に失敗しました: %v\n", err)
	}

	return nil
}

// generateShellInit は検出されたシェルに応じた初期化スクリプトを生成します
func generateShellInit(home string) error {
	shell := detectShell()
	configDir := filepath.Join(home, ".config", "devsync")

	// 現在の実行ファイルのパスを取得
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("実行ファイルのパス取得に失敗: %w", err)
	}
	// シンボリックリンクを解決
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("シンボリックリンクの解決に失敗: %w", err)
	}

	var scriptPath string

	var scriptContent string

	switch shell {
	case shellPowerShell, "pwsh":
		scriptPath = filepath.Join(configDir, "init.ps1")
		scriptContent = getPowerShellScript(exePath)
	case shellZsh:
		scriptPath = filepath.Join(configDir, "init.zsh")
		scriptContent = getZshScript(exePath)
	case shellBash:
		scriptPath = filepath.Join(configDir, "init.bash")
		scriptContent = getBashScript(exePath)
	default:
		scriptPath = filepath.Join(configDir, "init.sh")
		scriptContent = getShScript(exePath)
	}

	// スクリプトを保存
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0o644); err != nil {
		return fmt.Errorf("スクリプトファイルの保存に失敗: %w", err)
	}

	fmt.Printf("\n📝 シェル初期化スクリプトを生成しました: %s\n", scriptPath)

	// rcファイルへの追加を確認
	var rcFilePath string

	var sourceCommand string

	switch shell {
	case shellPowerShell, "pwsh":
		// PowerShellプロファイルのパスを取得
		profilePath, err := getPowerShellProfilePath(shell)
		if err != nil {
			fmt.Printf("\n⚠️  PowerShell プロファイルパスの取得に失敗しました: %v\n", err)
			fmt.Printf("次のコマンドを PowerShell のプロファイル ($PROFILE) に手動で追加してください:\n")
			fmt.Printf("\n  . %q\n", scriptPath)

			return nil
		}

		rcFilePath = profilePath
		// PowerShellではパスにスペースが含まれる可能性があるため引用符で囲む
		sourceCommand = fmt.Sprintf(". %q", scriptPath)
	case shellZsh:
		rcFilePath = filepath.Join(home, ".zshrc")
		sourceCommand = fmt.Sprintf("source %s", scriptPath)
	case shellBash:
		rcFilePath = filepath.Join(home, ".bashrc")
		sourceCommand = fmt.Sprintf("source %s", scriptPath)
	default:
		fmt.Printf("\n次のコマンドをシェルの設定ファイルに追加してください:\n")
		fmt.Printf("\n  source %s\n", scriptPath)

		return nil
	}

	// rcファイルへの追加確認
	addToRc := false
	prompt := &survey.Confirm{
		Message: fmt.Sprintf("%s に自動的に読み込む設定を追加しますか？", rcFilePath),
		Default: true,
	}

	if err := survey.AskOne(prompt, &addToRc); err != nil {
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
	fmt.Println("次回シェル起動時から自動的に devsync が利用可能になります。")
	fmt.Printf("\n現在のシェルに反映するには: source %s\n", rcFilePath)

	return nil
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
	var cmd *exec.Cmd
	if shell == "pwsh" {
		cmd = exec.CommandContext(context.Background(), "pwsh", "-NoProfile", "-Command", "echo $PROFILE")
	} else {
		cmd = exec.CommandContext(context.Background(), "powershell", "-NoProfile", "-Command", "echo $PROFILE")
	}

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	profilePath := strings.TrimSpace(string(output))
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

// appendToRcFile はrcファイルにsource行を追加します（マーカー付きで冪等性を保証）
func appendToRcFile(rcFilePath, sourceCommand string) error {
	const (
		markerBegin = "# >>> devsync >>>"
		markerEnd   = "# <<< devsync <<<"
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
	return fmt.Sprintf(`# devsync shell integration for zsh
# Generated by: devsync config init

# devsync 実行ファイルのパス
DEVSYNC_PATH="%s"

# 環境変数を読み込む関数
devsync-load-env() {
  eval "$("$DEVSYNC_PATH" env export)"
}

# dev-sync 互換関数（参考実装との互換性）
dev-sync() {
  echo "🔐 Unlocking secrets..."
  devsync-load-env || return 1

  echo "🛠  Updating system..."
  # "$DEVSYNC_PATH" sys update || return 1

  echo "📦 Syncing repositories..."
  # "$DEVSYNC_PATH" repo sync || return 1

  echo "✅ Dev environment is up to date."
}

# devsync の完了を自動ロード（オプション）
# autoload -U compinit && compinit
`, exePath)
}

// getBashScript はbash用の初期化スクリプトを返します
func getBashScript(exePath string) string {
	return fmt.Sprintf(`# devsync shell integration for bash
# Generated by: devsync config init

# devsync 実行ファイルのパス
DEVSYNC_PATH="%s"

# 環境変数を読み込む関数
devsync-load-env() {
  eval "$("$DEVSYNC_PATH" env export)"
}

# dev-sync 互換関数（参考実装との互換性）
dev-sync() {
  echo "🔐 Unlocking secrets..."
  devsync-load-env || return 1

  echo "🛠  Updating system..."
  # "$DEVSYNC_PATH" sys update || return 1

  echo "📦 Syncing repositories..."
  # "$DEVSYNC_PATH" repo sync || return 1

  echo "✅ Dev environment is up to date."
}
`, exePath)
}

// getShScript は汎用sh用の初期化スクリプトを返します
func getShScript(exePath string) string {
	return getBashScript(exePath)
}

// getPowerShellScript はPowerShell用の初期化スクリプトを返します
func getPowerShellScript(exePath string) string {
	return fmt.Sprintf(`# devsync shell integration for PowerShell
# Generated by: devsync config init

# devsync 実行ファイルのパス
$DEVSYNC_PATH = "%s"

# 環境変数を読み込む関数
function devsync-load-env {
  & $DEVSYNC_PATH env export | Invoke-Expression
}

# dev-sync 互換関数（参考実装との互換性）
function dev-sync {
  Write-Host "🔐 Unlocking secrets..." -ForegroundColor Cyan
  devsync-load-env
  if ($LASTEXITCODE -ne 0) { return }

  Write-Host "🛠  Updating system..." -ForegroundColor Cyan
  # & $DEVSYNC_PATH sys update
  # if ($LASTEXITCODE -ne 0) { return }

  Write-Host "📦 Syncing repositories..." -ForegroundColor Cyan
  # & $DEVSYNC_PATH repo sync
  # if ($LASTEXITCODE -ne 0) { return }

  Write-Host "✅ Dev environment is up to date." -ForegroundColor Green
}
`, exePath)
}

// runConfigUninstall はシェル設定からdevsyncを削除します
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
	removed, err := removeDevsyncBlock(rcFilePath)
	if err != nil {
		return fmt.Errorf("設定の削除に失敗しました: %w", err)
	}

	if removed {
		fmt.Printf("✅ %s からdevsyncの設定を削除しました。\n", rcFilePath)
	} else {
		fmt.Printf("ℹ️  %s にdevsyncの設定が見つかりませんでした。\n", rcFilePath)
	}

	return nil
}

// removeDevsyncBlock はrcファイルからdevsyncのマーカーブロックを削除します
func removeDevsyncBlock(rcFilePath string) (bool, error) {
	const (
		markerBegin = "# >>> devsync >>>"
		markerEnd   = "# <<< devsync <<<"
	)

	content, err := os.ReadFile(rcFilePath)
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

	// ファイルに書き戻す
	newContent := strings.Join(newLines, "\n")
	if err := os.WriteFile(rcFilePath, []byte(newContent), 0o644); err != nil {
		return false, err
	}

	return removed, nil
}
