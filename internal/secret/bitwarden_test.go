package secret

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureStderrは標準エラー出力をキャプチャするテストヘルパーです。
func captureStderr(t *testing.T, f func()) string {
	t.Helper()

	old := os.Stderr

	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stderr = w

	f()

	closeErr := w.Close()
	require.NoError(t, closeErr)

	os.Stderr = old

	var buf bytes.Buffer

	_, copyErr := io.Copy(&buf, r)
	require.NoError(t, copyErr)

	return buf.String()
}

// captureStderrWithErrorは関数の戻り値も取得するstderrキャプチャヘルパーです。
func captureStderrWithError(t *testing.T, f func() error) (string, error) {
	t.Helper()

	old := os.Stderr

	r, w, pipeErr := os.Pipe()
	require.NoError(t, pipeErr)

	os.Stderr = w

	err := f()

	closeErr := w.Close()
	require.NoError(t, closeErr)

	os.Stderr = old

	var buf bytes.Buffer

	_, copyErr := io.Copy(&buf, r)
	require.NoError(t, copyErr)

	return buf.String(), err
}

func TestIsValidEnvVarName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// 正常系
		{"大文字のみ", "MYVAR", true},
		{"小文字のみ", "myvar", true},
		{"混合ケース", "MyVar", true},
		{"アンダースコアで始まる", "_PRIVATE", true},
		{"大文字とアンダースコア", "MY_VAR", true},
		{"小文字とアンダースコア", "my_var", true},
		{"数字を含む", "VAR123", true},
		{"アンダースコアで始まり数字含む", "_VAR_123", true},

		// 境界値・エッジケース
		{"空文字列", "", false},
		{"単一文字_大文字", "A", true},
		{"単一文字_小文字", "a", true},
		{"単一アンダースコア", "_", true},

		// 失敗系（エラーパス）
		{"数字で始まる", "1VAR", false},
		{"ハイフンを含む", "MY-VAR", false},
		{"ドットを含む", "MY.VAR", false},
		{"スペースを含む", "MY VAR", false},
		{"特殊文字$を含む", "MY$VAR", false},
		{"特殊文字@を含む", "MY@VAR", false},
		{"日本語を含む", "変数", false},
		{"スペースのみ", "   ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidEnvVarName(tt.input)
			assert.Equal(t, tt.expected, result, "isValidEnvVarName(%q) should return %v", tt.input, tt.expected)
		})
	}
}

