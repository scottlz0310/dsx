package diag

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testMB = 1 << 20

func newTestRecorder(t *testing.T, capture Capture) *Recorder {
	t.Helper()

	r, err := New(Options{
		Dir:           filepath.Join(t.TempDir(), "diag"),
		Capture:       capture,
		MaxFiles:      20,
		MaxTotalBytes: 20 * testMB,
	}, Meta{DsxVersion: "v0.0.0-test", Command: "sys update", StdoutTTY: false, EventsFile: true})
	require.NoError(t, err)

	// 実行環境のユーザー名やホームに依存しないよう、固定の伏せ字化にする
	r.redactor = newRedactorFor(`C:\Users\alice`, "alice", "devbox")

	return r
}

func readJSONLines(t *testing.T, path string) []map[string]any {
	t.Helper()

	file, err := os.Open(path)
	require.NoError(t, err)

	defer func() { require.NoError(t, file.Close()) }()

	var lines []map[string]any

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1<<20), 8<<20)

	for scanner.Scan() {
		var line map[string]any

		require.NoError(t, json.Unmarshal(scanner.Bytes(), &line), "JSON Lines として読めること: %s", scanner.Text())
		lines = append(lines, line)
	}

	require.NoError(t, scanner.Err())

	return lines
}

func TestNew_Validation(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		ok   bool
	}{
		{"正常", Options{Dir: t.TempDir(), Capture: CaptureAnomaly, MaxFiles: 1, MaxTotalBytes: 1}, true},
		{"all も正常", Options{Dir: t.TempDir(), Capture: CaptureAll, MaxFiles: 1, MaxTotalBytes: 1}, true},
		{"未知の capture", Options{Dir: t.TempDir(), Capture: "bogus", MaxFiles: 1, MaxTotalBytes: 1}, false},
		{"capture が空", Options{Dir: t.TempDir(), Capture: "", MaxFiles: 1, MaxTotalBytes: 1}, false},
		{"max_files が 0", Options{Dir: t.TempDir(), Capture: CaptureAll, MaxFiles: 0, MaxTotalBytes: 1}, false},
		{"容量が 0", Options{Dir: t.TempDir(), Capture: CaptureAll, MaxFiles: 1, MaxTotalBytes: 0}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := New(tt.opts, Meta{})
			if tt.ok {
				require.NoError(t, err)
				assert.NotNil(t, r)

				return
			}

			require.Error(t, err)
		})
	}
}

func TestNew_DoesNotTouchFilesystem(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "never-created")

	r, err := New(Options{Dir: dir, Capture: CaptureAll, MaxFiles: 1, MaxTotalBytes: 1}, Meta{})
	require.NoError(t, err)
	assert.NotEmpty(t, r.Path())

	_, statErr := os.Stat(dir)
	assert.True(t, os.IsNotExist(statErr), "記録するまでディレクトリを作らない")
}

func TestRecorder_Reason(t *testing.T) {
	tests := []struct {
		name    string
		capture Capture
		failed  bool
		suspect string
		want    string
	}{
		{"anomaly: 異常なし", CaptureAnomaly, false, "", ""},
		{"anomaly: 疑いあり", CaptureAnomaly, false, "疑い", ReasonSuspect},
		{"anomaly: 失敗", CaptureAnomaly, true, "", ReasonCommandFailed},
		{"anomaly: 疑いと失敗の両方", CaptureAnomaly, true, "疑い", ReasonSuspect + "," + ReasonCommandFailed},
		{"all: 異常なしでも保存", CaptureAll, false, "", ReasonAll},
		{"all: 疑いありは suspect", CaptureAll, false, "疑い", ReasonSuspect},
		{"all: 失敗は command_failed", CaptureAll, true, "", ReasonCommandFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRecorder(t, tt.capture)
			assert.Equal(t, tt.want, r.Reason(tt.failed, tt.suspect))
		})
	}
}

func TestRecorder_NilIsDisabled(t *testing.T) {
	var r *Recorder

	assert.Empty(t, r.Reason(true, "疑い"), "無効なら保存しない")
	assert.Empty(t, r.Path())
	assert.NoError(t, r.Record(ReasonAll, &Entry{Manager: "winget"}))
}

