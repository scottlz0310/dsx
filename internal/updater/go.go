package updater

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/scottlz0310/dsx/internal/config"
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
			pkg += "@latest"
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

// GoBinaryInfo は "go version -m <binary>" の出力から取得したバイナリ情報を保持します。
type GoBinaryInfo struct {
	BinaryPath       string // バイナリの絶対パス
	BinaryName       string // filepath.Base(BinaryPath)、.exe を含む（trim しない）
	PackagePath      string // `path` 行 → go install に使う
	ModulePath       string // `mod` 行フィールド2（モジュールルート）
	InstalledVersion string // `mod` 行フィールド3
	GoVersion        string // ParseGoBinaryInfo では設定しない（将来タスクで対応予定）
}

// UpdateTarget は go install に渡すパスを返す（PackagePath + "@latest"）。
// フィールドではなくメソッドにすることで PackagePath との不整合を防ぐ。
// レシーバが nil の場合は空文字を返す。
func (i *GoBinaryInfo) UpdateTarget() string {
	if i == nil || i.PackagePath == "" {
		return ""
	}

	return i.PackagePath + "@latest"
}

// DiscoverResult は DiscoverGoBinaries / DiscoverGoBinariesInDir の結果を保持します。
type DiscoverResult struct {
	Detected []GoBinaryInfo
	Skipped  []SkippedBinary
}

// SkippedBinary はスキャン中にスキップされたバイナリの情報を保持します。
type SkippedBinary struct {
	Name   string
	Reason string // "Go モジュール情報なし" / "バックアップファイル"
}

// runGoVersionM は "go version -m <binaryPath>" を実行して出力を返します。
func runGoVersionM(ctx context.Context, binaryPath string) ([]byte, error) {
	return runCommandOutputWithLocaleC(ctx, "go", []string{"version", "-m", binaryPath}, "go version -m の実行に失敗: %w")
}

// discoverInDir は binDir 内のバイナリをスキャンし、DiscoverResult を返します。
// runCmd はテスト時に fakeRunCmd を注入できる関数パラメータです。
func discoverInDir(ctx context.Context, binDir string,
	runCmd func(context.Context, string) ([]byte, error)) (*DiscoverResult, error) {
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return nil, fmt.Errorf("%s の読み取りに失敗: %w", binDir, err)
	}

	result := &DiscoverResult{}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// *~ / *.exe~ パターン（バックアップファイル）を除外
		if strings.HasSuffix(name, "~") {
			result.Skipped = append(result.Skipped, SkippedBinary{
				Name:   name,
				Reason: "バックアップファイル",
			})

			continue
		}

		fullPath := filepath.Join(binDir, name)

		output, err := runCmd(ctx, fullPath)
		if err != nil {
			result.Skipped = append(result.Skipped, SkippedBinary{
				Name:   name,
				Reason: "Go モジュール情報なし",
			})

			continue
		}

		info, err := ParseGoBinaryInfo(fullPath, string(output))
		if err != nil {
			result.Skipped = append(result.Skipped, SkippedBinary{
				Name:   name,
				Reason: "Go モジュール情報なし",
			})

			continue
		}

		result.Detected = append(result.Detected, *info)
	}

	return result, nil
}

// DiscoverGoBinariesInDir は指定ディレクトリ内の Go バイナリ情報を収集します。
func DiscoverGoBinariesInDir(ctx context.Context, binDir string) (*DiscoverResult, error) {
	return discoverInDir(ctx, binDir, runGoVersionM)
}

// DiscoverGoBinaries は $GOBIN → $GOPATH/bin → ~/go/bin の順でパスを解決し、
// Go バイナリ情報を収集します。
func DiscoverGoBinaries(ctx context.Context) (*DiscoverResult, error) {
	binDir := os.Getenv("GOBIN")
	if binDir == "" {
		gopath := os.Getenv("GOPATH")
		if gopath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("ホームディレクトリの取得に失敗: %w", err)
			}

			gopath = filepath.Join(home, "go")
		}

		// GOPATH は OS によってはパスリスト（Unix では ":" 区切り、Windows では ";" 区切り）に
		// なり得るため、filepath.SplitList で先頭エントリを取得してから "bin" を連結する。
		gopathEntries := filepath.SplitList(gopath)
		binDir = filepath.Join(gopathEntries[0], "bin")
	}

	return DiscoverGoBinariesInDir(ctx, binDir)
}

// ParseGoBinaryInfo は "go version -m <binary>" の出力を解析し、GoBinaryInfo を返します。
// path 行が存在しない場合は nil と error を返します。
func ParseGoBinaryInfo(binaryPath, output string) (*GoBinaryInfo, error) {
	info := &GoBinaryInfo{
		BinaryPath: binaryPath,
		BinaryName: filepath.Base(binaryPath),
	}

	foundPath := false
	scanner := bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		fields := strings.Fields(line)

		if len(fields) >= 2 && fields[0] == "path" {
			info.PackagePath = fields[1]
			foundPath = true
		}

		if len(fields) >= 3 && fields[0] == "mod" {
			info.ModulePath = fields[1]
			info.InstalledVersion = fields[2]
		}
	}

	if !foundPath {
		return nil, fmt.Errorf("go version -m の出力に path 行が見つかりませんでした: %s", binaryPath)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("go version -m 出力のスキャン中にエラーが発生しました: %w", err)
	}

	return info, nil
}
