package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Save は設定を指定されたパスにYAML形式で保存します。
// パスが空の場合はデフォルトのパスを使用します。
func Save(cfg *Config, path string) error {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("ホームディレクトリの取得に失敗: %w", err)
		}

		path = filepath.Join(home, ".config", "dsx", "config.yaml")
	}

	// ディレクトリの作成
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("設定ディレクトリの作成に失敗: %w", err)
	}

	// YAMLへのマーシャル
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("設定のシリアライズに失敗: %w", err)
	}

	// ファイルへの書き込み
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("設定ファイルの書き込みに失敗: %w", err)
	}

	return nil
}

// SaveAtomic は既存ファイルをバックアップしてからアトミックに設定を保存します。
// 既存ファイルがある場合はタイムスタンプ付きバックアップを作成します。
// backupPath は作成したバックアップのパスを返します（既存ファイルがない場合は空文字列）。
func SaveAtomic(cfg *Config, path string) (string, error) {
	if path == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", fmt.Errorf("ホームディレクトリの取得に失敗: %w", homeErr)
		}

		path = filepath.Join(home, ".config", "dsx", "config.yaml")
	}

	// ディレクトリの作成
	dir := filepath.Dir(path)
	if mkdirErr := os.MkdirAll(dir, 0o755); mkdirErr != nil {
		return "", fmt.Errorf("設定ディレクトリの作成に失敗: %w", mkdirErr)
	}

	// マーシャルを先に行い、失敗時にバックアップが作成されないようにする
	data, marshalErr := yaml.Marshal(cfg)
	if marshalErr != nil {
		return "", fmt.Errorf("設定のシリアライズに失敗: %w", marshalErr)
	}

	// 既存ファイルのバックアップ（マーシャル成功後に実施）
	var backupPath string
	if info, statErr := os.Stat(path); statErr == nil {
		if info.IsDir() {
			return "", fmt.Errorf("設定ファイルのパスがディレクトリです: %s", path)
		}

		var backupErr error

		backupPath, backupErr = backupFile(path)
		if backupErr != nil {
			return "", fmt.Errorf("バックアップの作成に失敗: %w", backupErr)
		}
	}

	// アトミック書き込み
	if writeErr := writeFileAtomic(path, data, 0o644); writeErr != nil {
		return backupPath, writeErr
	}

	return backupPath, nil
}

// backupFile はファイルをナノ秒タイムスタンプ付きのバックアップパスにコピーします。
// ナノ秒精度を使用することで、同一秒内の複数呼び出しでもファイル名衝突を回避します。
func backupFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("バックアップ元の読み込みに失敗: %w", err)
	}

	timestamp := time.Now().Format("20060102-150405.000000000")
	backupPath := path + ".bak." + timestamp

	if err := os.WriteFile(backupPath, data, 0o644); err != nil { //nolint:gosec // G306: path + suffix で安全
		return "", fmt.Errorf("バックアップファイルの書き込みに失敗: %w", err)
	}

	return backupPath, nil
}

// writeFileAtomic は一時ファイルへの書き込み後にリネームしてアトミックに更新します。
// Go の os.Rename は Windows では MoveFileExW(MOVEFILE_REPLACE_EXISTING) を使用するため、
// 既存ファイルへの上書き置換が正しく動作します。
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"

	if err := os.WriteFile(tmp, data, perm); err != nil {
		return fmt.Errorf("一時ファイルの書き込みに失敗: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp) //nolint:errcheck // ベストエフォートでクリーンアップ
		return fmt.Errorf("ファイルのアトミック置換に失敗: %w", err)
	}

	return nil
}
