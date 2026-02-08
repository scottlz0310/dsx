package updater

import (
	"testing"

	"github.com/scottlz0310/devsync/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestSnapUpdater_Name(t *testing.T) {
	snap := &SnapUpdater{}
	assert.Equal(t, "snap", snap.Name())
}

func TestSnapUpdater_DisplayName(t *testing.T) {
	snap := &SnapUpdater{}
	assert.Equal(t, "snap (Ubuntu Snap パッケージ)", snap.DisplayName())
}

func TestSnapUpdater_Configure(t *testing.T) {
	testCases := []struct {
		name        string
		cfg         config.ManagerConfig
		expectSudo  bool
		description string
	}{
		{
			name:        "nilの設定",
			cfg:         nil,
			expectSudo:  false,
			description: "nil設定の場合はデフォルトのまま",
		},
		{
			name:        "空の設定",
			cfg:         config.ManagerConfig{},
			expectSudo:  false,
			description: "空の設定の場合はデフォルトのまま",
		},
		{
			name:        "use_sudo=true",
			cfg:         config.ManagerConfig{"use_sudo": true},
			expectSudo:  true,
			description: "use_sudoがtrueの場合はsudoを使用",
		},
		{
			name:        "use_sudo=false",
			cfg:         config.ManagerConfig{"use_sudo": false},
			expectSudo:  false,
			description: "use_sudoがfalseの場合はsudoを使用しない",
		},
		{
			name:        "旧キーsudo=true",
			cfg:         config.ManagerConfig{"sudo": true},
			expectSudo:  true,
			description: "旧キーsudoも後方互換で受け付ける",
		},
		{
			name:        "use_sudoを優先",
			cfg:         config.ManagerConfig{"use_sudo": false, "sudo": true},
			expectSudo:  false,
			description: "新キーと旧キーが両方ある場合はuse_sudoを優先する",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			snap := &SnapUpdater{useSudo: false}
			err := snap.Configure(tc.cfg)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectSudo, snap.useSudo, tc.description)
		})
	}
}

func TestIsSnapdUnavailable(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name: "snapd unavailable",
			output: `snap    2.73+ubuntu25.10
snapd   unavailable
series  -`,
			want: true,
		},
		{
			name: "snapd available",
			output: `snap    2.73+ubuntu25.10
snapd   2.73+ubuntu25.10
series  16`,
			want: false,
		},
		{
			name:   "大文字小文字の揺れを吸収",
			output: "SNAPD UNAVAILABLE",
			want:   true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := isSnapdUnavailable(tc.output)
			if got != tc.want {
				t.Fatalf("isSnapdUnavailable() = %v, want %v", got, tc.want)
			}
		})
	}
}
