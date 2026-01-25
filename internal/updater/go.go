package updater

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/scottlz0310/devsync/internal/config"
)

// GoUpdater は Go ツール (go install) の更新を管理します。
// go install コマンドでインストールしたバイナリを最新版に更新します。
type GoUpdater struct {
	// targets は更新対象のパッケージパス一覧
	// 例: ["golang.org/x/tools/gopls@latest", "github.com/golangci/golangci-lint/cmd/golangci-lint@latest"]
	targets []string
}

// 起動時にレジストリに登録
func init() {
	Register(&GoUpdater{})
}

func (g *GoUpdater) Name() string {
	return "go"
}

func (g *GoUpdater) DisplayName() string {
	return "Go ツール (go install)"
}

func (g *GoUpdater) IsAvailable() bool {
	_, err := exec.LookPath("go")
	return err == nil
}

func (g *GoUpdater) Configure(cfg config.ManagerConfig) error {
	if cfg == nil {
		return nil
	}

	// targets の設定を読み込む
	if targets, ok := cfg["targets"]; ok {
		switch v := targets.(type) {
		case []interface{}:
			g.targets = make([]string, 0, len(v))
			for _, item := range v {
				if s, ok := item.(string); ok {
					g.targets = append(g.targets, s)
				}
			}
		case []string:
			g.targets = v
		}
	}

	return nil
}

func (g *GoUpdater) Check(ctx context.Context) (*CheckResult, error) {
	// Go ツールは明示的なバージョン確認が難しいため、
	// 設定された targets 数を「更新可能」として返す
	if len(g.targets) == 0 {
		return &CheckResult{
			Message: "更新対象のGoツールが設定されていません",
		}, nil
	}

	packages := make([]PackageInfo, 0, len(g.targets))
	for _, target := range g.targets {
		// パッケージパスからツール名を抽出
		name := extractToolName(target)
		packages = append(packages, PackageInfo{
			Name:       name,
			NewVersion: "@latest",
		})
	}

	return &CheckResult{
		AvailableUpdates: len(packages),
		Packages:         packages,
		Message:          fmt.Sprintf("%d 件のGoツールが更新対象です", len(packages)),
	}, nil
}

func (g *GoUpdater) Update(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	result := &UpdateResult{}

	if len(g.targets) == 0 {
		result.Message = "更新対象のGoツールが設定されていません"
		return result, nil
	}

	if opts.DryRun {
		packages := make([]PackageInfo, 0, len(g.targets))
		for _, target := range g.targets {
			packages = append(packages, PackageInfo{
				Name:       extractToolName(target),
				NewVersion: "@latest",
			})
		}
		result.Packages = packages
		result.Message = fmt.Sprintf("%d 件のGoツールを更新予定（DryRunモード）", len(g.targets))
		return result, nil
	}

	// 各ツールを順番に更新
	for _, target := range g.targets {
		toolName := extractToolName(target)

		// @latest が付いていない場合は追加
		pkg := target
		if !strings.Contains(pkg, "@") {
			pkg = pkg + "@latest"
		}

		fmt.Printf("  📦 %s をインストール中...\n", toolName)

		cmd := exec.CommandContext(ctx, "go", "install", pkg)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = os.Environ()

		if err := cmd.Run(); err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, fmt.Errorf("%s: %w", toolName, err))
			continue
		}

		result.UpdatedCount++
		result.Packages = append(result.Packages, PackageInfo{
			Name:       toolName,
			NewVersion: "@latest",
		})
	}

	if result.FailedCount > 0 {
		result.Message = fmt.Sprintf("%d 件更新、%d 件失敗", result.UpdatedCount, result.FailedCount)
	} else {
		result.Message = fmt.Sprintf("%d 件のGoツールを更新しました", result.UpdatedCount)
	}

	return result, nil
}

// extractToolName はパッケージパスからツール名を抽出します
// 例: "github.com/golangci/golangci-lint/cmd/golangci-lint@latest" -> "golangci-lint"
func extractToolName(pkg string) string {
	// @version を除去
	if idx := strings.Index(pkg, "@"); idx != -1 {
		pkg = pkg[:idx]
	}

	// 最後のパスセグメントを取得
	parts := strings.Split(pkg, "/")
	return parts[len(parts)-1]
}

// DefaultGoTargets はよく使われるGoツールのデフォルトリストを返します。
// 設定ファイルで targets が未指定の場合の参考として使用できます。
func DefaultGoTargets() []string {
	return []string{
		"golang.org/x/tools/gopls@latest",
		"github.com/golangci/golangci-lint/cmd/golangci-lint@latest",
		"github.com/go-delve/delve/cmd/dlv@latest",
		"github.com/fatih/gomodifytags@latest",
		"github.com/cweill/gotests/gotests@latest",
		"github.com/josharian/impl@latest",
	}
}

// ListInstalledGoTools は $GOPATH/bin または $GOBIN にインストールされたツールを一覧表示します。
func ListInstalledGoTools() ([]string, error) {
	// GOBIN を優先、なければ GOPATH/bin
	gobin := os.Getenv("GOBIN")
	if gobin == "" {
		gopath := os.Getenv("GOPATH")
		if gopath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			gopath = home + "/go"
		}
		gobin = gopath + "/bin"
	}

	entries, err := os.ReadDir(gobin)
	if err != nil {
		return nil, fmt.Errorf("$GOBIN (%s) の読み取りに失敗: %w", gobin, err)
	}

	var tools []string
	for _, entry := range entries {
		if !entry.IsDir() {
			tools = append(tools, entry.Name())
		}
	}

	return tools, nil
}

// ParseGoVersionOutput は "go version -m <binary>" の出力からモジュールパスを取得します。
func ParseGoVersionOutput(output string) (modulePath string, version string) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "path") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				modulePath = parts[1]
			}
		}
		if strings.HasPrefix(line, "mod") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				modulePath = parts[1]
				version = parts[2]
			}
		}
	}
	return
}
