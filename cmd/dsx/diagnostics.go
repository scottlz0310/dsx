package main

import (
	"fmt"
	"os"

	"golang.org/x/term"

	"github.com/scottlz0310/dsx/internal/config"
	"github.com/scottlz0310/dsx/internal/diag"
	"github.com/scottlz0310/dsx/internal/updater"
)

const bytesPerMB = 1 << 20

// configureDiagnostics は、設定に応じて診断ログ（解析対象コマンドの生出力と環境情報の保存）を有効にします。
// 既定は無効です。無効のときは、ディレクトリもファイルも作りません。
// 診断ログの準備に失敗しても、更新は止めず、診断ログを無効にして続行します（警告を表示する）。
func configureDiagnostics(cfg *config.Config, command string) {
	updater.SetDiagnostics(nil)

	if cfg == nil || !cfg.Diagnostics.Enabled {
		return
	}

	settings := cfg.Diagnostics

	recorder, err := diag.New(diag.Options{
		Dir:           settings.Dir,
		Capture:       diag.Capture(settings.Capture),
		MaxFiles:      settings.MaxFiles,
		MaxTotalBytes: int64(settings.MaxTotalMB) * bytesPerMB,
	}, diag.Meta{
		DsxVersion: version,
		Command:    command,
		StdoutTTY:  term.IsTerminal(int(os.Stdout.Fd())),
		EventsFile: activeEvents != nil,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  診断ログを無効にして続行します: %v\n", err)

		return
	}

	updater.SetDiagnostics(recorder)
}
