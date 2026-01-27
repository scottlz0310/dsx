package secret

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// RunWithEnv は環境変数を注入してコマンドを実行します。
func RunWithEnv(args []string, envVars map[string]string) error {
	if len(args) == 0 {
		return fmt.Errorf("コマンドが指定されていません")
	}

	cmdName := args[0]
	cmdArgs := args[1:]

	// 実行可能ファイルのパスを取得
	cmdPath, err := exec.LookPath(cmdName)
	if err != nil {
		return fmt.Errorf("コマンド '%s' が見つかりません: %w", cmdName, err)
	}

	// 現在の環境変数を取得
	currentEnv := os.Environ()

	// Bitwardenから取得した環境変数を追加
	for key, value := range envVars {
		currentEnv = append(currentEnv, fmt.Sprintf("%s=%s", key, value))
	}

	// コマンドを準備
	cmd := exec.CommandContext(context.Background(), cmdPath, cmdArgs...)
	cmd.Env = currentEnv
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// コマンドを実行
	err = cmd.Run()
	if err != nil {
		return err
	}

	return nil
}
