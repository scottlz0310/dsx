package secret

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidExportKey(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		valid bool
	}{
		{"大文字のみ", "MYVAR", true},
		{"大文字とアンダースコア", "MY_VAR", true},
		{"大文字と数字", "VAR123", true},
		{"アンダースコアで始まる", "_PRIVATE", true},
		{"小文字を含む", "myVar", false},
		{"小文字のみ", "myvar", false},
		{"数字で始まる", "1VAR", false},
		{"ハイフンを含む", "MY-VAR", false},
		{"空文字列", "", false},
		{"ドットを含む", "MY.VAR", false},
		{"スペースを含む", "MY VAR", false},
		{"特殊文字を含む", "MY$VAR", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidExportKey(tt.key)
			assert.Equal(t, tt.valid, result, "IsValidExportKey(%q) should return %v", tt.key, tt.valid)
		})
	}
}

func TestFormatPosixExport(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		expected string
	}{
		{
			name:     "単純な値",
			key:      "MYVAR",
			value:    "simple_value",
			expected: "export MYVAR='simple_value'",
		},
		{
			name:     "空白を含む値",
			key:      "PATH_VAR",
			value:    "path with spaces",
			expected: "export PATH_VAR='path with spaces'",
		},
		{
			name:     "単一引用符を含む値",
			key:      "QUOTED",
			value:    "it's working",
			expected: "export QUOTED='it'\\''s working'",
		},
		{
			name:     "複数の単一引用符",
			key:      "MULTI_QUOTE",
			value:    "don't can't won't",
			expected: "export MULTI_QUOTE='don'\\''t can'\\''t won'\\''t'",
		},
		{
			name:     "特殊文字を含む値",
			key:      "SPECIAL",
			value:    "$PATH:~/.local/bin",
			expected: "export SPECIAL='$PATH:~/.local/bin'",
		},
		{
			name:     "空文字列",
			key:      "EMPTY",
			value:    "",
			expected: "export EMPTY=''",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatPosixExport(tt.key, tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatPowerShellExport(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		expected string
	}{
		{
			name:     "単純な値",
			key:      "MYVAR",
			value:    "simple_value",
			expected: "$env:MYVAR = 'simple_value'",
		},
		{
			name:     "空白を含む値",
			key:      "PATH_VAR",
			value:    "path with spaces",
			expected: "$env:PATH_VAR = 'path with spaces'",
		},
		{
			name:     "単一引用符を含む値",
			key:      "QUOTED",
			value:    "it's working",
			expected: "$env:QUOTED = 'it''s working'",
		},
		{
			name:     "複数の単一引用符",
			key:      "MULTI_QUOTE",
			value:    "don't can't won't",
			expected: "$env:MULTI_QUOTE = 'don''t can''t won''t'",
		},
		{
			name:     "特殊文字を含む値",
			key:      "SPECIAL",
			value:    "$env:PATH;C:\\Program Files",
			expected: "$env:SPECIAL = '$env:PATH;C:\\Program Files'",
		},
		{
			name:     "空文字列",
			key:      "EMPTY",
			value:    "",
			expected: "$env:EMPTY = ''",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatPowerShellExport(tt.key, tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatForShell(t *testing.T) {
	envVars := map[string]string{
		"VALID_VAR":   "value1",
		"ANOTHER_VAR": "value with spaces",
		"invalid_var": "this should be skipped", // 小文字なのでスキップされる
	}

	t.Run("Bash形式", func(t *testing.T) {
		output, err := FormatForShell(envVars, ShellBash)
		assert.NoError(t, err)
		assert.Contains(t, output, "export VALID_VAR='value1'")
		assert.Contains(t, output, "export ANOTHER_VAR='value with spaces'")
		assert.NotContains(t, output, "invalid_var")
	})

	t.Run("PowerShell形式", func(t *testing.T) {
		output, err := FormatForShell(envVars, ShellPowerShell)
		assert.NoError(t, err)
		assert.Contains(t, output, "$env:VALID_VAR = 'value1'")
		assert.Contains(t, output, "$env:ANOTHER_VAR = 'value with spaces'")
		assert.NotContains(t, output, "invalid_var")
	})

	t.Run("改行を含む値", func(t *testing.T) {
		envVarsWithNewline := map[string]string{
			"VALID_VAR":   "value1",
			"NEWLINE_VAR": "line1\nline2",
		}
		output, err := FormatForShell(envVarsWithNewline, ShellBash)
		// VALID_VARはあるが、NEWLINE_VARはスキップされる
		assert.NoError(t, err)
		assert.Contains(t, output, "export VALID_VAR='value1'")
		assert.NotContains(t, output, "NEWLINE_VAR")
	})

	t.Run("有効な変数が1つもない", func(t *testing.T) {
		invalidVars := map[string]string{
			"invalid": "value",
		}
		_, err := FormatForShell(invalidVars, ShellBash)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "有効な環境変数がありません")
	})

	t.Run("Zsh形式はBash形式と同じ", func(t *testing.T) {
		singleVar := map[string]string{"MY_VAR": "myvalue"}
		output, err := FormatForShell(singleVar, ShellZsh)
		assert.NoError(t, err)
		assert.Equal(t, "export MY_VAR='myvalue'", output)
	})

	t.Run("CRを含む値もスキップ", func(t *testing.T) {
		envVarsWithCR := map[string]string{
			"VALID_VAR": "value1",
			"CR_VAR":    "line1\rline2",
		}
		output, err := FormatForShell(envVarsWithCR, ShellBash)
		assert.NoError(t, err)
		assert.Contains(t, output, "export VALID_VAR='value1'")
		assert.NotContains(t, output, "CR_VAR")
	})

	t.Run("空のmap", func(t *testing.T) {
		_, err := FormatForShell(map[string]string{}, ShellBash)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "有効な環境変数がありません")
	})
}

func TestDetectShell(t *testing.T) {
	tests := []struct {
		name        string
		psModPath   string
		shell       string
		expected    ShellType
		description string
	}{
		// PSModulePath が設定されている場合は PowerShell
		{
			name:        "PSModulePathが設定されている場合はPowerShell",
			psModPath:   "/some/path",
			shell:       "/bin/bash",
			expected:    ShellPowerShell,
			description: "PSModulePathが設定されていればSHELL変数に関係なくPowerShell",
		},
		// SHELL 環境変数によるシェル検出
		{
			name:        "SHELL環境変数がzshの場合",
			psModPath:   "",
			shell:       "/bin/zsh",
			expected:    ShellZsh,
			description: "SHELL=/bin/zsh の場合は zsh",
		},
		{
			name:        "SHELL環境変数がbashの場合",
			psModPath:   "",
			shell:       "/bin/bash",
			expected:    ShellBash,
			description: "SHELL=/bin/bash の場合は bash",
		},
		{
			name:        "SHELL環境変数がzshへのフルパス",
			psModPath:   "",
			shell:       "/usr/local/bin/zsh",
			expected:    ShellZsh,
			description: "フルパスでも basename でシェルを判定",
		},
		// デフォルトケース
		{
			name:        "SHELL環境変数が空の場合はデフォルトbash",
			psModPath:   "",
			shell:       "",
			expected:    ShellBash,
			description: "SHELL が設定されていない場合はデフォルト bash",
		},
		{
			name:        "不明なシェルの場合はデフォルトbash",
			psModPath:   "",
			shell:       "/bin/fish",
			expected:    ShellBash,
			description: "fish など未対応シェルはデフォルト bash",
		},
		{
			name:        "カスタムシェルパス",
			psModPath:   "",
			shell:       "/custom/path/to/bash",
			expected:    ShellBash,
			description: "カスタムパスでも basename が bash なら bash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 環境変数を設定
			t.Setenv("PSModulePath", tt.psModPath)
			t.Setenv("SHELL", tt.shell)

			result := DetectShell()
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestGetShellName(t *testing.T) {
	tests := []struct {
		name      string
		psModPath string
		shell     string
		expected  string
	}{
		{
			name:      "bash",
			psModPath: "",
			shell:     "/bin/bash",
			expected:  "bash",
		},
		{
			name:      "zsh",
			psModPath: "",
			shell:     "/bin/zsh",
			expected:  "zsh",
		},
		{
			name:      "powershell",
			psModPath: "/some/path",
			shell:     "",
			expected:  "powershell",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PSModulePath", tt.psModPath)
			t.Setenv("SHELL", tt.shell)

			result := GetShellName()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetShellExecutable(t *testing.T) {
	tests := []struct {
		name      string
		psModPath string
		shell     string
		checkFunc func(t *testing.T, result string)
	}{
		{
			name:      "SHELL環境変数が設定されている場合はそれを返す",
			psModPath: "",
			shell:     "/bin/bash",
			checkFunc: func(t *testing.T, result string) {
				assert.Equal(t, "/bin/bash", result)
			},
		},
		{
			name:      "SHELL環境変数が空の場合は/bin/shを返す",
			psModPath: "",
			shell:     "",
			checkFunc: func(t *testing.T, result string) {
				assert.Equal(t, "/bin/sh", result)
			},
		},
		{
			name:      "PSModulePathが設定されている場合はpwshまたはpowershellを返す",
			psModPath: "/some/path",
			shell:     "",
			checkFunc: func(t *testing.T, result string) {
				// pwsh がインストールされている場合は pwsh、なければ powershell
				assert.Contains(t, []string{"pwsh", "powershell"}, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PSModulePath", tt.psModPath)
			t.Setenv("SHELL", tt.shell)

			result := GetShellExecutable()
			tt.checkFunc(t, result)
		})
	}
}