func TestGetCustomFieldValue(t *testing.T) {
	tests := []struct {
		name     string
		fields   []BitwardenCustomField
		target   string
		expected string
	}{
		// 正常系
		{
			name: "フィールドが見つかる",
			fields: []BitwardenCustomField{
				{Name: "value", Value: "secret123"},
			},
			target:   "value",
			expected: "secret123",
		},
		{
			name: "大文字小文字を区別しない",
			fields: []BitwardenCustomField{
				{Name: "VALUE", Value: "secret456"},
			},
			target:   "value",
			expected: "secret456",
		},
		{
			name: "混合ケースでも見つかる",
			fields: []BitwardenCustomField{
				{Name: "VaLuE", Value: "mixedcase"},
			},
			target:   "VALUE",
			expected: "mixedcase",
		},
		{
			name: "複数フィールドから最初にマッチしたものを返す",
			fields: []BitwardenCustomField{
				{Name: "other", Value: "other_value"},
				{Name: "value", Value: "target_value"},
				{Name: "another", Value: "another_value"},
			},
			target:   "value",
			expected: "target_value",
		},

		// 境界値・エッジケース
		{
			name:     "空のフィールドリスト",
			fields:   []BitwardenCustomField{},
			target:   "value",
			expected: "",
		},
		{
			name:     "nilフィールドリスト",
			fields:   nil,
			target:   "value",
			expected: "",
		},
		{
			name: "空文字列の値",
			fields: []BitwardenCustomField{
				{Name: "value", Value: ""},
			},
			target:   "value",
			expected: "",
		},

		// 失敗系
		{
			name: "フィールドが見つからない",
			fields: []BitwardenCustomField{
				{Name: "other", Value: "other_value"},
			},
			target:   "value",
			expected: "",
		},
		{
			name: "空文字列をターゲットにする",
			fields: []BitwardenCustomField{
				{Name: "value", Value: "secret"},
			},
			target:   "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getCustomFieldValue(tt.fields, tt.target)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetEnvValue(t *testing.T) {
	tests := []struct {
		name     string
		item     *BitwardenItem
		expected string
	}{
		// 正常系: カスタムフィールドから値を取得
		{
			name: "カスタムフィールドvalueから取得",
			item: &BitwardenItem{
				Name: "env:TEST_VAR",
				Fields: []BitwardenCustomField{
					{Name: "value", Value: "custom_field_value"},
				},
			},
			expected: "custom_field_value",
		},
		// 正常系: login.password をフォールバック
		{
			name: "login.passwordからフォールバック取得",
			item: &BitwardenItem{
				Name:   "env:TEST_VAR",
				Fields: []BitwardenCustomField{},
				Login: &BitwardenLogin{
					Username: "user",
					Password: "password123",
				},
			},
			expected: "password123",
		},
		{
			name: "login.passwordの前後空白をトリム",
			item: &BitwardenItem{
				Name: "env:TEST_VAR",
				Login: &BitwardenLogin{
					Password: "  spaced_password  ",
				},
			},
			expected: "spaced_password",
		},
		// カスタムフィールドが優先される
		{
			name: "カスタムフィールドがlogin.passwordより優先",
			item: &BitwardenItem{
				Name: "env:TEST_VAR",
				Fields: []BitwardenCustomField{
					{Name: "value", Value: "field_value"},
				},
				Login: &BitwardenLogin{
					Password: "login_password",
				},
			},
			expected: "field_value",
		},

		// 境界値・エッジケース
		{
			name: "どちらも空の場合",
			item: &BitwardenItem{
				Name:   "env:TEST_VAR",
				Fields: []BitwardenCustomField{},
				Login:  nil,
			},
			expected: "",
		},
		{
			name: "login.passwordが空文字列",
			item: &BitwardenItem{
				Name:   "env:TEST_VAR",
				Fields: []BitwardenCustomField{},
				Login: &BitwardenLogin{
					Password: "",
				},
			},
			expected: "",
		},
		{
			name: "login.passwordが空白のみ",
			item: &BitwardenItem{
				Name:   "env:TEST_VAR",
				Fields: []BitwardenCustomField{},
				Login: &BitwardenLogin{
					Password: "   ",
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getEnvValue(tt.item)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessEnvItem(t *testing.T) {
	tests := []struct {
		name           string
		item           *BitwardenItem
		expectedStats  LoadStats
		expectedEnvVar string // 設定される環境変数の期待値（空ならスキップ）
		expectedErr    bool
		checkStderr    string // stderrに含まれるべき文字列
	}{
		// 正常系
		{
			name: "有効な環境変数を設定",
			item: &BitwardenItem{
				Name: "env:VALID_VAR",
				Fields: []BitwardenCustomField{
					{Name: "value", Value: "test_value"},
				},
			},
			expectedStats:  LoadStats{Loaded: 1},
			expectedEnvVar: "test_value",
			checkStderr:    "VALID_VAR を注入しました",
		},

		// エラーパス: 無効な変数名
		{
			name: "無効な変数名（数字で始まる）をスキップ",
			item: &BitwardenItem{
				Name: "env:123INVALID",
				Fields: []BitwardenCustomField{
					{Name: "value", Value: "test_value"},
				},
			},
			expectedStats:  LoadStats{Invalid: 1},
			expectedEnvVar: "",
			checkStderr:    "無効な環境変数名をスキップ",
		},

		// エラーパス: valueフィールドがない
		{
			name: "valueフィールドがない",
			item: &BitwardenItem{
				Name:   "env:NO_VALUE_VAR",
				Fields: []BitwardenCustomField{},
				Login:  nil,
			},
			expectedStats:  LoadStats{Missing: 1},
			expectedEnvVar: "",
			checkStderr:    "'value' カスタムフィールドがありません",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := &LoadStats{}

			// 環境変数をクリーンアップ
			varName := ""
			if len(tt.item.Name) > 4 {
				varName = tt.item.Name[4:] // "env:" プレフィックスを除去
			}

			if varName != "" {
				t.Setenv(varName, "") // t.Setenvは自動でクリーンアップされる
			}

			stderr := captureStderr(t, func() {
				err := processEnvItem(tt.item, stats)
				if tt.expectedErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})

			// 統計の検証
			assert.Equal(t, tt.expectedStats.Loaded, stats.Loaded, "Loaded count mismatch")
			assert.Equal(t, tt.expectedStats.Missing, stats.Missing, "Missing count mismatch")
			assert.Equal(t, tt.expectedStats.Invalid, stats.Invalid, "Invalid count mismatch")

			// 環境変数の検証
			if tt.expectedEnvVar != "" && varName != "" {
				actual := os.Getenv(varName)
				assert.Equal(t, tt.expectedEnvVar, actual, "Environment variable value mismatch")
			}

			// stderrの検証
			if tt.checkStderr != "" {
				assert.Contains(t, stderr, tt.checkStderr, "Expected stderr to contain: %s", tt.checkStderr)
			}
		})
	}
}

func TestProcessEnvItems(t *testing.T) {
	tests := []struct {
		name          string
		items         []BitwardenItem
		expectedStats LoadStats
	}{
		{
			name:          "空のアイテムリスト",
			items:         []BitwardenItem{},
			expectedStats: LoadStats{},
		},
		{
			name: "env:プレフィックスのないアイテムをスキップ",
			items: []BitwardenItem{
				{Name: "not_env:VAR", Fields: []BitwardenCustomField{{Name: "value", Value: "val"}}},
				{Name: "OTHER_ITEM", Fields: []BitwardenCustomField{{Name: "value", Value: "val"}}},
			},
			expectedStats: LoadStats{},
		},
		{
			name: "複数の有効なアイテムを処理",
			items: []BitwardenItem{
				{Name: "env:VAR1", Fields: []BitwardenCustomField{{Name: "value", Value: "val1"}}},
				{Name: "env:VAR2", Fields: []BitwardenCustomField{{Name: "value", Value: "val2"}}},
			},
			expectedStats: LoadStats{Loaded: 2},
		},
		{
			name: "混合ケース（有効と無効）",
			items: []BitwardenItem{
				{Name: "env:VALID", Fields: []BitwardenCustomField{{Name: "value", Value: "val"}}},
				{Name: "env:123INVALID", Fields: []BitwardenCustomField{{Name: "value", Value: "val"}}},
				{Name: "not_env:SKIP", Fields: []BitwardenCustomField{{Name: "value", Value: "val"}}},
				{Name: "env:NO_VALUE"},
			},
			expectedStats: LoadStats{Loaded: 1, Invalid: 1, Missing: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := &LoadStats{}

			// 環境変数をクリーンアップ（t.Setenvは自動でクリーンアップされる）
			for i := range tt.items {
				if len(tt.items[i].Name) > 4 && tt.items[i].Name[:4] == "env:" {
					t.Setenv(tt.items[i].Name[4:], "")
				}
			}

			err := processEnvItems(tt.items, stats)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStats.Loaded, stats.Loaded, "Loaded count mismatch")
			assert.Equal(t, tt.expectedStats.Missing, stats.Missing, "Missing count mismatch")
			assert.Equal(t, tt.expectedStats.Invalid, stats.Invalid, "Invalid count mismatch")
		})
	}
}

func TestPrintLoadStats(t *testing.T) {
	tests := []struct {
		name         string
		stats        *LoadStats
		expectError  bool
		errorMessage string
		checkStderr  []string // stderrに含まれるべき文字列のリスト
	}{
		// エラーケース: 何も読み込まれていない
		{
			name:         "すべてゼロの場合エラー",
			stats:        &LoadStats{Loaded: 0, Missing: 0, Invalid: 0},
			expectError:  true,
			errorMessage: "bitwarden に env: 項目が見つかりません",
		},

		// 正常系: 読み込み成功のみ
		{
			name:        "読み込み成功のみ",
			stats:       &LoadStats{Loaded: 5, Missing: 0, Invalid: 0},
			expectError: false,
			checkStderr: []string{"5 個の環境変数を読み込みました"},
		},

		// 正常系: 欠損フィールドあり
		{
			name:        "欠損フィールドあり",
			stats:       &LoadStats{Loaded: 3, Missing: 2, Invalid: 0},
			expectError: false,
			checkStderr: []string{
				"3 個の環境変数を読み込みました",
				"2 個の項目で value フィールドが見つかりませんでした",
			},
		},

		// 正常系: 無効な変数名あり
		{
			name:        "無効な変数名あり",
			stats:       &LoadStats{Loaded: 1, Missing: 0, Invalid: 3},
			expectError: false,
			checkStderr: []string{
				"1 個の環境変数を読み込みました",
				"3 個の項目で無効な環境変数名がありました",
			},
		},

		// 正常系: すべて混在
		{
			name:        "すべて混在",
			stats:       &LoadStats{Loaded: 2, Missing: 1, Invalid: 1},
			expectError: false,
			checkStderr: []string{
				"2 個の環境変数を読み込みました",
				"1 個の項目で value フィールドが見つかりませんでした",
				"1 個の項目で無効な環境変数名がありました",
			},
		},

		// エッジケース: Loaded が 0 だが Missing/Invalid がある
		{
			name:        "Loadedゼロだが他に値あり",
			stats:       &LoadStats{Loaded: 0, Missing: 1, Invalid: 0},
			expectError: false,
			checkStderr: []string{
				"0 個の環境変数を読み込みました",
				"1 個の項目で value フィールドが見つかりませんでした",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stderr, err := captureStderrWithError(t, func() error {
				return printLoadStats(tt.stats)
			})

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMessage)
			} else {
				assert.NoError(t, err)

				for _, expected := range tt.checkStderr {
					assert.Contains(t, stderr, expected, "stderr should contain: %s", expected)
				}
			}
		})
	}
}
