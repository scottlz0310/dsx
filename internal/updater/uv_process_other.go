//go:build !windows

package updater

// listRunningExecutables は Windows 以外では常に空を返します。
// Unix 系は実行中の実行ファイルを置き換えられるため、実行中でも更新を妨げません。
func listRunningExecutables(string, []string) ([]string, error) {
	return nil, nil
}
