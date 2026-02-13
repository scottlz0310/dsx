package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/scottlz0310/devsync/internal/runner"
)

func TestPrintFailedJobDetails(t *testing.T) {
	tests := []struct {
		name     string
		summary  runner.Summary
		wantMsgs []string
		wantNone bool
	}{
		{
			name: "失敗ジョブが1件の場合",
			summary: runner.Summary{
				Results: []runner.Result{
					{Name: "apt", Status: runner.StatusSuccess},
					{Name: "npm", Status: runner.StatusFailed, Err: errors.New("npm ERR! network timeout")},
				},
			},
			wantMsgs: []string{
				"❌ 失敗ジョブの詳細:",
				"└── npm: npm ERR! network timeout",
			},
		},
		{
			name: "失敗ジョブが複数件の場合",
			summary: runner.Summary{
				Results: []runner.Result{
					{Name: "apt", Status: runner.StatusFailed, Err: errors.New("dpkg lock error")},
					{Name: "brew", Status: runner.StatusSuccess},
					{Name: "npm", Status: runner.StatusFailed, Err: errors.New("network timeout")},
				},
			},
			wantMsgs: []string{
				"❌ 失敗ジョブの詳細:",
				"├── apt: dpkg lock error",
				"└── npm: network timeout",
			},
		},
		{
			name: "失敗ジョブが0件の場合は何も出力しない",
			summary: runner.Summary{
				Results: []runner.Result{
					{Name: "apt", Status: runner.StatusSuccess},
				},
			},
			wantNone: true,
		},
		{
			name:     "結果が空の場合は何も出力しない",
			summary:  runner.Summary{},
			wantNone: true,
		},
		{
			name: "失敗だがErrがnilの場合はスキップされる",
			summary: runner.Summary{
				Results: []runner.Result{
					{Name: "apt", Status: runner.StatusFailed, Err: nil},
				},
			},
			wantNone: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStderr(t, func() {
				printFailedJobDetails(tt.summary)
			})

			if tt.wantNone {
				if output != "" {
					t.Errorf("出力なしを期待したが、出力あり: %q", output)
				}

				return
			}

			for _, msg := range tt.wantMsgs {
				if !strings.Contains(output, msg) {
					t.Errorf("出力に %q が含まれていない: %q", msg, output)
				}
			}
		})
	}
}

func TestPrintFailedErrors(t *testing.T) {
	tests := []struct {
		name     string
		errors   []error
		wantMsgs []string
		wantNone bool
	}{
		{
			name:   "エラーが1件の場合",
			errors: []error{errors.New("apt: dpkg lock error")},
			wantMsgs: []string{
				"❌ 失敗ジョブの詳細:",
				"└── apt: dpkg lock error",
			},
		},
		{
			name: "エラーが複数件の場合",
			errors: []error{
				errors.New("apt: dpkg lock error"),
				errors.New("npm: network timeout"),
			},
			wantMsgs: []string{
				"├── apt: dpkg lock error",
				"└── npm: network timeout",
			},
		},
		{
			name:     "エラーが0件の場合は何も出力しない",
			errors:   []error{},
			wantNone: true,
		},
		{
			name:     "nilスライスの場合は何も出力しない",
			errors:   nil,
			wantNone: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStderr(t, func() {
				printFailedErrors(tt.errors)
			})

			if tt.wantNone {
				if output != "" {
					t.Errorf("出力なしを期待したが、出力あり: %q", output)
				}

				return
			}

			for _, msg := range tt.wantMsgs {
				if !strings.Contains(output, msg) {
					t.Errorf("出力に %q が含まれていない: %q", msg, output)
				}
			}
		})
	}
}
