package secret

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var (
	// validExportKeyRegex は環境変数名の検証用の正規表現です
	validExportKeyRegex = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
)

// ShellType はサポートされているシェルの種類を表します。
type ShellType string

const (
	ShellBash       ShellType = "bash"
	ShellZsh        ShellType = "zsh"
	ShellPowerShell ShellType = "powershell"
)

// ExportFormat はenv exportの出力を生成します。
func ExportFormat(envVars map[string]string) (string, error) {
	shellType := DetectShell()
	return FormatForShell(envVars, shellType)
}

// DetectShell は現在のシェルを検出します。
func DetectShell() ShellType {
	// Windowsの場合、PowerShellを返す
	if runtime.GOOS == "windows" || os.Getenv("PSModulePath") != "" {
		return ShellPowerShell
	}

	// SHELL 環境変数から検出 (Linux/macOS)
	shell := os.Getenv("SHELL")
	if shell != "" {
		baseName := filepath.Base(shell)
		if baseName == "zsh" {
			return ShellZsh
		}
		if baseName == "bash" {
			return ShellBash
		}
	}

	// デフォルトはbash
	return ShellBash
}

// FormatForShell は指定されたシェル用の export 文を生成します。
func FormatForShell(envVars map[string]string, shellType ShellType) (string, error) {
	var lines []string

	for key, value := range envVars {
		// KEY名の検証
		if !IsValidExportKey(key) {
			// 無効なKEY名はスキップ（標準エラーに警告を出す）
			fmt.Fprintf(os.Stderr, "⚠️  無効な環境変数名をスキップ: %s\n", key)
			continue
		}

		// 改行を含む値は拒否
		if strings.Contains(value, "\n") || strings.Contains(value, "\r") {
			fmt.Fprintf(os.Stderr, "⚠️  改行を含む値はサポートされていません: %s\n", key)
			continue
		}

		var line string
		switch shellType {
		case ShellPowerShell:
			line = formatPowerShellExport(key, value)
		default: // ShellBash, ShellZsh
			line = formatPosixExport(key, value)
		}
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return "", fmt.Errorf("有効な環境変数がありません")
	}

	return strings.Join(lines, "\n"), nil
}

// IsValidExportKey は環境変数名がエクスポートに適しているかを検証します。
// 大文字・アンダースコア・数字のみを許可（POSIX互換 + 慣例的に大文字推奨）
// 注意: bitwarden.go の isValidEnvVarName は小文字も許可しますが、
// export コマンドでは大文字のみを要求します（シェルの慣例に従う）。
func IsValidExportKey(key string) bool {
	return validExportKeyRegex.MatchString(key)
}

// formatPosixExport はbash/zsh用のexport文を生成します。
// 単一引用符でクオートし、値の中の単一引用符は '\'' でエスケープします。
func formatPosixExport(key, value string) string {
	// 単一引用符内では全ての文字がリテラルとして扱われる（改行以外）
	// 単一引用符自体をエスケープするには: 'text'\''more'
	escapedValue := strings.ReplaceAll(value, "'", "'\\''")
	return fmt.Sprintf("export %s='%s'", key, escapedValue)
}

// formatPowerShellExport はPowerShell用の変数代入文を生成します。
// 単一引用符でクオートし、値の中の単一引用符は '' でエスケープします。
func formatPowerShellExport(key, value string) string {
	// PowerShellでは単一引用符内で '' が単一引用符のエスケープ
	escapedValue := strings.ReplaceAll(value, "'", "''")
	return fmt.Sprintf("$env:%s = '%s'", key, escapedValue)
}

// GetShellName はシェル判定用のデバッグ情報を返します。
func GetShellName() string {
	shellType := DetectShell()
	return string(shellType)
}

// GetShellExecutable は現在のシェルの実行ファイルパスを返します。
func GetShellExecutable() string {
	if runtime.GOOS == "windows" || os.Getenv("PSModulePath") != "" {
		// PowerShell Core (pwsh) の存在を確認
		if _, err := exec.LookPath("pwsh"); err == nil {
			return "pwsh"
		}
		return "powershell"
	}

	shell := os.Getenv("SHELL")
	if shell != "" {
		return shell
	}

	return "/bin/sh"
}
