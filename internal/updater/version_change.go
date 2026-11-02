package updater

import (
	"fmt"
	"sort"
)

// changedPackages は更新の前後の一覧を比べ、バージョンが変わったパッケージだけを返します。
// 更新前後の両方に存在し、どちらもバージョンが分かるものだけを対象にします。
// 新しく増えたパッケージや、一覧から消えたパッケージは、更新として数えません。
// 戻り値は名前順で、CurrentVersion は更新前、NewVersion は更新後です。
func changedPackages(before, after []PackageInfo) []PackageInfo {
	afterVersions := make(map[string]string, len(after))
	for _, pkg := range after {
		afterVersions[pkg.Name] = pkg.CurrentVersion
	}

	changed := make([]PackageInfo, 0, len(before))

	for _, pkg := range before {
		newVersion, ok := afterVersions[pkg.Name]
		if !ok || pkg.CurrentVersion == "" || newVersion == "" || pkg.CurrentVersion == newVersion {
			continue
		}

		changed = append(changed, PackageInfo{
			Name:           pkg.Name,
			CurrentVersion: pkg.CurrentVersion,
			NewVersion:     newVersion,
		})
	}

	sort.Slice(changed, func(i, j int) bool { return changed[i].Name < changed[j].Name })

	return changed
}

// versionChangeResult は、個別の更新結果を返さないマネージャ（uv tool / pipx）の更新後に、
// 更新前の一覧と更新後の一覧を比べた結果です。
type versionChangeResult struct {
	// Changed はバージョンが変わったパッケージです。
	Changed []PackageInfo
	// Suspect は、更新後の一覧を解釈できず、件数を確定できない場合の理由です。空なら疑いなし。
	Suspect string
}

// detectVersionChanges は、更新前の一覧 before と、更新後に取得し直した一覧 afterCheck を比べます。
// 更新後の一覧の取得に失敗した場合や、更新前にあったパッケージが 1 件も読み取れない場合は、
// 件数を作らず、判定不能の疑いとして返します（更新は実行済みのため、成功を失敗には変えません）。
func detectVersionChanges(manager string, before []PackageInfo, afterCheck *CheckResult, afterErr error) versionChangeResult {
	if afterErr != nil {
		return versionChangeResult{
			Suspect: fmt.Sprintf("%s の更新後の一覧を取得できず、更新件数を確定できません: %v", manager, afterErr),
		}
	}

	if len(before) > 0 && len(afterCheck.Packages) == 0 {
		return versionChangeResult{
			Suspect: fmt.Sprintf("%s の更新後の一覧からパッケージを 1 件も読み取れず、更新件数を確定できません", manager),
		}
	}

	return versionChangeResult{Changed: changedPackages(before, afterCheck.Packages)}
}
