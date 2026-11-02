package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/scottlz0310/dsx/internal/diag"
)

const (
	// versionFlag はツールの版を表示する引数です。
	versionFlag = "--version"

	toolVersionTimeout = 5 * time.Second
	toolVersionMaxLen  = 200
)

var (
	diagnostics  *diag.Recorder
	diagWarnOnce sync.Once
)

// SetDiagnostics は、解析対象コマンドの出力を診断ログへ保存する Recorder を設定します。
// nil は無効（既定）を表します。ジョブの開始前に 1 回だけ呼ぶ前提です。
func SetDiagnostics(recorder *diag.Recorder) {
	diagnostics = recorder
	diagWarnOnce = sync.Once{}
}

// DiagnosticsEnabled は、診断ログが有効（Recorder が設定済み）かどうかを返します。
func DiagnosticsEnabled() bool {
	return diagnostics != nil
}

// diagCapture は、診断ログに記録する解析対象コマンド（確認系コマンド）の結果です。
type diagCapture struct {
	Manager string
	// Command は実行したコマンドと引数です（先頭がコマンド名）。
	Command []string
	Output  []byte
	// Err はコマンドが返したエラーです。Failed は、dsx がそれをコマンドの失敗として扱ったかどうかです。
	// （winget のように、更新ありのときも 0 以外で終了するコマンドは、Err があっても Failed ではない。）
	Err    error
	Failed bool
	// Suspect は判定不能の疑いの理由です（#180）。
	Suspect string
	// VersionArgs はツールの版を取得する引数です（例: --version）。空なら取得しません。
	VersionArgs []string
}

// recordDiagnostic は、設定に応じて、確認系コマンドの出力と環境情報を診断ログへ保存します。
// 診断ログの失敗は更新の処理を止めず、警告を 1 回だけ表示します（失敗を隠さない）。
func recordDiagnostic(ctx context.Context, c *diagCapture) {
	reason := diagnostics.Reason(c.Failed, c.Suspect)
	if reason == "" {
		return
	}

	entry := &diag.Entry{
		Manager: c.Manager,
		Command: c.Command,
		Output:  c.Output,
		Err:     c.Err,
		Suspect: c.Suspect,
	}

	if len(c.Command) > 0 && len(c.VersionArgs) > 0 {
		entry.ToolVersion = toolVersion(ctx, c.Command[0], c.VersionArgs)
	}

	if err := diagnostics.Record(reason, entry); err != nil {
		diagWarnOnce.Do(func() {
			fmt.Fprintf(os.Stderr, "⚠️  診断ログの保存に失敗しました（更新は続行します）: %v\n", err)
		})
	}
}

// toolVersion はツールの版の出力の 1 行目を返します。取得できなければ空文字列です。
func toolVersion(ctx context.Context, name string, args []string) string {
	ctx, cancel := context.WithTimeout(ctx, toolVersionTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = localeCEnv()

	output, err := cmd.CombinedOutput()
	if err != nil && len(output) == 0 {
		return ""
	}

	for _, line := range strings.Split(string(output), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			if len(trimmed) > toolVersionMaxLen {
				trimmed = trimmed[:toolVersionMaxLen]
			}

			return trimmed
		}
	}

	return ""
}
