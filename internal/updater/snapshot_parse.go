package updater

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/scottlz0310/dsx/internal/snapshot"
)

type nodeSnapshotProject struct {
	Dependencies map[string]struct {
		Version string `json:"version"`
	} `json:"dependencies"`
	Error json.RawMessage `json:"error"`
}

func parseNpmSnapshot(output []byte) ([]snapshot.Tool, error) {
	var project *nodeSnapshotProject
	if err := json.Unmarshal(output, &project); err != nil || project == nil {
		return nil, fmt.Errorf("npm list の JSON 形式が不正です")
	}

	return nodeSnapshotTools([]nodeSnapshotProject{*project})
}

func parsePnpmSnapshot(output []byte) ([]snapshot.Tool, error) {
	var projects []*nodeSnapshotProject
	if err := json.Unmarshal(output, &projects); err != nil || projects == nil {
		return nil, fmt.Errorf("pnpm list の JSON 配列が不正です")
	}

	items := make([]nodeSnapshotProject, 0, len(projects))
	for _, project := range projects {
		if project == nil {
			return nil, fmt.Errorf("pnpm list の配列に null が含まれています")
		}

		items = append(items, *project)
	}

	return nodeSnapshotTools(items)
}

func nodeSnapshotTools(projects []nodeSnapshotProject) ([]snapshot.Tool, error) {
	var tools []snapshot.Tool
	for _, project := range projects {
		if len(project.Error) != 0 {
			return nil, fmt.Errorf("一覧取得結果にエラーが含まれています")
		}

		for name, dependency := range project.Dependencies {
			if dependency.Version == "" {
				return nil, fmt.Errorf("パッケージのバージョンがありません（リンク先のみの項目は保存できません）")
			}

			tools = append(tools, snapshot.Tool{Package: name, Version: dependency.Version})
		}
	}

	return tools, nil
}

func parsePipxSnapshot(output []byte) ([]snapshot.Tool, error) {
	var list struct {
		Venvs map[string]struct {
			Metadata struct {
				MainPackage struct {
					Package string `json:"package"`
					Version string `json:"package_version"`
				} `json:"main_package"`
			} `json:"metadata"`
		} `json:"venvs"`
	}
	if err := json.Unmarshal(output, &list); err != nil || list.Venvs == nil {
		return nil, fmt.Errorf("pipx list の venvs が不正または存在しません")
	}

	tools := make([]snapshot.Tool, 0, len(list.Venvs))
	for _, venv := range list.Venvs {
		pkg := venv.Metadata.MainPackage
		if pkg.Package == "" || pkg.Version == "" {
			return nil, fmt.Errorf("pipx の主パッケージ情報が不足しています")
		}

		tools = append(tools, snapshot.Tool{Package: pkg.Package, Version: pkg.Version})
	}

	return tools, nil
}

func parseWingetSnapshot(output []byte) ([]snapshot.Tool, error) {
	var list struct {
		Sources []struct {
			SourceDetails struct {
				Name string `json:"Name"`
			} `json:"SourceDetails"`
			Packages []struct {
				ID      string `json:"PackageIdentifier"`
				Version string `json:"Version"`
			} `json:"Packages"`
		} `json:"Sources"`
	}

	output = bytes.TrimPrefix(output, []byte{0xef, 0xbb, 0xbf})
	if err := json.Unmarshal(output, &list); err != nil || list.Sources == nil {
		return nil, fmt.Errorf("winget export の Sources が不正または存在しません")
	}

	var tools []snapshot.Tool
	for _, source := range list.Sources {
		if source.SourceDetails.Name == "" || source.Packages == nil {
			return nil, fmt.Errorf("winget export の source 情報が不足しています")
		}

		for _, pkg := range source.Packages {
			if pkg.ID == "" {
				return nil, fmt.Errorf("winget の Package ID がありません")
			}

			version := pkg.Version
			// winget の概算版は正確なインストール版として固定できません。
			if strings.HasPrefix(version, ">") || strings.HasPrefix(version, "<") || strings.EqualFold(version, "unknown") || version == "不明" {
				version = ""
			}

			tools = append(tools, snapshot.Tool{Package: pkg.ID, Version: version, Source: source.SourceDetails.Name})
		}
	}

	return tools, nil
}

func parseUVSnapshot(output []byte) ([]snapshot.Tool, error) {
	var tools []snapshot.Tool
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "- ") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasPrefix(fields[1], "v") || len(fields[1]) == 1 {
			return nil, fmt.Errorf("uv tool list に不正なパッケージ行があります")
		}

		tools = append(tools, snapshot.Tool{Package: fields[0], Version: strings.TrimPrefix(fields[1], "v")})
	}

	return tools, nil
}

func parseCargoSnapshot(output []byte) ([]snapshot.Tool, error) {
	var tools []snapshot.Tool
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}

		fields := strings.Fields(strings.TrimSuffix(line, ":"))
		if !strings.HasSuffix(line, ":") || len(fields) < 2 || !strings.HasPrefix(fields[1], "v") || len(fields[1]) == 1 {
			return nil, fmt.Errorf("cargo install --list に不正なパッケージ行があります")
		}

		tools = append(tools, snapshot.Tool{Package: fields[0], Version: strings.TrimPrefix(fields[1], "v")})
	}

	return tools, nil
}

func parseGemSnapshot(output []byte) ([]snapshot.Tool, error) {
	var tools []snapshot.Tool
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "*** LOCAL GEMS ***" {
			continue
		}

		open := strings.Index(line, " (")
		if open < 1 || !strings.HasSuffix(line, ")") {
			return nil, fmt.Errorf("gem list に不正なパッケージ行があります")
		}

		for _, entry := range strings.Split(line[open+2:len(line)-1], ",") {
			version := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(entry), "default:"))

			fields := strings.Fields(version)
			if len(fields) == 0 {
				return nil, fmt.Errorf("gem パッケージのバージョンがありません")
			}

			tools = append(tools, snapshot.Tool{Package: line[:open], Version: fields[0]})
		}
	}

	return tools, nil
}
