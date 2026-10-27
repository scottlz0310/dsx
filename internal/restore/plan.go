// Package restore はツールの導入を実行せず、復元計画を作成します。
package restore

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/scottlz0310/dsx/internal/snapshot"
)

// Command は表示専用の実行ファイル名と引数です。シェルとして評価しません。
type Command struct {
	Args    []string
	Version string
}

// Manager は読み取り専用の取得・照会処理を注入するための契約です。
type Manager struct {
	Available  func() bool
	Compatible func(string) bool
	Installed  func(context.Context) ([]snapshot.Tool, error)
	Resolve    func(context.Context, snapshot.Tool) (Command, error)
}

// State は復元項目の判定結果です。
type State string

const (
	// Existing は指定条件を満たす導入済みツールです。
	Existing State = "導入済み"
	// Add は提供版を確認した追加候補です。
	Add State = "追加候補"
	// Unresolved は自動的な復元計画を作成できなかった項目です。
	Unresolved State = "解決不能"
)

// Entry は1つのツールの状態と表示用コマンドを保持します。
type Entry struct {
	Manager string
	Tool    snapshot.Tool
	State   State
	Message string
	Command Command
}

// Plan は成功した項目と解決不能な項目を同じ一覧に保持します。
type Plan struct {
	Entries []Entry
}

// Build は manifest に含まれるマネージャだけを照会します。
func Build(ctx context.Context, manifest *snapshot.Manifest, managers map[string]Manager, goos string) (*Plan, error) {
	if _, err := snapshot.Marshal(manifest); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(manifest.Tools))
	for name := range manifest.Tools {
		names = append(names, name)
	}

	slices.Sort(names)

	plan := &Plan{}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return plan, err
		}

		tools, err := snapshot.Normalize(name, manifest.Tools[name])
		if err != nil {
			return nil, err
		}

		if len(tools) == 0 {
			continue
		}

		manager, ok := managers[name]
		problem := managerProblem(name, ok, manager, goos)

		entries, planErr := planManager(ctx, name, tools, manager, problem)

		plan.Entries = append(plan.Entries, entries...)
		if planErr != nil {
			return plan, planErr
		}
	}

	return plan, ctx.Err()
}

func planManager(ctx context.Context, name string, tools []snapshot.Tool, manager Manager, problem string) ([]Entry, error) {
	var installed []snapshot.Tool
	if problem == "" {
		var err error

		installed, err = manager.Installed(ctx)
		if err != nil {
			problem = fmt.Sprintf("インストール済み一覧の取得に失敗: %v", err)
		}
	}

	entries := make([]Entry, 0, len(tools))
	for _, tool := range tools {
		if err := ctx.Err(); err != nil {
			return entries, err
		}

		entry := Entry{Manager: name, Tool: tool, State: Unresolved, Message: problem}
		if problem == "" {
			entry = planTool(ctx, name, tool, tools, installed, manager)
		}

		entries = append(entries, entry)
	}

	return entries, ctx.Err()
}

func managerProblem(name string, supported bool, manager Manager, goos string) string {
	if !supported {
		return "復元計画に未対応のマネージャです"
	}

	if !manager.Compatible(goos) {
		return fmt.Sprintf("%s は OS %s で利用できません", name, goos)
	}

	if !manager.Available() {
		return "マネージャが未導入です（自動導入は行いません）"
	}

	return ""
}

func planTool(ctx context.Context, name string, tool snapshot.Tool, requested, installed []snapshot.Tool, manager Manager) Entry {
	entry := Entry{Manager: name, Tool: tool, State: Unresolved}
	if conflictingRequest(name, tool, requested) {
		entry.Message = "同じパッケージに異なる版または source が指定されています"
		return entry
	}

	for _, current := range installed {
		if !samePackage(name, current.Package, tool.Package) {
			continue
		}

		if (tool.Version == "" || sameVersion(name, tool.Version, current.Version)) && (tool.Source == "" || tool.Source == current.Source) {
			entry.State = Existing
			entry.Message = "指定条件を満たしています（変更なし）"
			return entry
		}
	}

	for _, current := range installed {
		if samePackage(name, current.Package, tool.Package) && name != "gem" {
			entry.Message = "既存の版または source と異なります（既存ツールの変更は行いません）"
			return entry
		}
	}

	command, err := manager.Resolve(ctx, tool)
	if err != nil {
		entry.Message = fmt.Sprintf("提供版・source の確認に失敗: %v", err)
		return entry
	}

	entry.State = Add
	entry.Command = command

	entry.Message = "提供版を確認しました（インストール・ビルドの成功は未検証）"
	if command.Version == "" {
		entry.Message = "パッケージと source を確認しました（版は未固定）"
	}

	return entry
}

func conflictingRequest(manager string, tool snapshot.Tool, requested []snapshot.Tool) bool {
	for _, other := range requested {
		if samePackage(manager, other.Package, tool.Package) && (other.Source != tool.Source || (manager != "gem" && other.Version != tool.Version)) {
			return true
		}
	}

	return false
}

func samePackage(manager, a, b string) bool {
	if manager == "pipx" || manager == "uv" {
		normalize := func(name string) string {
			return strings.Join(strings.FieldsFunc(strings.ToLower(name), func(c rune) bool { return c == '-' || c == '_' || c == '.' }), "-")
		}
		return normalize(a) == normalize(b)
	}

	if manager == "winget" || manager == "npm" || manager == "pnpm" || manager == "cargo" {
		return strings.EqualFold(a, b)
	}

	return a == b
}

func sameVersion(manager, want, got string) bool {
	if manager == "npm" || manager == "pnpm" || manager == "cargo" {
		return strings.TrimPrefix(want, "v") == strings.TrimPrefix(got, "v")
	}
	return want == got
}

// UnresolvedCount は計画を正常終了できない項目数を返します。
func (p *Plan) UnresolvedCount() int {
	count := 0
	for i := range p.Entries {
		if p.Entries[i].State == Unresolved {
			count++
		}
	}

	return count
}
