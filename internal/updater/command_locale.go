package updater

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// localeCEnv は LANG/LC_ALL を C に固定した子プロセス用の環境変数を返します。
// 出力を解析するコマンドは、ロケールによってメッセージや表の見出しが翻訳され、
// 英語の出力を前提にした解析が壊れる（例: apt の "upgradable from:" が消える）ため、
// 子プロセスにだけ渡す。親シェルや Arbor の環境は変更しない。
func localeCEnv() []string {
	return append(os.Environ(), "LANG=C", "LC_ALL=C")
}

// runCommandOutputWithLocaleC は LANG/LC_ALL を C に固定してコマンド出力を取得します。
func runCommandOutputWithLocaleC(ctx context.Context, command string, args []string, errFormat string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command, args...) //nolint:gosec // G702: 呼び出し側では command は "go"/"git" 等の固定値のみを渡す設計

	cmd.Env = localeCEnv()

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf(errFormat, buildCommandOutputErr(err, combineCommandOutputs(output, stderr.Bytes())))
	}

	return output, nil
}

// runCommandOutputWithLocaleCAllowExitCodes は LANG/LC_ALL を C に固定してコマンド出力を取得します。
// allowedExtraCodes に含まれる exit code はエラーとして扱いません。
func runCommandOutputWithLocaleCAllowExitCodes(ctx context.Context, command string, args []string, errFormat string, allowedExtraCodes ...int) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command, args...)

	cmd.Env = localeCEnv()

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError

		if errors.As(err, &exitErr) {
			for _, code := range allowedExtraCodes {
				if exitErr.ExitCode() == code {
					return output, nil
				}
			}
		}

		return nil, fmt.Errorf(errFormat, buildCommandOutputErr(err, combineCommandOutputs(output, stderr.Bytes())))
	}

	return output, nil
}
