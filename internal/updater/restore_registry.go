package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/scottlz0310/dsx/internal/restore"
	"github.com/scottlz0310/dsx/internal/snapshot"
)

func (r *restoreResolver) resolvePython(ctx context.Context, manager string, tool snapshot.Tool) (restore.Command, error) {
	address := metadataAddress("https://pypi.org/pypi/", tool.Package, tool.Version, "/json")

	data, found, err := r.getMetadata(ctx, address)
	if err != nil {
		return restore.Command{}, err
	}

	if !found {
		return restore.Command{}, fmt.Errorf("PyPI にパッケージまたは指定版がありません")
	}

	var result struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
		URLs []struct {
			Yanked bool `json:"yanked"`
		} `json:"urls"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return restore.Command{}, fmt.Errorf("PyPI の応答が不正です")
	}

	available := false
	for _, file := range result.URLs {
		available = available || !file.Yanked
	}

	if !available {
		return restore.Command{}, fmt.Errorf("PyPI に利用可能な配布ファイルがありません（撤回版は候補にしません）")
	}

	if err := checkResolvedVersion(manager, tool.Version, result.Info.Version); err != nil {
		return restore.Command{}, err
	}

	args := make([]string, 0, 6)

	args = append(args, manager)
	if manager == "uv" {
		args = append(args, "tool")
	}

	args = append(args, restoreInstallCommand, "--index-url", "https://pypi.org/simple", tool.Package+"=="+result.Info.Version)

	return restore.Command{Args: args, Version: result.Info.Version}, nil
}

func (r *restoreResolver) resolveCargo(ctx context.Context, tool snapshot.Tool) (restore.Command, error) {
	name := strings.ToLower(tool.Package)
	prefix := cargoIndexPrefix(name)

	data, found, err := r.getMetadata(ctx, "https://index.crates.io/"+prefix+"/"+name)
	if err != nil {
		return restore.Command{}, err
	}

	if !found {
		return restore.Command{}, fmt.Errorf("crates.io にパッケージがありません")
	}

	version, err := parseCargoRestoreVersions(data, tool.Version)
	if err != nil {
		return restore.Command{}, err
	}

	return restore.Command{Args: []string{restoreCargoManager, restoreInstallCommand, "--registry", "crates-io", "--version", "=" + version, tool.Package}, Version: version}, nil
}

func cargoIndexPrefix(name string) string {
	switch len(name) {
	case 1:
		return "1"
	case 2:
		return "2"
	case 3:
		return "3/" + name[:1]
	default:
		return name[:2] + "/" + name[2:4]
	}
}

func parseCargoRestoreVersions(data []byte, requested string) (string, error) {
	version := ""
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var record struct {
			Version string `json:"vers"`
			Yanked  bool   `json:"yanked"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil || record.Version == "" {
			return "", fmt.Errorf("crates.io の索引が不正です")
		}

		if record.Yanked {
			continue
		}

		if requested != "" {
			if record.Version == strings.TrimPrefix(requested, "v") {
				return record.Version, nil
			}
			continue
		}

		if strings.Contains(record.Version, "-") {
			continue
		}

		if version == "" || comparePnpmVersions(record.Version, version) > 0 {
			version = record.Version
		}
	}

	if version == "" {
		return "", fmt.Errorf("crates.io に利用可能な指定版または安定版がありません")
	}

	if err := checkResolvedVersion(restoreCargoManager, requested, version); err != nil {
		return "", err
	}

	return version, nil
}

func (r *restoreResolver) resolveGem(ctx context.Context, tool snapshot.Tool) (restore.Command, error) {
	version := tool.Version
	if version == "" {
		data, found, err := r.getMetadata(ctx, metadataAddress("https://rubygems.org/api/v1/versions/", tool.Package, "", "/latest.json"))
		if err != nil {
			return restore.Command{}, err
		}

		var latest struct {
			Version string `json:"version"`
		}
		if !found || json.Unmarshal(data, &latest) != nil || latest.Version == "" {
			return restore.Command{}, fmt.Errorf("RubyGems の安定版を取得できません")
		}

		version = latest.Version
	}

	data, found, err := r.getMetadata(ctx, "https://rubygems.org/api/v2/rubygems/"+url.PathEscape(tool.Package)+"/versions/"+url.PathEscape(version)+".json")
	if err != nil {
		return restore.Command{}, err
	}

	var result struct {
		Version string `json:"version"`
	}
	if !found || json.Unmarshal(data, &result) != nil {
		return restore.Command{}, fmt.Errorf("RubyGems にパッケージまたは指定版がありません")
	}

	if err := checkResolvedVersion("gem", tool.Version, result.Version); err != nil {
		return restore.Command{}, err
	}

	return restore.Command{Args: []string{"gem", restoreInstallCommand, tool.Package, "--version", result.Version, "--source", "https://rubygems.org"}, Version: result.Version}, nil
}
