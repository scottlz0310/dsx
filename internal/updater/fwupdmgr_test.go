package updater

import (
	"errors"
	"testing"

	"github.com/scottlz0310/devsync/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestFwupdmgrUpdater_Name(t *testing.T) {
	f := &FwupdmgrUpdater{}
	assert.Equal(t, "fwupdmgr", f.Name())
}

func TestFwupdmgrUpdater_DisplayName(t *testing.T) {
	f := &FwupdmgrUpdater{}
	assert.Equal(t, "fwupdmgr (Linux Firmware)", f.DisplayName())
}

func TestFwupdmgrUpdater_Configure(t *testing.T) {
	f := &FwupdmgrUpdater{}
	err := f.Configure(config.ManagerConfig{"dummy": true})
	assert.NoError(t, err)
}

func TestFwupdmgrUpdater_parseGetUpdatesJSON(t *testing.T) {
	testCases := []struct {
		name        string
		output      []byte
		want        []PackageInfo
		expectErr   bool
		errContains string
	}{
		{
			name:        "空出力はエラー",
			output:      nil,
			expectErr:   true,
			errContains: "出力が空",
		},
		{
			name:        "不正なJSONはエラー",
			output:      []byte("{invalid"),
			expectErr:   true,
			errContains: "JSON の解析に失敗",
		},
		{
			name:        "devicesキーがない場合はエラー",
			output:      []byte(`{"status":"ok"}`),
			expectErr:   true,
			errContains: "devices キーが見つかりません",
		},
		{
			name:        "devicesの型が不正な場合はエラー",
			output:      []byte(`{"devices":{"name":"not-array"}}`),
			expectErr:   true,
			errContains: "devices の型が不正",
		},
		{
			name: "更新対象1件",
			output: []byte(`{
  "Devices": [
    {
      "Name": "USB-C Dock",
      "CurrentVersion": "1.0.0",
      "Releases": [
        {"Version": "1.1.0"}
      ]
    }
  ]
}`),
			want: []PackageInfo{
				{
					Name:           "USB-C Dock",
					CurrentVersion: "1.0.0",
					NewVersion:     "1.1.0",
				},
			},
		},
		{
			name: "無効データはスキップ",
			output: []byte(`{
  "devices": [
    {"name":"NoRelease", "releases":[]},
    {"releases":[{"version":"2.0.0"}]},
    {"name":"NoVersion", "releases":[{}]}
  ]
}`),
			want: []PackageInfo{},
		},
		{
			name: "nameが空でもguidにフォールバック",
			output: []byte(`{
  "devices": [
    {
      "guid": "abcd-efgh",
      "version": "1.0",
      "releases": [{"version":"1.1"}]
    }
  ]
}`),
			want: []PackageInfo{
				{
					Name:           "abcd-efgh",
					CurrentVersion: "1.0",
					NewVersion:     "1.1",
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := &FwupdmgrUpdater{}
			got, err := f.parseGetUpdatesJSON(tc.output)

			if tc.expectErr {
				assert.Error(t, err)

				if tc.errContains != "" {
					assert.Contains(t, err.Error(), tc.errContains)
				}

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestIsNoFwupdmgrUpdatesMessage(t *testing.T) {
	testCases := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "no updatable devices",
			output: "No updatable devices",
			want:   true,
		},
		{
			name:   "no updates available",
			output: "There are no updates available",
			want:   true,
		},
		{
			name:   "no upgrades for",
			output: "No upgrades for device",
			want:   true,
		},
		{
			name:   "それ以外はfalse",
			output: "device update failed",
			want:   false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := isNoFwupdmgrUpdatesMessage(tc.output)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBuildCommandOutputErr(t *testing.T) {
	baseErr := errors.New("base error")

	t.Run("出力なしは元エラーを返す", func(t *testing.T) {
		t.Parallel()

		got := buildCommandOutputErr(baseErr, nil)
		assert.ErrorIs(t, got, baseErr)
		assert.Equal(t, "base error", got.Error())
	})

	t.Run("出力ありはメッセージを連結", func(t *testing.T) {
		t.Parallel()

		got := buildCommandOutputErr(baseErr, []byte("details"))
		assert.ErrorIs(t, got, baseErr)
		assert.Contains(t, got.Error(), "details")
	})
}

func TestCombineCommandOutputs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		stdout []byte
		stderr []byte
		want   []byte
	}{
		{
			name:   "stdout/stderrとも空",
			stdout: nil,
			stderr: nil,
			want:   nil,
		},
		{
			name:   "stderrのみ",
			stdout: nil,
			stderr: []byte("error details\n"),
			want:   []byte("error details"),
		},
		{
			name:   "stdoutのみ",
			stdout: []byte("json output\n"),
			stderr: nil,
			want:   []byte("json output"),
		},
		{
			name:   "stdout/stderr両方",
			stdout: []byte("json output\n"),
			stderr: []byte("warning message\n"),
			want:   []byte("json output\nwarning message"),
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := combineCommandOutputs(tc.stdout, tc.stderr)
			assert.Equal(t, tc.want, got)
		})
	}
}
