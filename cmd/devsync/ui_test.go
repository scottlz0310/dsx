package main

import "testing"

func TestResolveTUIEnabledByTerminal(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		requested bool
		stdoutTTY bool
		stderrTTY bool
		want      bool
		wantWarn  bool
	}{
		{
			name:      "未指定なら無効",
			requested: false,
			stdoutTTY: true,
			stderrTTY: true,
			want:      false,
			wantWarn:  false,
		},
		{
			name:      "対話端末なら有効",
			requested: true,
			stdoutTTY: true,
			stderrTTY: true,
			want:      true,
			wantWarn:  false,
		},
		{
			name:      "stdoutが非対話なら無効",
			requested: true,
			stdoutTTY: false,
			stderrTTY: true,
			want:      false,
			wantWarn:  true,
		},
		{
			name:      "stderrが非対話なら無効",
			requested: true,
			stdoutTTY: true,
			stderrTTY: false,
			want:      false,
			wantWarn:  true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, warn := resolveTUIEnabledByTerminal(tc.requested, tc.stdoutTTY, tc.stderrTTY)
			if got != tc.want {
				t.Fatalf("resolveTUIEnabledByTerminal() enabled = %v, want %v", got, tc.want)
			}

			if (warn != "") != tc.wantWarn {
				t.Fatalf("resolveTUIEnabledByTerminal() warning = %q, wantWarn=%v", warn, tc.wantWarn)
			}
		})
	}
}
