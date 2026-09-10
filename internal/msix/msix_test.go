package msix

import (
	"strings"
	"testing"
)

// go test のプロセスは MSIX パッケージ外で動くため、Windows では実 API が
// APPMODEL_ERROR_NO_PACKAGE を返し、非 Windows では常に false になることを確認する。
func TestIsPackaged_パッケージ外では偽を返す(t *testing.T) {
	t.Parallel()

	got, err := IsPackaged()
	if err != nil {
		t.Fatalf("IsPackaged() error = %v, want nil", err)
	}

	if got {
		t.Fatal("IsPackaged() = true, want false")
	}
}

func TestInstallCommand(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		want string
	}{
		{name: "Release の最新 install.ps1 を取得する", want: "https://github.com/scottlz0310/dsx/releases/latest/download/install.ps1"},
		{name: "UTF-8 として明示的に復号する", want: "[Text.Encoding]::UTF8.GetString("},
		{name: "バイト列を得るため iwr を使う", want: "iwr -UseBasicParsing"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if !strings.Contains(InstallCommand, tc.want) {
				t.Fatalf("InstallCommand = %q, want contains %q", InstallCommand, tc.want)
			}
		})
	}
}
