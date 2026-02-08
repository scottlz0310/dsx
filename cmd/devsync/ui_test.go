package main

import (
	"strings"
	"testing"
)

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

func TestBuildNoTargetTUIMessage(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		requested  bool
		command    string
		wantEmpty  bool
		wantPhrase string
	}{
		{
			name:      "未指定なら空文字",
			requested: false,
			command:   "sys update",
			wantEmpty: true,
		},
		{
			name:       "指定時は説明文を返す",
			requested:  true,
			command:    "repo update",
			wantEmpty:  false,
			wantPhrase: "repo update の対象が0件",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := buildNoTargetTUIMessage(tc.requested, tc.command)
			if tc.wantEmpty {
				if got != "" {
					t.Fatalf("buildNoTargetTUIMessage() = %q, want empty", got)
				}

				return
			}

			if got == "" {
				t.Fatalf("buildNoTargetTUIMessage() = empty, want message")
			}

			if !strings.Contains(got, tc.wantPhrase) {
				t.Fatalf("buildNoTargetTUIMessage() = %q, want contains %q", got, tc.wantPhrase)
			}
		})
	}
}
