// Package diag は、解析対象コマンドの生出力と環境情報を、ローカルの診断ログとして保存します。
//
// 目的は、パッケージマネージャの出力を誤って解釈したときに、後から原因（実際の出力・ロケール・
// 色の有無など）を追跡できるようにすることです。既定では無効で、設定で有効にしたときだけ保存します。
// 外部へは一切送信せず、保存前に、ホームのパス・ユーザー名・ホスト名・トークンらしい文字列を伏せ字にします。
package diag

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

const (
	dirName     = "dsx"
	diagDirName = "diagnostics"
)

// DefaultDir は診断ログの既定の保存先を返します。
//
//   - Windows: %LOCALAPPDATA%\dsx\diagnostics
//   - それ以外: $XDG_STATE_HOME/dsx/diagnostics（未設定なら ~/.local/state/dsx/diagnostics）
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	return defaultDirFor(runtime.GOOS, os.Getenv, home)
}

func defaultDirFor(goos string, getenv func(string) string, home string) (string, error) {
	if goos == "windows" {
		if base := getenv("LOCALAPPDATA"); base != "" {
			return filepath.Join(base, dirName, diagDirName), nil
		}

		if home == "" {
			return "", errors.New("保存先を決められません: LOCALAPPDATA もホームディレクトリも取得できません")
		}

		return filepath.Join(home, "AppData", "Local", dirName, diagDirName), nil
	}

	if state := getenv("XDG_STATE_HOME"); state != "" && filepath.IsAbs(state) {
		return filepath.Join(state, dirName, diagDirName), nil
	}

	if home == "" {
		return "", errors.New("保存先を決められません: XDG_STATE_HOME もホームディレクトリも取得できません")
	}

	return filepath.Join(home, ".local", "state", dirName, diagDirName), nil
}
