package diag

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Capture は診断ログに保存する対象です。
type Capture string

const (
	// CaptureAnomaly は判定不能の疑い、またはコマンド失敗のときだけ保存します。
	CaptureAnomaly Capture = "anomaly"
	// CaptureAll は解析対象コマンドの出力を毎回保存します。
	CaptureAll Capture = "all"
)

// 保存の理由（capture 行の reason）。複数該当するときは "," で連結する。
const (
	ReasonAll           = "all"
	ReasonSuspect       = "suspect"
	ReasonCommandFailed = "command_failed"
)

const (
	filePrefix = "diag-"
	fileSuffix = ".jsonl"

	// schemaVersion は診断ログのファイル形式の版です。
	schemaVersion = 1

	// maxOutputBytes は 1 回の出力として保存する上限です。超えた分は切り捨てます。
	maxOutputBytes = 256 * 1024

	dirPerm  = 0o700
	filePerm = 0o600
)

// ErrRunFileFull は、この実行のログファイルが容量の上限に達し、これ以上追記できないことを表します。
var ErrRunFileFull = errors.New("診断ログのファイルが容量の上限に達しました")

// Options は Recorder の設定です。
type Options struct {
	// Dir は保存先ディレクトリです。空なら DefaultDir を使います。
	Dir     string
	Capture Capture
	// MaxFiles は保持するファイルの最大数、MaxTotalBytes は合計容量の上限です。
	// 超えた分は古いファイルから削除します（今回の実行のファイルは削除しません）。
	MaxFiles      int
	MaxTotalBytes int64
}

// Meta は dsx の 1 回の実行に共通する環境情報です。ログファイルの先頭に 1 行で記録します。
type Meta struct {
	DsxVersion string
	Command    string
	StdoutTTY  bool
	EventsFile bool
}

// Entry は解析対象コマンド 1 回分の記録です。
type Entry struct {
	Manager     string
	Command     []string
	Output      []byte
	Err         error
	Suspect     string
	ToolVersion string
}

// Recorder は診断ログの書き込み先です。nil は「無効」を表し、すべての操作が何もしません。
type Recorder struct {
	opts     Options
	meta     Meta
	redactor *redactor
	now      func() time.Time

	mu        sync.Mutex
	path      string
	wroteMeta bool
	written   int64
}

// New は Recorder を作ります。ファイルとディレクトリは、最初に記録するときまで作りません。
func New(opts Options, meta Meta) (*Recorder, error) {
	if opts.Dir == "" {
		dir, err := DefaultDir()
		if err != nil {
			return nil, err
		}

		opts.Dir = dir
	}

	if opts.Capture != CaptureAnomaly && opts.Capture != CaptureAll {
		return nil, fmt.Errorf("未対応の capture です: %q", opts.Capture)
	}

	if opts.MaxFiles < 1 || opts.MaxTotalBytes < 1 {
		return nil, errors.New("max_files と max_total_mb は 1 以上が必要です")
	}

	r := &Recorder{opts: opts, meta: meta, redactor: newRedactor(), now: time.Now}
	r.path = filepath.Join(opts.Dir, fmt.Sprintf("%s%s-%d%s", filePrefix, r.now().UTC().Format("20060102T150405Z"), os.Getpid(), fileSuffix))

	return r, nil
}

// Reason は、この結果を保存すべきかを判定し、保存する理由を返します（保存しないなら空文字列）。
// failed は、コマンドの失敗として扱った（dsx がエラーにした）場合です。
func (r *Recorder) Reason(failed bool, suspect string) string {
	if r == nil {
		return ""
	}

	var reasons []string

	if suspect != "" {
		reasons = append(reasons, ReasonSuspect)
	}

	if failed {
		reasons = append(reasons, ReasonCommandFailed)
	}

	if len(reasons) == 0 && r.opts.Capture == CaptureAll {
		reasons = append(reasons, ReasonAll)
	}

	return strings.Join(reasons, ",")
}

// Path は今回の実行のログファイルのパスを返します（まだ作られていないこともあります）。
func (r *Recorder) Path() string {
	if r == nil {
		return ""
	}

	return r.path
}