func TestRecorder_Record(t *testing.T) {
	r := newTestRecorder(t, CaptureAnomaly)

	require.NoError(t, r.Record("suspect", &Entry{
		Manager:     "scoop",
		Command:     []string{"scoop", "status"},
		Output:      []byte("Name Installed Version\nC:\\Users\\alice\\scoop\\apps\\fd owner alice\n"),
		Suspect:     "表を読めません",
		ToolVersion: "Current Scoop version: v0.5.3",
	}))
	require.NoError(t, r.Record("command_failed", &Entry{
		Manager: "apt",
		Command: []string{"apt", "list", "--upgradable"},
		Err:     fmt.Errorf("apt list に失敗: C:\\Users\\alice\\x: %w", os.ErrNotExist),
	}))

	lines := readJSONLines(t, r.Path())
	require.Len(t, lines, 3, "メタ 1 行と記録 2 行")

	meta := lines[0]
	assert.Equal(t, "meta", meta["type"])
	assert.EqualValues(t, schemaVersion, meta["schema"])
	assert.Equal(t, "v0.0.0-test", meta["dsx_version"])
	assert.Equal(t, "sys update", meta["command"])
	assert.Equal(t, runtime.GOOS, meta["os"])
	assert.Equal(t, runtime.GOARCH, meta["arch"])
	assert.Equal(t, false, meta["stdout_tty"])
	assert.Equal(t, true, meta["events_file"])
	assert.Equal(t, "anomaly", meta["capture"])
	assert.Contains(t, []any{"set", "unset"}, meta["no_color"])

	first := lines[1]
	assert.Equal(t, "capture", first["type"])
	assert.Equal(t, "scoop", first["manager"])
	assert.Equal(t, "suspect", first["reason"])
	assert.Equal(t, "scoop status", first["command"])
	assert.Equal(t, "表を読めません", first["suspect"])
	assert.Equal(t, "Current Scoop version: v0.5.3", first["tool_version"])
	assert.Equal(t, "Name Installed Version\n~\\scoop\\apps\\fd owner <user>\n", first["output"], "ホームとユーザー名は伏せ字")
	assert.NotContains(t, first, "exit_code")
	assert.NotContains(t, first, "output_truncated")

	second := lines[2]
	assert.Equal(t, "apt", second["manager"])
	assert.Equal(t, "command_failed", second["reason"])
	assert.Contains(t, second["error"], "~\\x", "エラー文のホームも伏せ字")
	assert.NotContains(t, second["error"], "alice")
}

func TestRecorder_RecordWritesLazilyAndAppends(t *testing.T) {
	r := newTestRecorder(t, CaptureAll)

	_, err := os.Stat(r.Path())
	require.True(t, os.IsNotExist(err), "記録前はファイルが無い")

	require.NoError(t, r.Record(ReasonAll, &Entry{Manager: "a"}))
	require.NoError(t, r.Record(ReasonAll, &Entry{Manager: "b"}))

	lines := readJSONLines(t, r.Path())
	require.Len(t, lines, 3)

	metas := 0

	for _, line := range lines {
		if line["type"] == "meta" {
			metas++
		}
	}

	assert.Equal(t, 1, metas, "メタ行はファイルの先頭に 1 回だけ")
}

func TestRecorder_ExitCode(t *testing.T) {
	r := newTestRecorder(t, CaptureAnomaly)

	runErr := exec.CommandContext(t.Context(), "go", "no-such-subcommand").Run()

	var exitErr *exec.ExitError

	require.ErrorAs(t, runErr, &exitErr, "go のサブコマンドの誤りは終了コード付きで失敗する")

	require.NoError(t, r.Record(ReasonCommandFailed, &Entry{Manager: "m", Err: runErr}))

	lines := readJSONLines(t, r.Path())
	require.Len(t, lines, 2)
	assert.EqualValues(t, exitErr.ExitCode(), lines[1]["exit_code"])
}

