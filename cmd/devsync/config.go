package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/scottlz0310/devsync/internal/config"

	"github.com/scottlz0310/devsync/internal/env"
	"github.com/spf13/cobra"
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

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
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
		{
			Name: "UseBitwarden",
			Prompt: &survey.Confirm{
				Message: "Bitwardenを使用しますか？",
				Default: false,
				Help:    "Github Tokenなどの環境変数をBitwardenから自動注入する場合に有効にします。",
			},
		},
	}

	// 回答を受け取る構造体
	answers := struct {
		RepoRoot        string
		GithubOwner     string
		Concurrency     int
		EnabledManagers []string
		UseBitwarden    bool
	}{}

	// 質問実行
	if err := survey.Ask(questions, &answers); err != nil {
		if err == terminal.InterruptErr {
			fmt.Println("キャンセルしました。")
			return nil
		}
		return err
	}

	// Bitwardenを使用する場合、アイテムIDの入力を求める
	var secretItems []string
	if answers.UseBitwarden {
		var itemsInput string
		secretPrompt := &survey.Input{
			Message: "注入するBitwarden Item ID (カンマ区切り):",
			Help:    "BitwardenのアイテムIDを入力してください。例えばGitHub Tokenなど。",
		}
		if err := survey.AskOne(secretPrompt, &itemsInput); err != nil {
			return err
		}
		if itemsInput != "" {
			// カンマで分割してトリム
			parts := strings.Split(itemsInput, ",")
			for _, part := range parts {
				item := strings.TrimSpace(part)
				if item != "" {
					secretItems = append(secretItems, item)
				}
			}
		}
	}

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
			Enabled:  answers.UseBitwarden,
			Provider: "bitwarden",
			Items:    secretItems,
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
	return nil
}

// Helper to check slice containment (if needed)
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