type metaLine struct {
	Type       string `json:"type"`
	Schema     int    `json:"schema"`
	Time       string `json:"time"`
	DsxVersion string `json:"dsx_version"`
	Command    string `json:"command"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	Lang       string `json:"lang"`
	LCAll      string `json:"lc_all"`
	NoColor    string `json:"no_color"`
	StdoutTTY  bool   `json:"stdout_tty"`
	EventsFile bool   `json:"events_file"`
	Capture    string `json:"capture"`
}

type captureLine struct {
	Type            string `json:"type"`
	Time            string `json:"time"`
	Manager         string `json:"manager"`
	Reason          string `json:"reason"`
	Command         string `json:"command"`
	ExitCode        *int   `json:"exit_code,omitempty"`
	Error           string `json:"error,omitempty"`
	Suspect         string `json:"suspect,omitempty"`
	ToolVersion     string `json:"tool_version,omitempty"`
	Output          string `json:"output"`
	OutputTruncated bool   `json:"output_truncated,omitempty"`
	InvalidUTF8     bool   `json:"invalid_utf8,omitempty"`
}

// Record は 1 件を今回の実行のログファイルへ追記します。reason は Reason が返した値です。
func (r *Recorder) Record(reason string, e *Entry) error {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	lines, err := r.buildLines(reason, e)
	if err != nil {
		return err
	}

	// 容量の上限は、ファイルを作る前に判定する（上限を超える記録で空のファイルを残さない）
	if r.written+int64(len(lines)) > r.opts.MaxTotalBytes {
		return ErrRunFileFull
	}

	if mkdirErr := os.MkdirAll(r.opts.Dir, dirPerm); mkdirErr != nil {
		return fmt.Errorf("診断ログの保存先を作成できません: %w", mkdirErr)
	}

	file, openErr := os.OpenFile(r.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, filePerm)
	if openErr != nil {
		return fmt.Errorf("診断ログを開けません: %w", openErr)
	}

	if _, writeErr := file.Write(lines); writeErr != nil {
		return errors.Join(fmt.Errorf("診断ログへ書き込めません: %w", writeErr), file.Close())
	}

	if closeErr := file.Close(); closeErr != nil {
		return fmt.Errorf("診断ログを閉じられません: %w", closeErr)
	}

	r.written += int64(len(lines))
	r.wroteMeta = true

	return r.prune()
}

// buildLines は追記する内容（最初の記録ならメタ行を含む）を JSON Lines で組み立てます。
func (r *Recorder) buildLines(reason string, e *Entry) ([]byte, error) {
	var out []byte

	if !r.wroteMeta {
		line, err := marshalLine(metaLine{
			Type:       "meta",
			Schema:     schemaVersion,
			Time:       r.now().UTC().Format(time.RFC3339Nano),
			DsxVersion: r.meta.DsxVersion,
			Command:    r.meta.Command,
			OS:         runtime.GOOS,
			Arch:       runtime.GOARCH,
			Lang:       os.Getenv("LANG"),
			LCAll:      os.Getenv("LC_ALL"),
			NoColor:    envState("NO_COLOR"),
			StdoutTTY:  r.meta.StdoutTTY,
			EventsFile: r.meta.EventsFile,
			Capture:    string(r.opts.Capture),
		})
		if err != nil {
			return nil, fmt.Errorf("診断ログのメタ情報を組み立てられません: %w", err)
		}

		out = append(out, line...)
	}

	capture := captureLine{
		Type:        "capture",
		Time:        r.now().UTC().Format(time.RFC3339Nano),
		Manager:     e.Manager,
		Reason:      reason,
		Command:     r.redactor.redact(strings.Join(e.Command, " ")),
		Suspect:     r.redactor.redact(e.Suspect),
		ToolVersion: r.redactor.redact(e.ToolVersion),
	}

	if e.Err != nil {
		capture.Error = r.redactor.redact(e.Err.Error())

		if exitErr, ok := errors.AsType[*exec.ExitError](e.Err); ok {
			code := exitErr.ExitCode()
			capture.ExitCode = &code
		}
	}

	output, truncated := truncateUTF8(e.Output, maxOutputBytes)
	capture.InvalidUTF8 = !utf8.Valid(output)
	capture.OutputTruncated = truncated
	capture.Output = r.redactor.redact(strings.ToValidUTF8(string(output), "�"))

	line, err := marshalLine(capture)
	if err != nil {
		return nil, fmt.Errorf("診断ログの記録を組み立てられません: %w", err)
	}

	return append(out, line...), nil
}

// marshalLine は 1 件を JSON Lines の 1 行（末尾の改行つき）にします。
// 伏せ字（<user> など）を読みやすいまま残すため、HTML 用のエスケープはしません。
func marshalLine(v any) ([]byte, error) {
	var buf bytes.Buffer

	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(v); err != nil {
		return nil, fmt.Errorf("JSON に変換できません: %w", err)
	}

	return buf.Bytes(), nil
}

// prune は、保持数と合計容量の上限を超えた古いログファイルを削除します（今回のファイルは削除しません）。
func (r *Recorder) prune() error {
	entries, err := os.ReadDir(r.opts.Dir)
	if err != nil {
		return fmt.Errorf("診断ログの保存先を読めません: %w", err)
	}

	type logFile struct {
		path string
		size int64
	}

	var files []logFile

	var total int64

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, filePrefix) || !strings.HasSuffix(name, fileSuffix) {
			continue
		}

		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}

		files = append(files, logFile{path: filepath.Join(r.opts.Dir, name), size: info.Size()})
		total += info.Size()
	}

	// ファイル名は先頭に UTC の時刻を持つため、名前順が古い順になる
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })

	var errs []error

	remaining := len(files)

	for _, file := range files {
		if remaining <= r.opts.MaxFiles && total <= r.opts.MaxTotalBytes {
			break
		}

		if file.path == r.path {
			continue
		}

		if removeErr := os.Remove(file.path); removeErr != nil {
			errs = append(errs, fmt.Errorf("古い診断ログを削除できません: %w", removeErr))

			continue
		}

		remaining--
		total -= file.size
	}

	return errors.Join(errs...)
}

func envState(key string) string {
	if _, ok := os.LookupEnv(key); ok {
		return "set"
	}

	return "unset"
}

// truncateUTF8 は data を maxBytes 以内に切り詰めます。途中で切れる多バイト文字は捨てます。
func truncateUTF8(data []byte, maxBytes int) ([]byte, bool) {
	if len(data) <= maxBytes {
		return data, false
	}

	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(data[cut]) {
		cut--
	}

	return data[:cut], true
}
