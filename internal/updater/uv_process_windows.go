//go:build windows

package updater

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// listRunningExecutables は dir にある name のいずれかの実行ファイルで動作中のプロセスを "name (PID n)" で返します。
// 実行ファイルのパスを取得できないプロセス（権限不足等）は、dir 内かを判定できないため、名前が一致すれば含めます。
func listRunningExecutables(dir string, names []string) ([]string, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("プロセス一覧を取得できません: %w", err)
	}
	defer windows.CloseHandle(snapshot) //nolint:errcheck // 読み取り専用のスナップショットで、閉じる失敗は結果に影響しない

	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}

	if err := windows.Process32First(snapshot, &entry); err != nil {
		return nil, fmt.Errorf("プロセス一覧を取得できません: %w", err)
	}

	var running []string

	for {
		name := windows.UTF16ToString(entry.ExeFile[:])
		if containsFold(names, name) && executableInDir(entry.ProcessID, dir) {
			running = append(running, fmt.Sprintf("%s (PID %d)", name, entry.ProcessID))
		}

		err := windows.Process32Next(snapshot, &entry)
		if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
			return running, nil
		}

		if err != nil {
			return nil, fmt.Errorf("プロセス一覧を取得できません: %w", err)
		}
	}
}

func containsFold(names []string, name string) bool {
	for _, n := range names {
		if strings.EqualFold(n, name) {
			return true
		}
	}

	return false
}

// executableInDir は pid の実行ファイルが dir にあるかを返します。パスを取得できないときは true（保守的）です。
func executableInDir(pid uint32, dir string) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return true
	}
	defer windows.CloseHandle(handle) //nolint:errcheck // 読み取り専用のハンドルで、閉じる失敗は結果に影響しない

	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(windows.MAX_LONG_PATH)

	if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &size); err != nil {
		return true
	}

	return strings.EqualFold(filepath.Clean(filepath.Dir(windows.UTF16ToString(buf[:size]))), filepath.Clean(dir))
}
