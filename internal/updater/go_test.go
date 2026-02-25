package updater

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/scottlz0310/dsx/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractToolName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "version指定あり",
			input:    "github.com/golangci/golangci-lint/cmd/golangci-lint@latest",
			expected: "golangci-lint",
		},
		{
			name:     "version指定なし",
			input:    "golang.org/x/tools/gopls",
			expected: "gopls",
		},
		{
			name:     "単一セグメント",
			input:    "dlv@latest",
			expected: "dlv",
		},
		{
			name:     "単一セグメントでversion指定なし",
			input:    "gotests",
			expected: "gotests",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractToolName(tt.input))
		})
	}
}

func TestDefaultGoTargets(t *testing.T) {
	targets := DefaultGoTargets()
	require.NotEmpty(t, targets)

	for _, target := range targets {
		assert.Contains(t, target, "@", "デフォルトターゲットは@version指定を含む想定")
	}
}

func TestGoUpdater_Configure(t *testing.T) {
	tests := []struct {
		name           string
		cfg            config.ManagerConfig
		initialTargets []string
		expected       []string
	}{
		{
			name:           "nil設定は何もしない",
			cfg:            nil,
			initialTargets: []string{"a@latest"},
			expected:       []string{"a@latest"},
		},
		{
			name:           "targetsが[]interface{}",
			cfg:            config.ManagerConfig{"targets": []interface{}{"a@latest", 123, "b"}},
			initialTargets: nil,
			expected:       []string{"a@latest", "b"},
		},
		{
			name:           "targetsが[]string",
			cfg:            config.ManagerConfig{"targets": []string{"a@latest", "b@latest"}},
			initialTargets: nil,
			expected:       []string{"a@latest", "b@latest"},
		},
		{
			name:           "targetsの型が不正なら無視",
			cfg:            config.ManagerConfig{"targets": "a@latest"},
			initialTargets: []string{"keep"},
			expected:       []string{"keep"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GoUpdater{targets: tt.initialTargets}
			require.NoError(t, g.Configure(tt.cfg))
			assert.Equal(t, tt.expected, g.targets)
		})
	}
}

func TestGoUpdater_Check(t *testing.T) {
	t.Run("更新対象なし", func(t *testing.T) {
		g := &GoUpdater{}

		got, err := g.Check(context.Background())
		require.NoError(t, err)
		require.NotNil(t, got)

		assert.Equal(t, 0, got.AvailableUpdates)
		assert.Contains(t, got.Message, "設定されていません")
		assert.Empty(t, got.Packages)
	})

	t.Run("更新対象あり", func(t *testing.T) {
		g := &GoUpdater{
			targets: []string{
				"golang.org/x/tools/gopls@latest",
				"github.com/fatih/gomodifytags",
			},
		}

		got, err := g.Check(context.Background())
		require.NoError(t, err)
		require.NotNil(t, got)

		assert.Equal(t, 2, got.AvailableUpdates)
		assert.Contains(t, got.Message, "2 件")
		require.Len(t, got.Packages, 2)

		assert.Equal(t, "gopls", got.Packages[0].Name)
		assert.Equal(t, "@latest", got.Packages[0].NewVersion)
		assert.Equal(t, "gomodifytags", got.Packages[1].Name)
		assert.Equal(t, "@latest", got.Packages[1].NewVersion)
	})
}

func TestGoUpdater_Update_DryRun(t *testing.T) {
	t.Run("更新対象なし", func(t *testing.T) {
		g := &GoUpdater{}

		got, err := g.Update(context.Background(), UpdateOptions{DryRun: true})
		require.NoError(t, err)
		require.NotNil(t, got)

		assert.Contains(t, got.Message, "設定されていません")
		assert.Empty(t, got.Packages)
		assert.Equal(t, 0, got.UpdatedCount)
	})

	t.Run("更新対象あり", func(t *testing.T) {
		g := &GoUpdater{
			targets: []string{
				"golang.org/x/tools/gopls@latest",
				"github.com/fatih/gomodifytags",
			},
		}

		got, err := g.Update(context.Background(), UpdateOptions{DryRun: true})
		require.NoError(t, err)
		require.NotNil(t, got)

		assert.Contains(t, got.Message, "DryRun")
		require.Len(t, got.Packages, 2)

		assert.Equal(t, "gopls", got.Packages[0].Name)
		assert.Equal(t, "@latest", got.Packages[0].NewVersion)
		assert.Equal(t, "gomodifytags", got.Packages[1].Name)
		assert.Equal(t, "@latest", got.Packages[1].NewVersion)
	})
}

func TestListInstalledGoTools(t *testing.T) {
	t.Run("GOBIN配下のファイルを列挙する", func(t *testing.T) {
		dir := t.TempDir()

		require.NoError(t, os.WriteFile(filepath.Join(dir, "tool1"), []byte(""), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "tool2.exe"), []byte(""), 0o644))
		require.NoError(t, os.Mkdir(filepath.Join(dir, "somedir"), 0o755))

		t.Setenv("GOBIN", dir)

		got, err := ListInstalledGoTools()
		require.NoError(t, err)

		sort.Strings(got)
		assert.Equal(t, []string{"tool1", "tool2.exe"}, got)
	})

	t.Run("GOBINが存在しない場合はエラー", func(t *testing.T) {
		t.Setenv("GOBIN", filepath.Join(t.TempDir(), "missing"))

		_, err := ListInstalledGoTools()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "$GOBIN")
	})
}

func TestParseGoVersionOutput(t *testing.T) {
	tests := []struct {
		name            string
		output          string
		expectedModule  string
		expectedVersion string
	}{
		{
			name:           "path行のみ",
			output:         "go version go1.25.6 windows/amd64\npath\tgithub.com/example/tool\n",
			expectedModule: "github.com/example/tool",
		},
		{
			name:            "mod行を優先",
			output:          "path\tgithub.com/example/tool\nmod\tgithub.com/example/tool\tv1.2.3\th1:dummy\n",
			expectedModule:  "github.com/example/tool",
			expectedVersion: "v1.2.3",
		},
		{
			name:   "該当なし",
			output: "go version go1.25.6 windows/amd64\n",
		},
		{
			name:           "余分な空白を含む",
			output:         "   path    github.com/example/tool   \n",
			expectedModule: "github.com/example/tool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modulePath, version := ParseGoVersionOutput(tt.output)
			assert.Equal(t, tt.expectedModule, modulePath)
			assert.Equal(t, tt.expectedVersion, version)
		})
	}

	t.Run("複数行のmodとpathが混在してもパースできる", func(t *testing.T) {
		output := strings.Join([]string{
			"go version -m dummy",
			"path github.com/example/tool",
			"mod github.com/example/tool v9.9.9 h1:dummy",
		}, "\n")

		modulePath, version := ParseGoVersionOutput(output)
		assert.Equal(t, "github.com/example/tool", modulePath)
		assert.Equal(t, "v9.9.9", version)
	})
}
