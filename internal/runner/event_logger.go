package runner

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// EventLogger はジョブ実行イベントをファイルに記録します。
type EventLogger struct {
	file      *os.File
	mu        sync.Mutex
	startedAt time.Time
	writeErr  error
}

// NewEventLogger は指定パスにログファイルを作成し、EventLogger を返します。
func NewEventLogger(path string) (*EventLogger, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("ログファイルを作成できません: %w", err)
	}

	now := time.Now()
	logger := &EventLogger{
		file:      f,
		startedAt: now,
	}

	logger.writeLine(fmt.Sprintf("# devsync ジョブログ — %s", now.Format(time.RFC3339)))
	logger.writeLine("")

	return logger, nil
}

// LogEvent はイベントを1行のログとしてファイルに書き込みます。
func (l *EventLogger) LogEvent(event *Event) {
	ts := event.Timestamp.Format("15:04:05.000")

	var line string

	switch event.Type {
	case EventQueued:
		line = fmt.Sprintf("%s [QUEUED]   %s", ts, event.JobName)
	case EventStarted:
		line = fmt.Sprintf("%s [STARTED]  %s", ts, event.JobName)
	case EventFinished:
		status := statusLabel(event.Status)
		dur := event.Duration.Round(time.Millisecond)

		if event.Err != nil {
			line = fmt.Sprintf("%s [%s] %s (%s): %v", ts, status, event.JobName, dur, event.Err)
		} else {
			line = fmt.Sprintf("%s [%s] %s (%s)", ts, status, event.JobName, dur)
		}
	default:
		// 未知のイベントタイプはログ出力をスキップ
		return
	}

	l.writeLine(line)
}

// WriteSummary はログ末尾にサマリーを書き込みます。
func (l *EventLogger) WriteSummary(summary Summary) {
	l.writeLine("")
	l.writeLine(fmt.Sprintf("# サマリー: 成功 %d / 失敗 %d / スキップ %d / 総数 %d",
		summary.Success, summary.Failed, summary.Skipped, summary.Total))
	l.writeLine(fmt.Sprintf("# 所要時間: %s", time.Since(l.startedAt).Round(time.Millisecond)))
}

// Close はログファイルを閉じます。書き込みエラーがあった場合はそれも報告します。
func (l *EventLogger) Close() error {
	closeErr := l.file.Close()

	if l.writeErr != nil {
		return l.writeErr
	}

	return closeErr
}

func (l *EventLogger) writeLine(line string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.writeErr != nil {
		return
	}

	if _, err := fmt.Fprintln(l.file, line); err != nil {
		l.writeErr = fmt.Errorf("ログファイルへの書き込みに失敗: %w", err)
	}
}

func statusLabel(s ResultStatus) string {
	switch s {
	case StatusSuccess:
		return "SUCCESS"
	case StatusFailed:
		return "FAILED "
	case StatusSkipped:
		return "SKIPPED"
	default:
		return "UNKNOWN"
	}
}
