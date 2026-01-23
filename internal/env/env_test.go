package env

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsContainer(t *testing.T) {
	// このテストは実行環境に依存するため、
	// 確実にコントロールできる環境変数のケースのみを検証します。

	// ケース1: 環境変数なし (ローカル想定だが、CI/DevContainer環境ではtrueになる可能性があるためSkip推奨か、値を確認)
	// ここでは明示的に環境変数をセットするケースをテストする。

	t.Run("CODESPACES=true", func(t *testing.T) {
		t.Setenv("CODESPACES", "true")
		assert.True(t, IsContainer())
	})

	t.Run("REMOTE_CONTAINERS=true", func(t *testing.T) {
		t.Setenv("REMOTE_CONTAINERS", "true")
		assert.True(t, IsContainer())
	})
}

func TestGetRecommendedManagers(t *testing.T) {
	managers := GetRecommendedManagers()
	assert.Contains(t, managers, "go")
	assert.Contains(t, managers, "npm")

	// 実行環境がDebian系(例えばこのDevContainer)であればaptが含まれるはず
	if _, err := os.Stat("/usr/bin/apt-get"); err == nil {
		assert.Contains(t, managers, "apt")
	}
}
