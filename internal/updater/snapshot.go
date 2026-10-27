package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/scottlz0310/dsx/internal/snapshot"
)

// SnapshotProvider は構成の保存に対応するマネージャだけが実装します。
type SnapshotProvider interface {
	Snapshot(context.Context) ([]snapshot.Tool, error)
}

type snapshotCommandRunner func(context.Context, string, ...string) ([]byte, error)

func runSnapshotCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	cmd.Env = append(os.Environ(), "LANG=C", "LC_ALL=C", "NO_COLOR=1")
	output, err := cmd.Output()

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	// 生の出力には認証情報やローカルパスが含まれる可能性があるため表示しません。
	if err != nil {
		return nil, fmt.Errorf("%s の一覧取得コマンドが失敗しました: %w", name, err)
	}

	return output, nil
}

func snapshotFromCommand(ctx context.Context, run snapshotCommandRunner, name string, args []string,
	parse func([]byte) ([]snapshot.Tool, error)) ([]snapshot.Tool, error) {
	if run == nil {
		run = runSnapshotCommand
	}

	output, err := run(ctx, name, args...)
	if err != nil {
		return nil, err
	}

	tools, err := parse(output)
	if err != nil {
		return nil, fmt.Errorf("一覧の解析に失敗: %w", err)
	}

	return snapshot.Normalize(name, tools)
}

func (n *NpmUpdater) Snapshot(ctx context.Context) ([]snapshot.Tool, error) {
	return snapshotFromCommand(ctx, n.snapshotRun, "npm", []string{"list", "--global", "--depth=0", "--json"}, parseNpmSnapshot)
}

func (p *PnpmUpdater) Snapshot(ctx context.Context) ([]snapshot.Tool, error) {
	return snapshotFromCommand(ctx, p.snapshotRun, "pnpm", []string{"list", "--global", "--depth=0", "--json"}, parsePnpmSnapshot)
}

func (p *PipxUpdater) Snapshot(ctx context.Context) ([]snapshot.Tool, error) {
	return snapshotFromCommand(ctx, p.snapshotRun, "pipx", []string{"list", "--json"}, parsePipxSnapshot)
}

func (u *UVUpdater) Snapshot(ctx context.Context) ([]snapshot.Tool, error) {
	return snapshotFromCommand(ctx, u.snapshotRun, "uv", []string{"tool", "list", "--color", "never"}, parseUVSnapshot)
}

func (c *CargoUpdater) Snapshot(ctx context.Context) ([]snapshot.Tool, error) {
	return snapshotFromCommand(ctx, c.snapshotRun, "cargo", []string{"install", "--list"}, parseCargoSnapshot)
}

func (g *GemUpdater) Snapshot(ctx context.Context) ([]snapshot.Tool, error) {
	return snapshotFromCommand(ctx, g.snapshotRun, "gem", []string{"list", "--local", "--all", "--no-verbose"}, parseGemSnapshot)
}

func (g *GoUpdater) Snapshot(ctx context.Context) ([]snapshot.Tool, error) {
	discover := g.discoverGoBinaries
	if discover == nil {
		discover = DiscoverGoBinaries
	}

	result, err := discover(ctx)
	if errors.Is(err, os.ErrNotExist) {
		return []snapshot.Tool{}, nil
	}

	if err != nil {
		return nil, err
	}

	tools := make([]snapshot.Tool, 0, len(result.Detected))
	for _, info := range result.Detected {
		version := info.InstalledVersion
		if version == "(devel)" {
			version = ""
		}

		tools = append(tools, snapshot.Tool{Package: info.PackagePath, Version: version})
	}

	return snapshot.Normalize("go", tools)
}

func (w *WingetUpdater) Snapshot(ctx context.Context) (tools []snapshot.Tool, err error) {
	dir, err := os.MkdirTemp("", "dsx-winget-snapshot-*")
	if err != nil {
		return nil, fmt.Errorf("winget export 用ディレクトリの作成に失敗: %w", err)
	}
	defer func() {
		if removeErr := os.RemoveAll(dir); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("winget export 一時データの削除に失敗: %w", removeErr))
		}
	}()

	path := filepath.Join(dir, "packages.json")

	run := w.snapshotRun
	if run == nil {
		run = runSnapshotCommand
	}

	if _, err = run(ctx, "winget", "export", "--output", path, "--include-versions", "--disable-interactivity", "--accept-source-agreements"); err != nil {
		return nil, err
	}

	output, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("winget export の結果の読み取りに失敗: %w", err)
	}

	tools, err = parseWingetSnapshot(output)
	if err != nil {
		return nil, err
	}

	return snapshot.Normalize("winget", tools)
}

// SnapshotStatus は未対応・利用不可・取得失敗を区別する表示用の結果です。
type SnapshotStatus struct {
	Manager string
	Message string
}

// CollectSnapshot は sys.enable に依存せず、利用可能な対応マネージャを収集します。
// 一部の取得が失敗した場合はマニフェストを返さず、部分保存を防ぎます。
func CollectSnapshot(ctx context.Context, managers []Updater) (*snapshot.Manifest, []SnapshotStatus, error) {
	managers = slices.Clone(managers)
	slices.SortFunc(managers, func(a, b Updater) int { return strings.Compare(a.Name(), b.Name()) })

	manifest := &snapshot.Manifest{Version: 1, Tools: map[string][]snapshot.Tool{}}
	statuses := make([]SnapshotStatus, 0, len(managers))

	var failures []error
	for _, manager := range managers {
		if err := ctx.Err(); err != nil {
			failures = append(failures, err)
			break
		}

		provider, supported := manager.(SnapshotProvider)

		message := "スナップショット未対応"
		switch {
		case !supported:
		case !manager.IsAvailable():
			message = "利用不可（未導入または対象 OS 外）"
		default:
			tools, err := provider.Snapshot(ctx)
			if err == nil {
				tools, err = snapshot.Normalize(manager.Name(), tools)
			}

			if err != nil {
				message = "取得失敗"

				failures = append(failures, fmt.Errorf("%s のスナップショット取得に失敗: %w", manager.Name(), err))
			} else {
				manifest.Tools[manager.Name()] = tools

				message = fmt.Sprintf("%d 件を取得", len(tools))
				if unpinned := countUnpinnedSnapshotTools(tools); unpinned > 0 {
					message += fmt.Sprintf("（%d 件は正確な版が不明なため version を省略）", unpinned)
				}
			}
		}

		statuses = append(statuses, SnapshotStatus{Manager: manager.Name(), Message: message})
	}

	if err := errors.Join(failures...); err != nil {
		return nil, statuses, err
	}

	if len(manifest.Tools) == 0 {
		return nil, statuses, fmt.Errorf("利用可能なスナップショット対応マネージャがありません")
	}

	return manifest, statuses, nil
}

func countUnpinnedSnapshotTools(tools []snapshot.Tool) int {
	count := 0
	for _, tool := range tools {
		if tool.Version == "" {
			count++
		}
	}

	return count
}
