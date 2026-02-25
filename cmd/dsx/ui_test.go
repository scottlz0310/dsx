package main

import (
	"strings"
	"testing"
)

func TestResolveTUIRequest(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		configDefault bool
		tuiChanged    bool
		tuiValue      bool
		noTUIChanged  bool
		noTUIValue    bool
		want          tuiRequest
		wantErr       bool
	}{
		{
			name:          "フラグ未指定かつ設定OFFなら未要求",
			configDefault: false,
			want:          tuiRequest{Requested: false, Source: tuiSourceNone},
		},
		{
			name:          "フラグ未指定かつ設定ONなら設定由来で要求",
			configDefault: true,
			want:          tuiRequest{Requested: true, Source: tuiSourceConfig},
		},
		{
			name:       "--tui は設定より優先して要求",
			tuiChanged: true,
			tuiValue:   true,
			want:       tuiRequest{Requested: true, Source: tuiSourceFlag},
		},
		{
			name:          "--tui=false は設定ONでも明示的に無効化",
			configDefault: true,
			tuiChanged:    true,
			tuiValue:      false,
			want:          tuiRequest{Requested: false, Source: tuiSourceFlag},
		},
		{
			name:          "--no-tui は無効化を強制",
			configDefault: true,
			noTUIChanged:  true,
			noTUIValue:    true,
			want:          tuiRequest{Requested: false, Source: tuiSourceFlag},
		},
		{
			name:         "--tui と --no-tui の矛盾はエラー",
			tuiChanged:   true,
			tuiValue:     true,
			noTUIChanged: true,
			noTUIValue:   true,
			wantErr:      true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveTUIRequest(tc.configDefault, tc.tuiChanged, tc.tuiValue, tc.noTUIChanged, tc.noTUIValue)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveTUIRequest() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("resolveTUIRequest() unexpected error: %v", err)
			}

			if got.Requested != tc.want.Requested {
				t.Fatalf("resolveTUIRequest() requested = %v, want %v", got.Requested, tc.want.Requested)
			}

			if got.Source != tc.want.Source {
				t.Fatalf("resolveTUIRequest() source = %v, want %v", got.Source, tc.want.Source)
			}
		})
	}
}

func TestResolveTUIEnabledByTerminal(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		request   tuiRequest
		stdoutTTY bool
		stderrTTY bool
		want      bool
		wantWarn  bool
		wantInMsg string
	}{
		{
			name:      "未指定なら無効",
			request:   tuiRequest{Requested: false, Source: tuiSourceNone},
			stdoutTTY: true,
			stderrTTY: true,
			want:      false,
			wantWarn:  false,
		},
		{
			name:      "対話端末なら有効",
			request:   tuiRequest{Requested: true, Source: tuiSourceFlag},
			stdoutTTY: true,
			stderrTTY: true,
			want:      true,
			wantWarn:  false,
		},
		{
			name:      "stdoutが非対話なら無効（フラグ由来の警告）",
			request:   tuiRequest{Requested: true, Source: tuiSourceFlag},
			stdoutTTY: false,
			stderrTTY: true,
			want:      false,
			wantWarn:  true,
			wantInMsg: "--tui",
		},
		{
			name:      "stderrが非対話なら無効（設定由来の警告）",
			request:   tuiRequest{Requested: true, Source: tuiSourceConfig},
			stdoutTTY: true,
			stderrTTY: false,
			want:      false,
			wantWarn:  true,
			wantInMsg: "ui.tui",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, warn := resolveTUIEnabledByTerminal(tc.request, tc.stdoutTTY, tc.stderrTTY)
			if got != tc.want {
				t.Fatalf("resolveTUIEnabledByTerminal() enabled = %v, want %v", got, tc.want)
			}

			if (warn != "") != tc.wantWarn {
				t.Fatalf("resolveTUIEnabledByTerminal() warning = %q, wantWarn=%v", warn, tc.wantWarn)
			}

			if tc.wantInMsg != "" && !strings.Contains(warn, tc.wantInMsg) {
				t.Fatalf("resolveTUIEnabledByTerminal() warning = %q, want contains %q", warn, tc.wantInMsg)
			}
		})
	}
}

func TestBuildNoTargetTUIMessage(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		request    tuiRequest
		command    string
		wantEmpty  bool
		wantPhrase string
	}{
		{
			name:      "未指定なら空文字",
			request:   tuiRequest{Requested: false, Source: tuiSourceNone},
			command:   "sys update",
			wantEmpty: true,
		},
		{
			name:       "フラグ指定時は説明文を返す",
			request:    tuiRequest{Requested: true, Source: tuiSourceFlag},
			command:    "repo update",
			wantEmpty:  false,
			wantPhrase: "repo update の対象が0件",
		},
		{
			name:       "設定由来でも説明文を返す",
			request:    tuiRequest{Requested: true, Source: tuiSourceConfig},
			command:    "sys update",
			wantEmpty:  false,
			wantPhrase: "ui.tui",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := buildNoTargetTUIMessage(tc.request, tc.command)
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
