package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/scottlz0310/dsx/internal/event"
	repomgr "github.com/scottlz0310/dsx/internal/repo"
	"github.com/scottlz0310/dsx/internal/updater"
)

// activeEvents は実行中のコマンドのイベント出力です。--events-file が無いときは nil（何もしない）。
// run が sys → repo を呼ぶとき、1 つの Stream を共有して、全体で 1 つの要約にするために使います。
var activeEvents *event.Stream

// beginEvents は --events-file のイベント出力を開始します。戻り値の finish を、コマンドの終了時に呼びます。
// path が空のとき、または run などの上位のコマンドが開始済みのときは、何もしない finish を返します。
func beginEvents(command, path string) (finish func(runErr error), err error) {
	noop := func(error) {}

	if activeEvents != nil || path == "" {
		return noop, nil
	}

	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("イベントファイルを作成できません: %w", err)
	}

	stream := event.New(file, command, version, nil)
	activeEvents = stream

	return func(runErr error) {
		activeEvents = nil

		if finishErr := stream.Finish(runErr); finishErr != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %v\n", finishErr)
		}

		if closeErr := file.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "⚠️  イベントファイルのクローズに失敗: %v\n", closeErr)
		}
	}, nil
}

// recordEnableWarning は updater.GetEnabled の警告をイベントにします。
// 利用不可のマネージャは実行されなかったジョブ（skipped）、未知の名前と設定の適用失敗は警告として記録します。
func recordEnableWarning(err error) {
	var enableWarning *updater.EnableWarning
	if !errors.As(err, &enableWarning) {
		activeEvents.Warn(event.CategoryManagerConfig, "", err.Error())

		return
	}

	for _, name := range enableWarning.Unavailable {
		activeEvents.JobFinished(event.KindManager, name, event.JobResult{
			Status: event.StatusSkipped,
			Code:   event.CodeUnavailable,
			Reason: "未インストールまたは利用不可のためスキップ",
		})
	}

	for _, name := range enableWarning.NotFound {
		activeEvents.Warn(event.CategoryManagerUnknown, name, "未知のマネージャが指定されています")
	}
}

// updaterJobResult はマネージャの更新結果を、終了コードの判定と一致するジョブ結果にします。
// 失敗（部分的なエラーを含む）は failed、キャンセルとタイムアウトは skipped（cancelled）です。
func updaterJobResult(result *updater.UpdateResult, err error) event.JobResult {
	if err != nil {
		return failedJobResult(err)
	}

	if result == nil {
		return event.JobResult{Status: event.StatusSuccess}
	}

	if result.FailedCount > 0 || len(result.Errors) > 0 {
		reasons := make([]string, 0, len(result.Errors))
		for _, e := range result.Errors {
			reasons = append(reasons, e.Error())
		}

		reason := strings.Join(reasons, "; ")
		if reason == "" {
			reason = fmt.Sprintf("%d 件の更新に失敗しました", result.FailedCount)
		}

		return event.JobResult{Status: event.StatusFailed, Code: event.CodeError, Reason: reason, Updated: result.UpdatedCount}
	}

	return event.JobResult{Status: event.StatusSuccess, Updated: result.UpdatedCount}
}

// selfUpdateJobResult はマネージャ本体更新の結果をジョブ結果にします。
// 実行中で更新を見送った場合は skipped（in_use）で、終了コードには影響しません。
func selfUpdateJobResult(result *updater.SelfUpdateResult, err error) event.JobResult {
	if err == nil && result != nil && result.InUse {
		return event.JobResult{Status: event.StatusSkipped, Code: event.CodeInUse, Reason: result.Message}
	}

	var updateResult *updater.UpdateResult
	if result != nil {
		updateResult = &result.UpdateResult
	}

	return updaterJobResult(updateResult, err)
}

// repoJobResult はリポジトリの更新結果をジョブ結果にします。
// pull を実行しなかったリポジトリは skipped（pull_skipped）で、終了コードには影響しません。
func repoJobResult(result *repomgr.UpdateResult, err error) event.JobResult {
	if err != nil {
		return failedJobResult(err)
	}

	if result != nil && len(result.SkippedMessages) > 0 {
		return event.JobResult{
			Status: event.StatusSkipped,
			Code:   event.CodePullSkipped,
			Reason: strings.Join(result.SkippedMessages, "; "),
		}
	}

	return event.JobResult{Status: event.StatusSuccess}
}

func failedJobResult(err error) event.JobResult {
	if isContextCancellation(err) {
		return event.JobResult{Status: event.StatusSkipped, Code: event.CodeCancelled, Reason: "キャンセルまたはタイムアウトにより完了しませんでした"}
	}

	return event.JobResult{Status: event.StatusFailed, Code: event.CodeError, Reason: err.Error()}
}

// managerNames はマネージャ名の一覧を返します。
func managerNames(updaters []updater.Updater) []string {
	names := make([]string, 0, len(updaters))
	for _, u := range updaters {
		names = append(names, u.Name())
	}

	return names
}

// recordPhaseFailure は sys・repo の段階が返したエラーを、failed（または cancelled）のイベントにします。
// run は段階のエラーを件数だけのエラーにまとめるため、個々のジョブより前に失敗した原因
// （root が不正など）を、ここで具体的な理由のまま残します。
func recordPhaseFailure(phase string, err error) {
	if err == nil {
		return
	}

	activeEvents.JobFinished(event.KindPhase, phase, failedJobResult(err))
}