func TestRecorder_OutputHandling(t *testing.T) {
	tests := []struct {
		name          string
		output        []byte
		wantTruncated bool
		wantInvalid   bool
		wantLen       func(t *testing.T, got string)
	}{
		{
			name:    "空の出力",
			output:  nil,
			wantLen: func(t *testing.T, got string) { assert.Empty(t, got) },
		},
		{
			name:          "上限ちょうどは切り捨てない",
			output:        []byte(strings.Repeat("x", maxOutputBytes)),
			wantTruncated: false,
			wantLen:       func(t *testing.T, got string) { assert.Len(t, got, maxOutputBytes) },
		},
		{
			name:          "上限を超えたら切り捨てる",
			output:        []byte(strings.Repeat("x", maxOutputBytes+10)),
			wantTruncated: true,
			wantLen:       func(t *testing.T, got string) { assert.Len(t, got, maxOutputBytes) },
		},
		{
			// 3 バイト文字が上限の境界をまたいでも、壊れた文字を残さない
			name:          "多バイト文字の途中では切らない",
			output:        []byte(strings.Repeat("あ", maxOutputBytes/3+5)),
			wantTruncated: true,
			wantLen: func(t *testing.T, got string) {
				assert.LessOrEqual(t, len(got), maxOutputBytes)
				assert.NotContains(t, got, "\uFFFD")
			},
		},
		{
			name:        "不正な UTF-8 は印を付ける",
			output:      []byte("ok\xff\xfe end"),
			wantInvalid: true,
			wantLen:     func(t *testing.T, got string) { assert.Contains(t, got, "ok") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRecorder(t, CaptureAll)

			require.NoError(t, r.Record(ReasonAll, &Entry{Manager: "m", Output: tt.output}))

			lines := readJSONLines(t, r.Path())
			require.Len(t, lines, 2)

			got, ok := lines[1]["output"].(string)
			require.True(t, ok, "output は文字列")
			tt.wantLen(t, got)
			assert.Equal(t, tt.wantTruncated, lines[1]["output_truncated"] == true)
			assert.Equal(t, tt.wantInvalid, lines[1]["invalid_utf8"] == true)
		})
	}
}

func TestRecorder_Permissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows では POSIX の権限ビットを使わない（LOCALAPPDATA の利用者別 ACL に従う）")
	}

	r := newTestRecorder(t, CaptureAll)

	require.NoError(t, r.Record(ReasonAll, &Entry{Manager: "m"}))

	dirInfo, err := os.Stat(filepath.Dir(r.Path()))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm(), "保存先は利用者のみ")

	fileInfo, err := os.Stat(r.Path())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm(), "ログは利用者のみ")
}

// writeOldLog は、ローテーションの検証用に、指定した時刻の名前と大きさのログファイルを作ります。
func writeOldLog(t *testing.T, dir string, at time.Time, size int) string {
	t.Helper()

	require.NoError(t, os.MkdirAll(dir, 0o700))

	path := filepath.Join(dir, fmt.Sprintf("%s%s-1%s", filePrefix, at.UTC().Format("20060102T150405Z"), fileSuffix))
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("x", size)), 0o600))

	return path
}

func TestRecorder_RotationByCount(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "diag")
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	old := make([]string, 0, 5)
	for i := range 5 {
		old = append(old, writeOldLog(t, dir, base.Add(time.Duration(i)*time.Hour), 10))
	}

	// 対象外のファイルとディレクトリは、数にも削除にも含めない
	other := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(other, []byte("keep"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, filePrefix+"subdir"+fileSuffix), 0o700))

	r, err := New(Options{Dir: dir, Capture: CaptureAll, MaxFiles: 3, MaxTotalBytes: 20 * testMB}, Meta{})
	require.NoError(t, err)
	require.NoError(t, r.Record(ReasonAll, &Entry{Manager: "m"}))

	// 今回のファイル + 古い 2 件 = 3 件。古い方 3 件が消える
	for i, path := range old {
		_, statErr := os.Stat(path)
		if i < 3 {
			assert.True(t, os.IsNotExist(statErr), "古い %d 件目は削除される", i)
		} else {
			assert.NoError(t, statErr, "新しい %d 件目は残る", i)
		}
	}

	assert.FileExists(t, r.Path(), "今回のファイルは削除しない")
	assert.FileExists(t, other, "対象外のファイルは削除しない")
}

