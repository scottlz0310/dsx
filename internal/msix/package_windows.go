//go:build windows

package msix

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// appmodelErrorNoPackage は GetCurrentPackageFullName がパッケージ外のプロセスで返す
// APPMODEL_ERROR_NO_PACKAGE です（golang.org/x/sys/windows には定義がない）。
const appmodelErrorNoPackage = windows.Errno(15700)

var procGetCurrentPackageFullName = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetCurrentPackageFullName")

func isPackaged() (bool, error) {
	// バッファ長 0 で呼ぶと、パッケージ実行時は ERROR_INSUFFICIENT_BUFFER、
	// パッケージ外では APPMODEL_ERROR_NO_PACKAGE が返る
	var length uint32

	r, _, _ := procGetCurrentPackageFullName.Call(uintptr(unsafe.Pointer(&length)), 0) //nolint:errcheck // 結果は戻り値で返り、GetLastError は設定されないため 3 番目の値は無意味

	switch errno := windows.Errno(r); errno {
	case windows.ERROR_INSUFFICIENT_BUFFER:
		return true, nil
	case appmodelErrorNoPackage:
		return false, nil
	default:
		return false, fmt.Errorf("MSIX パッケージとしての実行かどうかを判定できません（GetCurrentPackageFullName: %w）", errno)
	}
}
