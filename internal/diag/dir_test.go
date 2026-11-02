package diag

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultDirFor(t *testing.T) {
	absState := filepath.Join(t.TempDir(), "state")

	// 区切り文字を含む基準パスは、ホストの OS の表記に変換してから Join に渡す
	winLocal := `C:\Users\alice\AppData\Local`
	winHome := `C:\Users\alice`
	posixHome := filepath.FromSlash("/home/alice")
	macHome := filepath.FromSlash("/Users/alice")

	tests := []struct {
		name    string
		goos    string
		env     map[string]string
		home    string
		want    string
		wantErr bool
	}{
		{
			name: "Windows は LOCALAPPDATA",
			goos: "windows", env: map[string]string{"LOCALAPPDATA": winLocal}, home: winHome,
			want: filepath.Join(winLocal, "dsx", "diagnostics"),
		},
		{
			name: "Windows で LOCALAPPDATA が無ければホーム配下",
			goos: "windows", env: map[string]string{}, home: winHome,
			want: filepath.Join(winHome, "AppData", "Local", "dsx", "diagnostics"),
		},
		{
			name: "Windows でどちらも無ければエラー",
			goos: "windows", env: map[string]string{}, home: "", wantErr: true,
		},
		{
			name: "Linux は XDG_STATE_HOME（絶対パス）",
			goos: "linux", env: map[string]string{"XDG_STATE_HOME": absState}, home: posixHome,
			want: filepath.Join(absState, "dsx", "diagnostics"),
		},
		{
			name: "Linux で XDG_STATE_HOME が相対パスなら無視してホーム配下",
			goos: "linux", env: map[string]string{"XDG_STATE_HOME": "relative"}, home: posixHome,
			want: filepath.Join(posixHome, ".local", "state", "dsx", "diagnostics"),
		},
		{
			name: "Linux で XDG_STATE_HOME が無ければホーム配下",
			goos: "linux", env: map[string]string{}, home: posixHome,
			want: filepath.Join(posixHome, ".local", "state", "dsx", "diagnostics"),
		},
		{
			name: "macOS も XDG 規約に従う",
			goos: "darwin", env: map[string]string{}, home: macHome,
			want: filepath.Join(macHome, ".local", "state", "dsx", "diagnostics"),
		},
		{
			name: "Linux でどちらも無ければエラー",
			goos: "linux", env: map[string]string{}, home: "", wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := defaultDirFor(tt.goos, func(key string) string { return tt.env[key] }, tt.home)
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
