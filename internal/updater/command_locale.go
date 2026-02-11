package updater

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
)

// runCommandOutputWithLocaleC は LANG/LC_ALL を C に固定してコマンド出力を取得します。
func runCommandOutputWithLocaleC(ctx context.Context, command string, args []string, errFormat string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command, args...)

	cmd.Env = append(os.Environ(), "LANG=C", "LC_ALL=C")

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf(errFormat, buildCommandOutputErr(err, combineCommandOutputs(output, stderr.Bytes())))
	}

	return output, nil
}
