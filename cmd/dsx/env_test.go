package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/scottlz0310/dsx/internal/secret"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupEnvCommandMocks(t *testing.T) {
	t.Helper()

	origGetEnvVars := getEnvVarsFunc
	origGetStatus := getBitwardenSessionStatusFunc
	origExportFormat := exportFormatFunc
	origFormatForShell := formatForShellFunc
	origDetectShell := detectShellFunc
	origStatusQuiet := envStatusQuiet

	t.Cleanup(func() {
		getEnvVarsFunc = origGetEnvVars
		getBitwardenSessionStatusFunc = origGetStatus
		exportFormatFunc = origExportFormat
		formatForShellFunc = origFormatForShell
		detectShellFunc = origDetectShell
		envStatusQuiet = origStatusQuiet
	})
}

func TestRunEnvExport(t *testing.T) {
	tests := []struct {
		name           string
		bwSession      string
		getEnvErr      error
		exportErr      error
		sessionErr     error
		wantErr        string
		wantOutput     []string
		wantSessionMap map[string]string
	}{
		{
			name:           "BW_SESSIONを先頭に含めて環境変数を出力する",
			bwSession:      "session-token",
			wantOutput:     []string{"SESSION_EXPORT", "ENV_EXPORT"},
			wantSessionMap: map[string]string{"BW_SESSION": "session-token"},
		},
		{
			name:      "セッション確保失敗は環境変数取得エラーとして返す",
			getEnvErr: errors.New("unlock failed"),
			wantErr:   "環境変数の取得に失敗しました",
		},
		{
			name:      "env項目取得失敗は環境変数取得エラーとして返す",
			getEnvErr: errors.New("list failed"),
			wantErr:   "環境変数の取得に失敗しました",
		},
		{
			name:      "env項目のエクスポート形式生成失敗を返す",
			exportErr: errors.New("format failed"),
			wantErr:   "エクスポート形式の生成に失敗しました",
		},
		{
			name:       "BW_SESSIONのエクスポート形式生成失敗を返す",
			sessionErr: errors.New("session format failed"),
			wantErr:    "BW_SESSION のエクスポート形式の生成に失敗しました",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupEnvCommandMocks(t)

			if tt.bwSession != "" {
				t.Setenv("BW_SESSION", tt.bwSession)
			}

			getEnvVarsFunc = func() (map[string]string, error) {
				return map[string]string{"TEST_VAR": "secret-value"}, tt.getEnvErr
			}
			exportFormatFunc = func(envVars map[string]string) (string, error) {
				assert.Equal(t, map[string]string{"TEST_VAR": "secret-value"}, envVars)

				return "ENV_EXPORT", tt.exportErr
			}
			formatForShellFunc = func(envVars map[string]string, shellType secret.ShellType) (string, error) {
				if tt.wantSessionMap != nil {
					assert.Equal(t, tt.wantSessionMap, envVars)
				}

				assert.Equal(t, secret.ShellPowerShell, shellType)

				return "SESSION_EXPORT", tt.sessionErr
			}
			detectShellFunc = func() secret.ShellType {
				return secret.ShellPowerShell
			}

			output := captureStdout(t, func() {
				err := runEnvExport(&cobra.Command{}, nil)
				if tt.wantErr != "" {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.wantErr)

					return
				}

				require.NoError(t, err)
			})

			for _, want := range tt.wantOutput {
				assert.Contains(t, output, want)
			}

			if len(tt.wantOutput) == 2 {
				assert.Less(t, strings.Index(output, tt.wantOutput[0]), strings.Index(output, tt.wantOutput[1]))
			}
		})
	}
}

func TestRunEnvStatus(t *testing.T) {
	tests := []struct {
		name         string
		quiet        bool
		status       string
		statusErr    error
		wantErr      string
		wantOutput   string
		wantNoOutput bool
	}{
		{
			name:         "quietでアンロック済みなら出力せず成功する",
			quiet:        true,
			status:       "unlocked",
			wantNoOutput: true,
		},
		{
			name:    "quietでロック中なら非ゼロ相当のエラーを返す",
			quiet:   true,
			status:  "locked",
			wantErr: "bitwarden はアンロックされていません",
		},
		{
			name:       "通常表示でBW_SESSION未設定を表示する",
			status:     "missing",
			wantOutput: "BW_SESSION が設定されていません。",
		},
		{
			name:       "通常表示で未知の状態を表示する",
			status:     "unauthenticated",
			wantOutput: "Bitwarden の状態: unauthenticated",
		},
		{
			name:      "状態確認失敗をエラーとして返す",
			statusErr: errors.New("status failed"),
			wantErr:   "bitwarden セッション状態の確認に失敗しました",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupEnvCommandMocks(t)

			envStatusQuiet = tt.quiet
			getBitwardenSessionStatusFunc = func() (string, error) {
				return tt.status, tt.statusErr
			}

			output := captureStdout(t, func() {
				err := runEnvStatus(&cobra.Command{}, nil)
				if tt.wantErr != "" {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.wantErr)

					return
				}

				require.NoError(t, err)
			})

			if tt.wantNoOutput {
				assert.Empty(t, strings.TrimSpace(output))
			}

			if tt.wantOutput != "" {
				assert.Contains(t, output, tt.wantOutput)
			}
		})
	}
}
