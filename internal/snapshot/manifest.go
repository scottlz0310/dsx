// Package snapshot は手動編集可能なツール構成の保存形式を定義します。
package snapshot

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Tool は再構築の入力となるパッケージ情報です。パスや取得元 URL は保存しません。
type Tool struct {
	Package string `yaml:"package"`
	Version string `yaml:"version,omitempty"`
	Source  string `yaml:"source,omitempty"`
}

// Manifest はスキーマバージョンとマネージャ別のツール一覧を保持します。
type Manifest struct {
	Version int               `yaml:"version"`
	Tools   map[string][]Tool `yaml:"tools"`
}

var (
	packagePattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	goPackagePattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*(/[A-Za-z0-9][A-Za-z0-9._-]*)+$`)
	nodePackagePattern   = regexp.MustCompile(`^(@[A-Za-z0-9][A-Za-z0-9._-]*/)?[A-Za-z0-9][A-Za-z0-9._-]*$`)
	wingetPackagePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+_-]*$`)
	versionPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.!+_() -]*$`)
)

// Normalize は識別子を検証し、完全重複を除去して決定的な順序で返します。
func Normalize(manager string, tools []Tool) ([]Tool, error) {
	pattern := packagePattern
	switch manager {
	case "go":
		pattern = goPackagePattern
	case "npm", "pnpm":
		pattern = nodePackagePattern
	case "winget":
		pattern = wingetPackagePattern
	case "cargo", "gem", "pipx", "uv":
	default:
		return nil, fmt.Errorf("未対応のスナップショットマネージャです")
	}

	result := make([]Tool, 0, len(tools))
	for _, tool := range tools {
		if !pattern.MatchString(tool.Package) || strings.Contains(tool.Package, "..") {
			return nil, fmt.Errorf("%s: パッケージ識別子が不正です（URL・ローカルパスは保存できません）", manager)
		}

		if tool.Version != "" && !versionPattern.MatchString(tool.Version) {
			return nil, fmt.Errorf("%s: バージョンに保存できない文字が含まれています", manager)
		}

		if tool.Source != "" && (manager != "winget" || !packagePattern.MatchString(tool.Source)) {
			return nil, fmt.Errorf("%s: source が不正です（source 名のみ保存できます）", manager)
		}

		result = append(result, tool)
	}

	slices.SortFunc(result, func(a, b Tool) int {
		if n := strings.Compare(a.Package, b.Package); n != 0 {
			return n
		}

		if n := strings.Compare(a.Version, b.Version); n != 0 {
			return n
		}

		return strings.Compare(a.Source, b.Source)
	})

	return slices.Compact(result), nil
}

// Marshal は検証済みのマニフェストを安定した YAML に変換します。
func Marshal(manifest *Manifest) ([]byte, error) {
	if manifest.Version != 1 {
		return nil, fmt.Errorf("スナップショットのスキーマバージョンは 1 が必要です")
	}

	normalized := Manifest{Version: 1, Tools: make(map[string][]Tool, len(manifest.Tools))}
	for manager, tools := range manifest.Tools {
		items, err := Normalize(manager, tools)
		if err != nil {
			return nil, err
		}

		normalized.Tools[manager] = items
	}

	return yaml.Marshal(normalized)
}
