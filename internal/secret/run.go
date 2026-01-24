package secret

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
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
	cmd := exec.Command(cmdPath, cmdArgs...)
	cmd.Env = currentEnv
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// コマンドを実行
	if err := cmd.Run(); err != nil {
		// 終了コードを保持して返す
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				os.Exit(status.ExitStatus())
			}
		}
		return err
	}

	return nil
}
