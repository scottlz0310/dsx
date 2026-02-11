package updater

import (
	"bytes"
	"fmt"
	"strings"
)

// buildCommandOutputErr はコマンドエラーに出力内容を付加します。
func buildCommandOutputErr(baseErr error, output []byte) error {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return baseErr
	}

	return fmt.Errorf("%w: %s", baseErr, trimmed)
}

// combineCommandOutputs は stdout/stderr を結合し、エラー表示しやすい形式に整形します。
func combineCommandOutputs(stdout, stderr []byte) []byte {
	stdoutTrimmed := bytes.TrimSpace(stdout)
	stderrTrimmed := bytes.TrimSpace(stderr)

	switch {
	case len(stdoutTrimmed) == 0 && len(stderrTrimmed) == 0:
		return nil
	case len(stdoutTrimmed) == 0:
		return append([]byte(nil), stderrTrimmed...)
	case len(stderrTrimmed) == 0:
		return append([]byte(nil), stdoutTrimmed...)
	default:
		combined := make([]byte, 0, len(stdoutTrimmed)+len(stderrTrimmed)+1)
		combined = append(combined, stdoutTrimmed...)
		combined = append(combined, '\n')
		combined = append(combined, stderrTrimmed...)

		return combined
	}
}
