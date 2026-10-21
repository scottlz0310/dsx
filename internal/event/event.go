// Package event は dsx の実行結果を機械可読な JSON Lines として出力します。
//
// 人が読む標準出力・標準エラーは変えず、別の出力先（--events-file）へ
// 1 行 1 イベントで書き出します。Arbor など外部のツールは、このイベントから
// 警告・スキップ・失敗を、テキストの解析なしに取得できます。
//
// スキーマは SchemaVersion で版管理します。互換性のない変更は版を上げます。
package event

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// SchemaVersion はイベントのスキーマ版です。すべてのイベントの schema フィールドに入ります。
const SchemaVersion = 1

// イベントの種別（type フィールド）。
const (
	typeStart       = "start"
	typeJobStarted  = "job_started"
	typeJobFinished = "job_finished"
	typeWarning     = "warning"
	typeSummary     = "summary"
)

// Kind はジョブの種別です。
type Kind string

const (
	// KindManager はパッケージマネージャの更新です。
	KindManager Kind = "manager"
	// KindManagerSelfUpdate はパッケージマネージャ本体の更新です。
	KindManagerSelfUpdate Kind = "manager_self_update"
	// KindRepo はリポジトリの更新です。
	KindRepo Kind = "repo"
	// KindPhase は sys・repo の段階そのものの失敗です（root が不正、設定エラーなど。個々のジョブより前に失敗した原因が入ります）。
	KindPhase Kind = "phase"
)

// Status はジョブの結果です。
// failed は終了コード 1 の原因になるもの、skipped は実行しなかった（できなかった）ものです。
type Status string

const (
	StatusSuccess Status = "success"
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped"
)

// Code は failed / skipped の理由を表す機械可読な識別子です。
type Code string

const (
	// CodeError は更新に失敗したことを表します。
	CodeError Code = "error"
	// CodeUnavailable はマネージャが未インストールまたは利用不可で実行しなかったことを表します。
	CodeUnavailable Code = "unavailable"
	// CodeCancelled はキャンセルまたはタイムアウトで実行しなかった（完了しなかった）ことを表します。
	CodeCancelled Code = "cancelled"
	// CodePullSkipped は pull を実行しなかったことを表します（未コミットの変更、upstream なし等）。
	CodePullSkipped Code = "pull_skipped"
	// CodeInUse は更新対象が実行中で置き換えられないため、更新を見送ったことを表します（Windows の uv 本体更新等）。
	CodeInUse Code = "in_use"
)

// 警告の分類（warning イベントの category フィールド）。
const (
	// CategoryManagerUnknown は設定で指定されたマネージャが dsx に存在しないことを表します。
	CategoryManagerUnknown = "manager_unknown"
	// CategoryManagerConfig はマネージャ固有の設定を適用できず、マネージャを有効化できなかったことを表します。
	CategoryManagerConfig = "manager_config"
	// CategoryRepoWarning は pull 実行後のリポジトリの警告（BEHIND 残存等）を表します。
	CategoryRepoWarning = "repo_warning"
	// CategoryConfigFallback は設定ファイルを読めず、既定の設定で続行したことを表します。
	CategoryConfigFallback = "config_fallback"
	// CategoryGitHubRateLimit は GitHub のレート制限でリポジトリ一覧の補完を省略したことを表します。
	CategoryGitHubRateLimit = "github_rate_limit"
)

// JobResult はジョブの結果です。
type JobResult struct {
	Status  Status
	Code    Code   // success のときは空
	Reason  string // 人が読む理由。success のときは空
	Updated int    // 更新したパッケージ数
}

// Issue は要約に載る、failed / skipped のジョブです。
type Issue struct {
	Phase  string `json:"phase"`
	Kind   Kind   `json:"kind"`
	Name   string `json:"name"`
	Status Status `json:"status"`
	Code   Code   `json:"code"`
	Reason string `json:"reason"`
}

// WarningItem は要約に載る警告です。
type WarningItem struct {
	Phase    string `json:"phase"`
	Category string `json:"category"`
	Subject  string `json:"subject"`
	Message  string `json:"message"`
}

// JobCounts は要約のジョブ件数です。
type JobCounts struct {
	Success int `json:"success"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

type header struct {
	Schema int    `json:"schema"`
	Type   string `json:"type"`
	Time   string `json:"time"`
}

type startEvent struct {
	header
	Command string `json:"command"`
	Version string `json:"version"`
}

type jobStartedEvent struct {
	header
	Phase string `json:"phase"`
	Kind  Kind   `json:"kind"`
	Name  string `json:"name"`
}

type jobFinishedEvent struct {
	header
	Phase      string `json:"phase"`
	Kind       Kind   `json:"kind"`
	Name       string `json:"name"`
	Status     Status `json:"status"`
	Code       Code   `json:"code,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Updated    int    `json:"updated"`
	DurationMs int64  `json:"duration_ms"`
}

type warningEvent struct {
	header
	WarningItem
}

type summaryEvent struct {
	header
	Command  string        `json:"command"`
	OK       bool          `json:"ok"`
	Error    string        `json:"error,omitempty"`
	Jobs     JobCounts     `json:"jobs"`
	Updated  int           `json:"updated"`
	Warnings int           `json:"warnings"`
	Issues   []Issue       `json:"issues"`
	Items    []WarningItem `json:"warning_items"`
}

