package restore

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExecute(t *testing.T) {
	add := func(name string) Entry {
		return Entry{Manager: "uv", State: Add, Command: Command{Args: []string{"uv", "tool", "install", name}}}
	}

	tests := []struct {
		name      string
		plan      *Plan
		failOn    string
		cancelOn  string
		wantRan   []string
		wantErrs  int
		wantError bool
	}{
		{name: "追加候補だけ実行する", plan: &Plan{Entries: []Entry{add("a"), {State: Existing}, add("b")}}, wantRan: []string{"a", "b"}},
		{name: "空の計画", plan: &Plan{}},
		{name: "nil の計画", plan: nil, wantError: true},
		{name: "解決不能があれば何も実行しない", plan: &Plan{Entries: []Entry{add("a"), {State: Unresolved}}}, wantError: true},
		{name: "失敗しても残りを続行する", plan: &Plan{Entries: []Entry{add("a"), add("b")}}, failOn: "a", wantRan: []string{"a", "b"}, wantErrs: 1},
		{name: "コマンドが空の項目はエラー", plan: &Plan{Entries: []Entry{{State: Add}, add("b")}}, wantRan: []string{"b"}, wantErrs: 1},
		{name: "キャンセルで打ち切る", plan: &Plan{Entries: []Entry{add("a"), add("b")}}, cancelOn: "a", wantRan: []string{"a"}, wantErrs: 1, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var ran []string

			run := func(_ context.Context, args []string) error {
				name := args[len(args)-1]
				ran = append(ran, name)

				if name == tt.cancelOn {
					cancel()
				}

				if name == tt.failOn {
					return errors.New("失敗")
				}

				return nil
			}

			results, err := Execute(ctx, tt.plan, run)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tt.wantRan, ran)

			failed := 0

			for _, r := range results {
				if r.Err != nil {
					failed++
				}
			}

			require.Equal(t, tt.wantErrs, failed)
		})
	}

	_, err := Execute(context.Background(), &Plan{}, nil)
	require.Error(t, err)
}
