package updater

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChangedPackages(t *testing.T) {
	testCases := []struct {
		name   string
		before []PackageInfo
		after  []PackageInfo
		want   []PackageInfo
	}{
		{
			name:   "どちらも空",
			before: nil,
			after:  nil,
			want:   []PackageInfo{},
		},
		{
			name:   "版が変わらなければ空",
			before: []PackageInfo{{Name: "ruff", CurrentVersion: "0.6.2"}},
			after:  []PackageInfo{{Name: "ruff", CurrentVersion: "0.6.2"}},
			want:   []PackageInfo{},
		},
		{
			name:   "版が変わったものを更新前→更新後で返す",
			before: []PackageInfo{{Name: "ruff", CurrentVersion: "0.6.2"}},
			after:  []PackageInfo{{Name: "ruff", CurrentVersion: "0.6.3"}},
			want:   []PackageInfo{{Name: "ruff", CurrentVersion: "0.6.2", NewVersion: "0.6.3"}},
		},
		{
			name:   "更新後に増えたパッケージは数えない",
			before: []PackageInfo{{Name: "ruff", CurrentVersion: "0.6.2"}},
			after: []PackageInfo{
				{Name: "ruff", CurrentVersion: "0.6.2"},
				{Name: "newtool", CurrentVersion: "1.0.0"},
			},
			want: []PackageInfo{},
		},
		{
			name: "更新後に一覧から消えたパッケージは数えない",
			before: []PackageInfo{
				{Name: "ruff", CurrentVersion: "0.6.2"},
				{Name: "gone", CurrentVersion: "1.0.0"},
			},
			after: []PackageInfo{{Name: "ruff", CurrentVersion: "0.6.2"}},
			want:  []PackageInfo{},
		},
		{
			name:   "更新前の版が分からないものは数えない",
			before: []PackageInfo{{Name: "ruff"}},
			after:  []PackageInfo{{Name: "ruff", CurrentVersion: "0.6.3"}},
			want:   []PackageInfo{},
		},
		{
			name:   "更新後の版が分からないものは数えない",
			before: []PackageInfo{{Name: "ruff", CurrentVersion: "0.6.2"}},
			after:  []PackageInfo{{Name: "ruff"}},
			want:   []PackageInfo{},
		},
		{
			name: "複数あるときは名前順で返す",
			before: []PackageInfo{
				{Name: "ruff", CurrentVersion: "0.6.2"},
				{Name: "black", CurrentVersion: "24.1.0"},
				{Name: "same", CurrentVersion: "1.0.0"},
			},
			after: []PackageInfo{
				{Name: "same", CurrentVersion: "1.0.0"},
				{Name: "black", CurrentVersion: "24.2.0"},
				{Name: "ruff", CurrentVersion: "0.6.3"},
			},
			want: []PackageInfo{
				{Name: "black", CurrentVersion: "24.1.0", NewVersion: "24.2.0"},
				{Name: "ruff", CurrentVersion: "0.6.2", NewVersion: "0.6.3"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, changedPackages(tc.before, tc.after))
		})
	}
}

func TestDetectVersionChanges(t *testing.T) {
	before := []PackageInfo{{Name: "ruff", CurrentVersion: "0.6.2"}}

	testCases := []struct {
		name        string
		before      []PackageInfo
		afterCheck  *CheckResult
		afterErr    error
		wantChanged []PackageInfo
		wantSuspect string
	}{
		{
			name:        "版が変われば更新として返す",
			before:      before,
			afterCheck:  &CheckResult{Packages: []PackageInfo{{Name: "ruff", CurrentVersion: "0.6.3"}}},
			wantChanged: []PackageInfo{{Name: "ruff", CurrentVersion: "0.6.2", NewVersion: "0.6.3"}},
		},
		{
			name:        "版が変わらなければ疑いなしで空",
			before:      before,
			afterCheck:  &CheckResult{Packages: []PackageInfo{{Name: "ruff", CurrentVersion: "0.6.2"}}},
			wantChanged: []PackageInfo{},
		},
		{
			name:        "更新後の取得に失敗したら件数を作らず疑いにする",
			before:      before,
			afterErr:    errors.New("boom"),
			wantSuspect: "更新後の一覧を取得できず",
		},
		{
			name:        "更新前にあったのに更新後に 1 件も読み取れなければ疑いにする",
			before:      before,
			afterCheck:  &CheckResult{Packages: nil},
			wantSuspect: "1 件も読み取れず",
		},
		{
			name:        "更新前から空なら疑いなし",
			before:      nil,
			afterCheck:  &CheckResult{Packages: nil},
			wantChanged: []PackageInfo{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := detectVersionChanges("テスト", tc.before, tc.afterCheck, tc.afterErr)

			assert.Equal(t, tc.wantChanged, got.Changed)

			if tc.wantSuspect == "" {
				assert.Empty(t, got.Suspect)
			} else {
				assert.Contains(t, got.Suspect, tc.wantSuspect)
			}
		})
	}
}
