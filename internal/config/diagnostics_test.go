package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scottlz0310/dsx/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefault_Diagnostics(t *testing.T) {
	cfg := Default()

	assert.False(t, cfg.Diagnostics.Enabled, "診断ログは既定で無効")
	assert.Equal(t, DiagnosticsCaptureAnomaly, cfg.Diagnostics.Capture)
	assert.Empty(t, cfg.Diagnostics.Dir)
	assert.Equal(t, DiagnosticsDefaultMaxFiles, cfg.Diagnostics.MaxFiles)
	assert.Equal(t, DiagnosticsDefaultMaxTotalMB, cfg.Diagnostics.MaxTotalMB)
}

func TestLoad_Diagnostics(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantCfg DiagnosticsConfig
	}{
		{
			name: "diagnostics を書かなければ既定値（無効）",
			yaml: "version: 1\n",
			wantCfg: DiagnosticsConfig{
				Enabled: false, Capture: DiagnosticsCaptureAnomaly,
				MaxFiles: DiagnosticsDefaultMaxFiles, MaxTotalMB: DiagnosticsDefaultMaxTotalMB,
			},
		},
		{
			name: "enabled だけ書けば他は既定値",
			yaml: "version: 1\ndiagnostics:\n  enabled: true\n",
			wantCfg: DiagnosticsConfig{
				Enabled: true, Capture: DiagnosticsCaptureAnomaly,
				MaxFiles: DiagnosticsDefaultMaxFiles, MaxTotalMB: DiagnosticsDefaultMaxTotalMB,
			},
		},
		{
			name: "すべて指定",
			yaml: "version: 1\ndiagnostics:\n  enabled: true\n  capture: all\n  dir: /var/tmp/dsx-diag\n  max_files: 5\n  max_total_mb: 3\n",
			wantCfg: DiagnosticsConfig{
				Enabled: true, Capture: DiagnosticsCaptureAll, Dir: "/var/tmp/dsx-diag", MaxFiles: 5, MaxTotalMB: 3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.SetTestHome(t, t.TempDir())

			home, err := os.UserHomeDir()
			require.NoError(t, err)

			configDir := filepath.Join(home, ".config", "dsx")
			require.NoError(t, os.MkdirAll(configDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(tt.yaml), 0o644))

			cfg, err := Load()
			require.NoError(t, err)

			assert.Equal(t, tt.wantCfg, cfg.Diagnostics)
		})
	}
}

func TestValidate_Diagnostics(t *testing.T) {
	absDir := filepath.Join(t.TempDir(), "diag")

	tests := []struct {
		name       string
		diag       DiagnosticsConfig
		wantFields []string
	}{
		{
			name: "無効なら不正な値でも検証しない",
			diag: DiagnosticsConfig{Enabled: false, Capture: "bogus", MaxFiles: 0, MaxTotalMB: 0},
		},
		{
			name: "既定値は有効にしても正常",
			diag: DiagnosticsConfig{Enabled: true, Capture: DiagnosticsCaptureAnomaly, MaxFiles: 20, MaxTotalMB: 20},
		},
		{
			name: "all と絶対パスの保存先は正常",
			diag: DiagnosticsConfig{Enabled: true, Capture: DiagnosticsCaptureAll, Dir: absDir, MaxFiles: 1, MaxTotalMB: 1},
		},
		{
			name:       "未知の capture",
			diag:       DiagnosticsConfig{Enabled: true, Capture: "bogus", MaxFiles: 20, MaxTotalMB: 20},
			wantFields: []string{"diagnostics.capture"},
		},
		{
			name:       "capture が空",
			diag:       DiagnosticsConfig{Enabled: true, Capture: "", MaxFiles: 20, MaxTotalMB: 20},
			wantFields: []string{"diagnostics.capture"},
		},
		{
			name:       "max_files が 0",
			diag:       DiagnosticsConfig{Enabled: true, Capture: DiagnosticsCaptureAnomaly, MaxFiles: 0, MaxTotalMB: 20},
			wantFields: []string{"diagnostics.max_files"},
		},
		{
			name:       "max_total_mb が負",
			diag:       DiagnosticsConfig{Enabled: true, Capture: DiagnosticsCaptureAnomaly, MaxFiles: 20, MaxTotalMB: -1},
			wantFields: []string{"diagnostics.max_total_mb"},
		},
		{
			name:       "相対パスの保存先",
			diag:       DiagnosticsConfig{Enabled: true, Capture: DiagnosticsCaptureAnomaly, Dir: "relative/dir", MaxFiles: 20, MaxTotalMB: 20},
			wantFields: []string{"diagnostics.dir"},
		},
		{
			name:       "複数の誤りは全件報告する",
			diag:       DiagnosticsConfig{Enabled: true, Capture: "x", Dir: "rel", MaxFiles: 0, MaxTotalMB: 0},
			wantFields: []string{"diagnostics.capture", "diagnostics.max_files", "diagnostics.max_total_mb", "diagnostics.dir"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.Repo.Root = t.TempDir()
			cfg.Diagnostics = tt.diag

			result := Validate(cfg, ValidateOptions{})

			var got []string

			for _, issue := range result.Errors {
				if len(issue.Field) > len("diagnostics.") && issue.Field[:len("diagnostics.")] == "diagnostics." {
					got = append(got, issue.Field)
				}
			}

			assert.ElementsMatch(t, tt.wantFields, got)
		})
	}
}
