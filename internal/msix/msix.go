// Package msix は Windows 向け MSIX パッケージとしての配布・実行に関する情報を提供します。
package msix

// InstallCommand は MSIX 版 dsx の導入・更新を行う PowerShell ワンライナーです。
// Release アセットは application/octet-stream で配信され、Windows PowerShell 5.1 の irm では
// 日本語が文字化けするため、バイト列を UTF-8 として明示的に復号して実行します。
// scripts/install.ps1 は PowerShell 5.1 のファイル実行互換性のため UTF-8 with BOM で保存されているため、
// GetString で得られる先頭の BOM 文字（\uFEFF）を除去して iex に渡します。
const InstallCommand = `iex ([Text.Encoding]::UTF8.GetString((iwr -UseBasicParsing https://github.com/scottlz0310/dsx/releases/latest/download/install.ps1).Content).TrimStart([char]0xFEFF))`

// IsPackaged は現在のプロセスが MSIX パッケージとして実行されているかを返します。
// Windows 以外では常に false を返します。
func IsPackaged() (bool, error) {
	return isPackaged()
}
