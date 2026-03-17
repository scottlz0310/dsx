package secret

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// RunWithEnv は環境変数を注入してコマンドを実行します（プロセス完了まで待機）。
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
	currentEnv := mergeEnv(os.Environ(), envVars)

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

// RunWithEnvDetach は環境変数を注入してコマンドをデタッチ起動します。
// GUIアプリなどプロセスの完了を待たない場合に使用します。
// ターミナルとの標準I/O接続を切り離してバックグラウンド起動します。
func RunWithEnvDetach(args []string, envVars map[string]string) error {
	if len(args) == 0 {
		return fmt.Errorf("コマンドが指定されていません")
	}

	cmdPath := args[0]
	cmdArgs := args[1:]

	// PATH を通じた検索を試みる（失敗した場合は絶対パスとしてそのまま使用）
	if resolved, err := exec.LookPath(cmdPath); err == nil {
		cmdPath = resolved
	}

	currentEnv := mergeEnv(os.Environ(), envVars)

	cmd := exec.Command(cmdPath, cmdArgs...)
	cmd.Env = currentEnv
	// ターミナルとのI/Oを切り離す（GUIアプリのデタッチ起動）
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("コマンドの起動に失敗しました: %w", err)
	}

	// プロセスをデタッチ（待機せず即時リターン）
	// Wait はゾンビプロセス回避のため goroutine で処理
	go func() { _ = cmd.Wait() }()

	fmt.Fprintf(os.Stderr, "✅ プロセスを起動しました (PID: %d)\n", cmd.Process.Pid)

	return nil
}

func mergeEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}

	overrideKV := make(map[string]string, len(overrides))
	for key, value := range overrides {
		overrideKV[normalizeEnvKey(key)] = fmt.Sprintf("%s=%s", key, value)
	}

	filtered := make([]string, 0, len(base)+len(overrideKV))
	for _, kv := range base {
		key := kv
		if idx := strings.IndexByte(kv, '='); idx != -1 {
			key = kv[:idx]
		}

		if _, ok := overrideKV[normalizeEnvKey(key)]; ok {
			continue
		}

		filtered = append(filtered, kv)
	}

	for _, kv := range overrideKV {
		filtered = append(filtered, kv)
	}

	return filtered
}
