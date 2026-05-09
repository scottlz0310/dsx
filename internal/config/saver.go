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
			return fmt.Errorf("failed to get home directory: %w", err)
		}

		path = filepath.Join(home, ".config", "dsx", "config.yaml")
	}

	// ディレクトリの作成
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// YAMLへのマーシャル
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// ファイルへの書き込み
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
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
			return "", fmt.Errorf("failed to get home directory: %w", homeErr)
		}

		path = filepath.Join(home, ".config", "dsx", "config.yaml")
	}

	// ディレクトリの作成
	dir := filepath.Dir(path)
	if mkdirErr := os.MkdirAll(dir, 0o755); mkdirErr != nil {
		return "", fmt.Errorf("failed to create config directory: %w", mkdirErr)
	}

	// 既存ファイルのバックアップ
	var backupPath string
	if _, statErr := os.Stat(path); statErr == nil {
		var backupErr error

		backupPath, backupErr = backupFile(path)
		if backupErr != nil {
			return "", fmt.Errorf("バックアップの作成に失敗: %w", backupErr)
		}
	}

	// YAMLへのマーシャル
	data, marshalErr := yaml.Marshal(cfg)
	if marshalErr != nil {
		return backupPath, fmt.Errorf("failed to marshal config: %w", marshalErr)
	}

	// アトミック書き込み
	if writeErr := writeFileAtomic(path, data, 0o644); writeErr != nil {
		return backupPath, writeErr
	}

	return backupPath, nil
}

// backupFile はファイルをタイムスタンプ付きのバックアップパスにコピーします。
func backupFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("バックアップ元の読み込みに失敗: %w", err)
	}

	timestamp := time.Now().Format("20060102-150405")
	backupPath := path + ".bak." + timestamp

	if err := os.WriteFile(backupPath, data, 0o644); err != nil { //nolint:gosec // G306: path + suffix で安全
		return "", fmt.Errorf("バックアップファイルの書き込みに失敗: %w", err)
	}

	return backupPath, nil
}

// writeFileAtomic は一時ファイルへの書き込み後にリネームしてアトミックに更新します。
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
