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
}
