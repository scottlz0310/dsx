package snapshot

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecode(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		invalid     bool
	}{
		{"通常", "version: 1\ntools:\n  uv:\n    - package: ruff\n      version: 0.12.0\n", false},
		{"空の構成", "version: 1\ntools: {}", false},
		{"空一覧", "version: 1\ntools: {uv: []}", false},
		{"空ファイル", "", true},
		{"null", "null", true},
		{"版なし", "tools: {}", true},
		{"未来のスキーマ", "version: 2\ntools: {}", true},
		{"tools なし", "version: 1", true},
		{"一覧が null", "version: 1\ntools: {uv: null}", true},
		{"未知のマネージャ", "version: 1\ntools: {unknown: []}", true},
		{"未知のフィールド", "version: 1\ntools: {}\nsecret: hidden", true},
		{"ツールの未知フィールド", "version: 1\ntools: {uv: [{package: ruff, password: hidden}]}", true},
		{"複数文書", "version: 1\ntools: {}\n---\nversion: 1\ntools: {}", true},
		{"重複キー", "version: 1\nversion: 1\ntools: {}", true},
		{"認証 URL", "version: 1\ntools: {npm: [{package: 'https://user:hidden@example.org'}]}", true},
		{"シェル文字", "version: 1\ntools: {uv: [{package: 'ruff; echo hidden'}]}", true},
		{"型の不正", "version: 1\ntools: hidden", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest, err := Decode(strings.NewReader(tc.input))
			if tc.invalid {
				require.Error(t, err)
				require.Nil(t, manifest)
				require.NotContains(t, err.Error(), "hidden")
				return
			}

			require.NoError(t, err)
			require.Equal(t, 1, manifest.Version)
		})
	}
}
