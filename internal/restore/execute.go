package restore

import (
	"context"
	"errors"
	"fmt"
)

// Runner は復元計画のコマンドを1件実行します。引数はシェルを介さず、そのまま渡します。
type Runner func(ctx context.Context, args []string) error

// Result は実行対象になった1項目の結果です。
type Result struct {
	Entry Entry
	Err   error
}

// Execute は「追加候補」だけを順に実行します。
// 導入済み・解決不能の項目には触れず、既存ツールの削除・ダウングレードも行いません。
// 1件が失敗しても残りを続行し、キャンセル時のみ打ち切ります。
func Execute(ctx context.Context, plan *Plan, run Runner) ([]Result, error) {
	if plan == nil || run == nil {
		return nil, errors.New("復元計画または実行処理が指定されていません")
	}

	// 解決不能が1件でもある計画は、不完全な復元を避けるため一切実行しません。
	if count := plan.UnresolvedCount(); count > 0 {
		return nil, fmt.Errorf("解決不能な項目が %d 件あるため実行しません", count)
	}

	var results []Result

	for i := range plan.Entries {
		entry := plan.Entries[i]
		if entry.State != Add {
			continue
		}

		if err := ctx.Err(); err != nil {
			return results, err
		}

		if len(entry.Command.Args) == 0 {
			results = append(results, Result{Entry: entry, Err: errors.New("実行コマンドがありません")})
			continue
		}

		err := run(ctx, entry.Command.Args)
		if ctxErr := ctx.Err(); ctxErr != nil {
			results = append(results, Result{Entry: entry, Err: ctxErr})
			return results, ctxErr
		}

		results = append(results, Result{Entry: entry, Err: err})
	}

	return results, nil
}
