//go:build !windows

package msix

func isPackaged() (bool, error) {
	return false, nil
}