// Stream はイベントの書き出し先です。
// nil の *Stream はすべての操作が何もしないため、イベント出力が無効なときも呼び出し側は分岐不要です。
// 書き込みの失敗は最初の 1 件を保持し、Finish が返します（実行そのものは止めません）。
type Stream struct {
	mu       sync.Mutex
	w        io.Writer
	now      func() time.Time
	command  string
	phase    string
	started  map[string]time.Time
	counts   JobCounts
	updated  int
	issues   []Issue
	warnings []WarningItem
	writeErr error
}

// New は Stream を作り、start イベントを書き込みます。now が nil のときは time.Now を使います。
func New(w io.Writer, command, version string, now func() time.Time) *Stream {
	if now == nil {
		now = time.Now
	}

	s := &Stream{
		w:        w,
		now:      now,
		command:  command,
		started:  make(map[string]time.Time),
		issues:   []Issue{},
		warnings: []WarningItem{},
	}

	s.write(&startEvent{header: s.header(typeStart), Command: command, Version: version})

	return s
}

// SetPhase は以降のイベントに付ける段階名（"sys"・"repo"）を設定します。
func (s *Stream) SetPhase(phase string) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.phase = phase
}

// JobStarted はジョブの開始を記録します。
func (s *Stream) JobStarted(kind Kind, name string) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.started[s.key(kind, name)] = s.now()
	s.write(&jobStartedEvent{header: s.header(typeJobStarted), Phase: s.phase, Kind: kind, Name: name})
}

// JobFinished はジョブの完了を記録します。JobStarted を呼んでいれば、所要時間も記録します。
func (s *Stream) JobFinished(kind Kind, name string, r JobResult) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.finishLocked(kind, name, r)
}

// SkipUnstarted は、まだ開始していないジョブを skipped として記録します。
// キャンセルやタイムアウトで実行されなかったジョブを、取りこぼさず要約に載せるために使います。
func (s *Stream) SkipUnstarted(kind Kind, names []string, code Code, reason string) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, name := range names {
		if _, ok := s.started[s.key(kind, name)]; ok {
			continue
		}

		s.finishLocked(kind, name, JobResult{Status: StatusSkipped, Code: code, Reason: reason})
	}
}

// Warn は警告を記録します。警告は終了コードに影響しません。
func (s *Stream) Warn(category, subject, message string) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	item := WarningItem{Phase: s.phase, Category: category, Subject: subject, Message: message}
	s.warnings = append(s.warnings, item)
	s.write(&warningEvent{header: s.header(typeWarning), WarningItem: item})
}

// Finish は summary イベントを書き込み、書き込みで最初に起きたエラーを返します。
// runErr はコマンドが返すエラーで、nil のとき ok が true になります。
func (s *Stream) Finish(runErr error) error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ev := &summaryEvent{
		header:   s.header(typeSummary),
		Command:  s.command,
		OK:       runErr == nil,
		Jobs:     s.counts,
		Updated:  s.updated,
		Warnings: len(s.warnings),
		Issues:   s.issues,
		Items:    s.warnings,
	}
	if runErr != nil {
		ev.Error = runErr.Error()
	}

	s.write(ev)

	return s.writeErr
}

func (s *Stream) finishLocked(kind Kind, name string, r JobResult) {
	var duration time.Duration

	key := s.key(kind, name)
	if startedAt, ok := s.started[key]; ok {
		duration = s.now().Sub(startedAt)
	}

	// 開始済みとして残し、同じジョブを SkipUnstarted が二重に記録しないようにする。
	s.started[key] = s.now()

	switch r.Status {
	case StatusSuccess:
		s.counts.Success++
	case StatusFailed:
		s.counts.Failed++
	case StatusSkipped:
		s.counts.Skipped++
	}

	s.updated += r.Updated

	if r.Status != StatusSuccess {
		s.issues = append(s.issues, Issue{
			Phase: s.phase, Kind: kind, Name: name, Status: r.Status, Code: r.Code, Reason: r.Reason,
		})
	}

	s.write(&jobFinishedEvent{
		header:     s.header(typeJobFinished),
		Phase:      s.phase,
		Kind:       kind,
		Name:       name,
		Status:     r.Status,
		Code:       r.Code,
		Reason:     r.Reason,
		Updated:    r.Updated,
		DurationMs: duration.Milliseconds(),
	})
}

func (s *Stream) key(kind Kind, name string) string {
	return s.phase + "\x00" + string(kind) + "\x00" + name
}

func (s *Stream) header(eventType string) header {
	return header{Schema: SchemaVersion, Type: eventType, Time: s.now().UTC().Format(time.RFC3339Nano)}
}

// write は 1 イベントを 1 行で書き込みます。s.mu を保持して呼びます（New からは生成直後で競合しません）。
func (s *Stream) write(ev any) {
	if s.writeErr != nil {
		return
	}

	line, err := json.Marshal(ev)
	if err != nil {
		s.writeErr = fmt.Errorf("イベントの JSON 化に失敗: %w", err)

		return
	}

	if _, err := s.w.Write(append(line, '\n')); err != nil {
		s.writeErr = fmt.Errorf("イベントファイルへの書き込みに失敗: %w", err)
	}
}
