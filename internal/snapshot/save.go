package snapshot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// CheckOutput は取得処理の前に保存先の誤指定と無許可の上書きを検出します。
func CheckOutput(path string, force bool) error {
	if path == "" {
		return fmt.Errorf("保存先を指定してください")
	}

	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("保存先の確認に失敗: %w", err)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("保存先は通常ファイルを指定してください: %s", path)
	}

	if !force {
		return fmt.Errorf("保存先が既に存在します。上書きには --force が必要です: %s", path)
	}

	return nil
}

// Save は取得・検証に成功したマニフェストだけを保存します。
func Save(path string, manifest *Manifest, force bool) (err error) {
	data, err := Marshal(manifest)
	if err != nil {
		return err
	}

	if checkErr := CheckOutput(path, force); checkErr != nil {
		return checkErr
	}

	if !force {
		return saveExclusive(path, data)
	}

	file, err := os.CreateTemp(filepath.Dir(path), ".dsx-snapshot-*")
	if err != nil {
		return fmt.Errorf("保存用一時ファイルの作成に失敗: %w", err)
	}

	temp := file.Name()
	defer func() {
		if removeErr := os.Remove(temp); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("一時ファイルの削除に失敗: %w", removeErr))
		}
	}()

	if writeErr := writeAndClose(file, data); writeErr != nil {
		return writeErr
	}

	if err = os.Rename(temp, path); err != nil {
		return fmt.Errorf("スナップショットの置き換えに失敗: %w", err)
	}

	return nil
}

func saveExclusive(path string, data []byte) (err error) {
	// O_EXCL により、事前確認の後に作られたファイルも上書きしません。
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("保存先の新規作成に失敗（既存ファイルの上書きには --force が必要です）: %w", err)
	}

	if err = writeAndClose(file, data); err != nil {
		if removeErr := os.Remove(path); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("書き込みに失敗したファイルの削除に失敗: %w", removeErr))
		}
	}

	return err
}

func writeAndClose(file *os.File, data []byte) error {
	_, writeErr := file.Write(data)

	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return fmt.Errorf("スナップショットの書き込みに失敗: %w", err)
	}

	return nil
}
