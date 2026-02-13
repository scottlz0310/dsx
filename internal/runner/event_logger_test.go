package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEventLogger_基本的なイベント記録(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	logger, err := NewEventLogger(logPath)
	if err != nil {
		t.Fatalf("NewEventLogger() error = %v", err)
	}

	now := time.Now()

	logger.LogEvent(&Event{
		Type:      EventQueued,
		JobIndex:  0,
		JobName:   "apt",
		Timestamp: now,
	})

	logger.LogEvent(&Event{
		Type:      EventStarted,
		JobIndex:  0,
		JobName:   "apt",
		Timestamp: now.Add(time.Second),
	})

	logger.LogEvent(&Event{
		Type:      EventFinished,
		JobIndex:  0,
		JobName:   "apt",
		Status:    StatusSuccess,
		Duration:  3 * time.Second,
		Timestamp: now.Add(4 * time.Second),
	})

	logger.WriteSummary(Summary{
		Total:   1,
		Success: 1,
	})

	content := closeAndRead(t, logger, logPath)

	checks := []struct {
		name     string
		contains string
	}{
		{"ヘッダー", "# devsync ジョブログ"},
		{"キュー", "[QUEUED]"},
		{"開始", "[STARTED]"},
		{"成功", "[SUCCESS]"},
		{"ジョブ名", "apt"},
		{"所要時間", "3s"},
		{"サマリー", "# サマリー: 成功 1"},
		{"総所要時間", "# 所要時間:"},
	}

	for _, c := range checks {
		if !strings.Contains(content, c.contains) {
			t.Errorf("%s: ログに %q が含まれていません\n実際の内容:\n%s", c.name, c.contains, content)
		}
	}
}

func TestEventLogger_失敗イベント(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "fail.log")

	logger, err := NewEventLogger(logPath)
	if err != nil {
		t.Fatalf("NewEventLogger() error = %v", err)
	}

	logger.LogEvent(&Event{
		Type:      EventFinished,
		JobIndex:  0,
		JobName:   "brew",
		Status:    StatusFailed,
		Err:       errForTest("brew update failed"),
		Duration:  5 * time.Second,
		Timestamp: time.Now(),
	})

	content := closeAndRead(t, logger, logPath)

	if !strings.Contains(content, "[FAILED ]") {
		t.Errorf("ログに [FAILED ] が含まれていません: %s", content)
	}

	if !strings.Contains(content, "brew update failed") {
		t.Errorf("ログにエラーメッセージが含まれていません: %s", content)
	}
}

func TestEventLogger_スキップイベント(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "skip.log")

	logger, err := NewEventLogger(logPath)
	if err != nil {
		t.Fatalf("NewEventLogger() error = %v", err)
	}

	logger.LogEvent(&Event{
		Type:      EventFinished,
		JobIndex:  0,
		JobName:   "snap",
		Status:    StatusSkipped,
		Duration:  0,
		Timestamp: time.Now(),
	})

	content := closeAndRead(t, logger, logPath)

	if !strings.Contains(content, "[SKIPPED]") {
		t.Errorf("ログに [SKIPPED] が含まれていません: %s", content)
	}
}

func TestNewEventLogger_無効なパス(t *testing.T) {
	_, err := NewEventLogger(filepath.Join(t.TempDir(), "nonexistent", "deep", "dir", "test.log"))
	if err == nil {
		t.Error("存在しないディレクトリでエラーが返されるべきです")
	}
}

// errForTest はテスト用のエラーを生成します。
type errForTest string

func (e errForTest) Error() string { return string(e) }

// closeAndRead はロガーを閉じてログファイルの内容を文字列で返します。
func closeAndRead(t *testing.T, logger *EventLogger, path string) string {
	t.Helper()

	if err := logger.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	return string(data)
}