func TestRecorder_RotationBySize(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "diag")
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	old := []string{
		writeOldLog(t, dir, base, 1000),
		writeOldLog(t, dir, base.Add(time.Hour), 1000),
		writeOldLog(t, dir, base.Add(2*time.Hour), 1000),
	}

	// 合計が 2500 バイトを超えない範囲で、新しいものを残す
	r, err := New(Options{Dir: dir, Capture: CaptureAll, MaxFiles: 100, MaxTotalBytes: 2500}, Meta{})
	require.NoError(t, err)
	require.NoError(t, r.Record(ReasonAll, &Entry{Manager: "m"}))

	_, firstErr := os.Stat(old[0])
	assert.True(t, os.IsNotExist(firstErr), "いちばん古いものから削除される")
	assert.FileExists(t, old[2], "新しいものは残る")
	assert.FileExists(t, r.Path())

	var total int64

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	for _, entry := range entries {
		info, infoErr := entry.Info()
		require.NoError(t, infoErr)

		total += info.Size()
	}

	assert.LessOrEqual(t, total, int64(2500), "合計容量が上限以内")
}

func TestRecorder_RunFileFull(t *testing.T) {
	r, err := New(Options{Dir: filepath.Join(t.TempDir(), "diag"), Capture: CaptureAll, MaxFiles: 5, MaxTotalBytes: 300}, Meta{})
	require.NoError(t, err)

	err = r.Record(ReasonAll, &Entry{Manager: "m", Output: []byte(strings.Repeat("x", 1000))})
	require.ErrorIs(t, err, ErrRunFileFull)

	_, statErr := os.Stat(r.Path())
	assert.True(t, os.IsNotExist(statErr), "上限を超える記録では空のファイルも残さない")
}

func TestRecorder_ConcurrentRecord(t *testing.T) {
	r := newTestRecorder(t, CaptureAll)

	const workers = 8

	var wg sync.WaitGroup

	for i := range workers {
		wg.Go(func() {
			assert.NoError(t, r.Record(ReasonAll, &Entry{Manager: fmt.Sprintf("m%d", i), Output: []byte("out")}))
		})
	}

	wg.Wait()

	lines := readJSONLines(t, r.Path())
	assert.Len(t, lines, workers+1, "並行して書いても、メタ 1 行と記録 %d 行がすべて JSON として読める", workers)
}

func TestTruncateUTF8(t *testing.T) {
	tests := []struct {
		name          string
		data          string
		max           int
		want          string
		wantTruncated bool
	}{
		{"収まる", "abc", 5, "abc", false},
		{"ちょうど", "abc", 3, "abc", false},
		{"ASCII を切る", "abcdef", 3, "abc", true},
		{"3 バイト文字の途中で切れる位置は手前に戻る", "あいう", 4, "あ", true},
		{"3 バイト文字の境界ならそのまま", "あいう", 6, "あい", true},
		{"空", "", 3, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, truncated := truncateUTF8([]byte(tt.data), tt.max)
			assert.Equal(t, tt.want, string(got))
			assert.Equal(t, tt.wantTruncated, truncated)
		})
	}
}

// 伏せ字（<user> など）は、HTML 用にエスケープせず、読める形のままファイルに残る。
func TestRecorder_RedactionMarkersStayReadable(t *testing.T) {
	r := newTestRecorder(t, CaptureAll)

	require.NoError(t, r.Record(ReasonAll, &Entry{Manager: "m", Output: []byte("owner alice token=abc123")}))

	raw, err := os.ReadFile(r.Path())
	require.NoError(t, err)

	assert.Contains(t, string(raw), "<user>")
	assert.Contains(t, string(raw), "<redacted>")
	assert.NotContains(t, string(raw), "u003c", "< を \u003c にエスケープしない")
	assert.NotContains(t, string(raw), "alice")
	assert.NotContains(t, string(raw), "abc123")
}
