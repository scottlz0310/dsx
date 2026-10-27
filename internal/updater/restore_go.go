package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/scottlz0310/dsx/internal/restore"
	"github.com/scottlz0310/dsx/internal/snapshot"
)

func (r *restoreResolver) resolveGo(ctx context.Context, tool snapshot.Tool) (restore.Command, error) {
	version, err := r.findGoRestoreVersion(ctx, tool.Package, tool.Version)
	if err != nil {
		return restore.Command{}, err
	}

	if err := checkResolvedVersion("go", tool.Version, version); err != nil {
		return restore.Command{}, err
	}

	return restore.Command{Args: []string{"go", restoreInstallCommand, tool.Package + "@" + version}, Version: version}, nil
}

func (r *restoreResolver) findGoRestoreVersion(ctx context.Context, pkg, requested string) (string, error) {
	// Go のモジュール解決に合わせ、パッケージの最長の接頭辞から照会します。
	// 未登録（404/410）の場合だけ親へ進み、通信・認証・解析エラーは伝播させます。
	for module := pkg; strings.Contains(module, "/"); module = module[:strings.LastIndex(module, "/")] {
		suffix := "/@latest"
		if requested != "" {
			suffix = "/@v/" + escapeRestoreGoPath(requested) + ".info"
		}

		data, found, err := r.getMetadata(ctx, "https://proxy.golang.org/"+escapeRestoreGoPath(module)+suffix)
		if err != nil {
			return "", err
		}

		if !found {
			continue
		}

		return parseGoRestoreVersion(data)
	}

	return "", fmt.Errorf("公開 Go module proxy に対応するモジュールまたは指定版がありません")
}

func parseGoRestoreVersion(data []byte) (string, error) {
	var info struct{ Version string }
	if err := json.Unmarshal(data, &info); err != nil || info.Version == "" {
		return "", fmt.Errorf("go module proxy の版情報が不正です")
	}

	return info.Version, nil
}

func escapeRestoreGoPath(value string) string {
	var result strings.Builder
	for _, char := range value {
		if char >= 'A' && char <= 'Z' {
			result.WriteByte('!')
			result.WriteRune(char + 'a' - 'A')
		} else {
			result.WriteRune(char)
		}
	}

	return result.String()
}
