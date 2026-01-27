package env

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsContainer(t *testing.T) {
	t.Run("CODESPACES=true", func(t *testing.T) {
		t.Setenv("CODESPACES", "true")
		t.Setenv("REMOTE_CONTAINERS", "")
		assert.True(t, IsContainer())
	})

	t.Run("REMOTE_CONTAINERS=true", func(t *testing.T) {
		t.Setenv("CODESPACES", "")
		t.Setenv("REMOTE_CONTAINERS", "true")
		assert.True(t, IsContainer())
	})

	t.Run("両方の環境変数がtrue", func(t *testing.T) {
		t.Setenv("CODESPACES", "true")
		t.Setenv("REMOTE_CONTAINERS", "true")
		assert.True(t, IsContainer())
	})
}

func TestIsWSL(t *testing.T) {
	t.Run("関数がパニックせず実行される", func(t *testing.T) {
		// IsWSL() は /proc/version の内容に依存するため環境依存
		// ここでは関数が正常に終了することを確認
		result := IsWSL()
		_ = result
	})
}

func TestGetRecommendedManagers(t *testing.T) {
	t.Run("共通のマネージャが含まれる", func(t *testing.T) {
		managers := GetRecommendedManagers()
		assert.Contains(t, managers, "go")
		assert.Contains(t, managers, "npm")
	})

	t.Run("Debian系環境ではaptが含まれる", func(t *testing.T) {
		managers := GetRecommendedManagers()
		// 実行環境がDebian系(例えばこのDevContainer)であればaptが含まれるはず
		if _, err := os.Stat("/usr/bin/apt-get"); err == nil {
			assert.Contains(t, managers, "apt")
		}
	})

	t.Run("コンテナ環境でのマネージャリスト", func(t *testing.T) {
		// コンテナ環境を強制
		t.Setenv("CODESPACES", "true")

		managers := GetRecommendedManagers()
		assert.Contains(t, managers, "go")
		assert.Contains(t, managers, "npm")
		// コンテナ環境でDebian系ならaptも含まれる
		if _, err := os.Stat("/usr/bin/apt-get"); err == nil {
			assert.Contains(t, managers, "apt")
		}
	})
}

func TestIsDebianLike(t *testing.T) {
	t.Run("/usr/bin/apt-getが存在する場合はtrue", func(t *testing.T) {
		result := isDebianLike()
		// このDevContainer環境はDebian系なのでtrueが期待される
		if _, err := os.Stat("/usr/bin/apt-get"); err == nil {
			assert.True(t, result)
		} else {
			assert.False(t, result)
		}
	})
}
