package updater

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// effectiveEnv は環境変数のスライスから、重複キーは最後の値を採用した値を返します（os/exec と同じ解釈）。
func effectiveEnv(env []string, key string) (string, bool) {
	value, found := "", false

	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			value, found = v, true
		}
	}

	return value, found
}

func TestLocaleCEnv(t *testing.T) {
	testCases := []struct {
		name string
		lang string
		lc   string
	}{
		{"日本語ロケールを C に上書きする", "ja_JP.UTF-8", "ja_JP.UTF-8"},
		{"LANG のみ設定されていても C に上書きする", "ja_JP.UTF-8", ""},
		{"未設定でも C を設定する", "", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LANG", tc.lang)
			t.Setenv("LC_ALL", tc.lc)
			t.Setenv("DSX_TEST_KEEP", "kept")

			env := localeCEnv()

			lang, _ := effectiveEnv(env, "LANG")
			lcAll, _ := effectiveEnv(env, "LC_ALL")
			keep, found := effectiveEnv(env, "DSX_TEST_KEEP")

			assert.Equal(t, "C", lang)
			assert.Equal(t, "C", lcAll)
			assert.True(t, found, "他の環境変数は引き継ぐ")
			assert.Equal(t, "kept", keep)
		})
	}
}
