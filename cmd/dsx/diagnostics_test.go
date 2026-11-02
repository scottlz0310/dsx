package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scottlz0310/dsx/internal/config"
	"github.com/scottlz0310/dsx/internal/diag"
	"github.com/scottlz0310/dsx/internal/updater"
)

// 注意: updater の診断ログ設定はパッケージのグローバル状態です。これを書き換えるテストは t.Parallel を使いません。

func TestConfigureDiagnostics(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "never-created")

	tests := []struct {
		name        string
		cfg         func() *config.Config
		wantEnabled bool
		wantWarning bool
	}{
		{
			name:        "設定が nil なら無効",
			cfg:         func() *config.Config { return nil },
			wantEnabled: false,
		},
		{
			name:        "既定の設定は無効",
			cfg:         config.Default,
			wantEnabled: false,
		},
		{
			name: "enabled なら有効",
			cfg: func() *config.Config {
				cfg := config.Default()
				cfg.Diagnostics.Enabled = true
				cfg.Diagnostics.Dir = missingDir

				return cfg
			},
			wantEnabled: true,
		},
		{
			name: "保存先を省略しても有効（OS ごとの既定の保存先）",
			cfg: func() *config.Config {
				cfg := config.Default()
				cfg.Diagnostics.Enabled = true

				return cfg
			},
			wantEnabled: true,
		},
		{
			name: "不正な capture なら警告して無効のまま続行",
			cfg: func() *config.Config {
				cfg := config.Default()
				cfg.Diagnostics.Enabled = true
				cfg.Diagnostics.Capture = "bogus"

				return cfg
			},
			wantEnabled: false,
			wantWarning: true,
		},
		{
			name: "容量の上限が 0 なら警告して無効のまま続行",
			cfg: func() *config.Config {
				cfg := config.Default()
				cfg.Diagnostics.Enabled = true
				cfg.Diagnostics.MaxTotalMB = 0

				return cfg
			},
			wantEnabled: false,
			wantWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 前の設定が残っていても、設定のたびに作り直される（無効なら確実に外れる）ことを確認する
			previous, err := diag.New(diag.Options{Dir: t.TempDir(), Capture: diag.CaptureAll, MaxFiles: 1, MaxTotalBytes: 1 << 20}, diag.Meta{})
			if err != nil {
				t.Fatal(err)
			}

			updater.SetDiagnostics(previous)
			t.Cleanup(func() { updater.SetDiagnostics(nil) })

			stderr := captureStderr(t, func() { configureDiagnostics(tt.cfg(), "sys update") })

			if got := updater.DiagnosticsEnabled(); got != tt.wantEnabled {
				t.Errorf("DiagnosticsEnabled() = %v, want %v", got, tt.wantEnabled)
			}

			if got := strings.Contains(stderr, "診断ログを無効にして続行します"); got != tt.wantWarning {
				t.Errorf("警告 = %v, want %v（stderr: %q）", got, tt.wantWarning, stderr)
			}
		})
	}

	if _, err := os.Stat(missingDir); !os.IsNotExist(err) {
		t.Errorf("記録するまで保存先を作ってはいけません: %v", err)
	}
}
