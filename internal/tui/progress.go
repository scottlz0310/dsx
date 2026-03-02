// Package tui は Bubble Tea を使った進捗表示を提供します。
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/scottlz0310/dsx/internal/runner"
)

const (
	defaultBarWidth = 18
	maxLogLines     = 8
	maxBufferedLogs = 200
)

type jobState string

const (
	jobPending jobState = "pending"
	jobRunning jobState = "running"
	jobSuccess jobState = "success"
	jobFailed  jobState = "failed"
	jobSkipped jobState = "skipped"
)

type logLevel string

const (
	logInfo  logLevel = "info"
	logWarn  logLevel = "warn"
	logError logLevel = "error"
)

type jobProgress struct {
	Name      string
	State     jobState
	Duration  time.Duration
	Err       string
	StartedAt time.Time
}

type logEntry struct {
	At      time.Time
	Level   logLevel
	Message string
}

type runnerEventMsg struct {
	Event runner.Event
}

type completedMsg struct {
	Summary runner.Summary
}

type tickMsg time.Time

type model struct {
	title      string
	jobs       []jobProgress
	indexByJob map[string]int
	logs       []logEntry
	frame      int
	done       bool
	summary    runner.Summary
	startedAt  time.Time
}

var (
	styleTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	styleInfo    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	styleWarn    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleError   = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	styleSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	styleMuted   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

// RunJobProgress はジョブの実行進捗を Bubble Tea で表示し、実行結果を返します。
func RunJobProgress(ctx context.Context, title string, maxJobs int, jobs []runner.Job) (runner.Summary, error) {
	return RunJobProgressWithLogger(ctx, title, maxJobs, jobs, nil)
}

// RunJobProgressWithLogger はジョブの実行進捗を Bubble Tea で表示し、
// オプションでイベントをログファイルに記録します。
func RunJobProgressWithLogger(ctx context.Context, title string, maxJobs int, jobs []runner.Job, logger *runner.EventLogger) (runner.Summary, error) {
	m := newModel(title, jobs)
	program := tea.NewProgram(m, tea.WithContext(ctx))
	summaryCh := make(chan runner.Summary, 1)

	go func() {
		summary := runner.ExecuteWithEvents(ctx, maxJobs, jobs, func(event runner.Event) {
			program.Send(runnerEventMsg{Event: event})

			if logger != nil {
				logger.LogEvent(&event)
			}
		})

		publishCompletion(program, summaryCh, summary)
	}()

	_, runErr := program.Run()
	summary := <-summaryCh

	return summary, runErr
}

func publishCompletion(program *tea.Program, summaryCh chan<- runner.Summary, summary runner.Summary) {
	msg := completedMsg{Summary: summary}

	summaryCh <- summary

	program.Send(msg)
}

func newModel(title string, jobs []runner.Job) *model {
	progressJobs := make([]jobProgress, 0, len(jobs))
	indexByJob := make(map[string]int, len(jobs))

	for index, job := range jobs {
		name := job.Name
		if strings.TrimSpace(name) == "" {
			name = fmt.Sprintf("job-%d", index+1)
		}

		progressJobs = append(progressJobs, jobProgress{
			Name:  name,
			State: jobPending,
		})
		if _, exists := indexByJob[name]; !exists {
			indexByJob[name] = index
		}
	}

	return &model{
		title:      title,
		jobs:       progressJobs,
		indexByJob: indexByJob,
		logs:       make([]logEntry, 0, maxLogLines),
		startedAt:  time.Now(),
	}
}

func (m *model) Init() tea.Cmd {
	return tickCmd()
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tickMsg:
		if m.done {
			return m, nil
		}

		m.frame++

		return m, tickCmd()
	case runnerEventMsg:
		m.applyEvent(&typed.Event)
		return m, nil
	case completedMsg:
		m.done = true
		m.summary = typed.Summary
		m.appendLog(logInfo, "すべてのジョブが完了しました")
		return m, tea.Quit
	default:
		return m, nil
	}
}

// View は Bubble Tea Model インターフェースを実装します。
// v2 では戻り値が string から tea.View に変わったため、tea.NewView() でラップします。
func (m *model) View() tea.View {
	builder := strings.Builder{}
	builder.WriteString(styleTitle.Render(fmt.Sprintf("🖥️  %s", m.title)))
	builder.WriteString("\n")
	builder.WriteString(styleMuted.Render(fmt.Sprintf("経過: %s", time.Since(m.startedAt).Round(time.Second))))
	builder.WriteString("\n\n")

	success, failed, skipped, running := summarizeStates(m.jobs)
	fmt.Fprintf(&builder, "成功: %d  失敗: %d  スキップ: %d  実行中: %d  総数: %d", success, failed, skipped, running, len(m.jobs))
	builder.WriteString("\n\n")
	builder.WriteString("進捗:\n")

	for index, job := range m.jobs {
		percent := progressPercent(job.State, m.frame+index)
		bar := renderBar(percent, defaultBarWidth)
		status := renderStatus(&job)
		duration := renderDuration(job.Duration)

		fmt.Fprintf(&builder, "  %-24s %s %s %s\n", truncate(job.Name, 24), bar, status, duration)
	}

	builder.WriteString("\nログ:\n")

	if len(m.logs) == 0 {
		builder.WriteString(styleMuted.Render("  (ログはまだありません)"))
		builder.WriteString("\n")
	} else {
		for _, log := range tailLogs(m.logs, maxLogLines) {
			builder.WriteString(renderLog(log))
			builder.WriteString("\n")
		}
	}

	if m.done {
		builder.WriteString("\n")
		builder.WriteString(styleSuccess.Render(fmt.Sprintf("完了: 成功 %d / 失敗 %d / スキップ %d", m.summary.Success, m.summary.Failed, m.summary.Skipped)))
		builder.WriteString("\n")
	}

	return tea.NewView(builder.String())
}

func (m *model) applyEvent(event *runner.Event) {
	index := m.resolveJobIndex(event.JobIndex, event.JobName)
	if index < 0 || index >= len(m.jobs) {
		return
	}

	job := m.jobs[index]

	switch event.Type {
	case runner.EventQueued:
		job.State = jobPending
	case runner.EventStarted:
		job.State = jobRunning
		job.StartedAt = event.Timestamp
		m.appendLog(logInfo, fmt.Sprintf("開始: %s", event.JobName))
	case runner.EventFinished:
		job.Duration = event.Duration
		m.applyFinishedState(&job, event)
	}

	m.jobs[index] = job
}

func (m *model) applyFinishedState(job *jobProgress, event *runner.Event) {
	switch event.Status {
	case runner.StatusSuccess:
		job.State = jobSuccess

		m.appendLog(logInfo, fmt.Sprintf("完了: %s (%s)", event.JobName, event.Duration.Round(time.Millisecond)))
	case runner.StatusFailed:
		job.State = jobFailed
		if event.Err != nil {
			job.Err = event.Err.Error()
		}

		m.appendLog(logError, fmt.Sprintf("失敗: %s (%v)", event.JobName, event.Err))
	case runner.StatusSkipped:
		job.State = jobSkipped
		if event.Err != nil {
			job.Err = event.Err.Error()
			m.appendLog(logWarn, fmt.Sprintf("スキップ: %s (%v)", event.JobName, event.Err))
		} else {
			m.appendLog(logWarn, fmt.Sprintf("スキップ: %s", event.JobName))
		}
	default:
		job.State = jobFailed

		m.appendLog(logError, fmt.Sprintf("失敗: %s (不明な状態)", event.JobName))
	}
}

func (m *model) appendLog(level logLevel, message string) {
	m.logs = append(m.logs, logEntry{
		At:      time.Now(),
		Level:   level,
		Message: message,
	})

	if len(m.logs) > maxBufferedLogs {
		m.logs = m.logs[len(m.logs)-maxBufferedLogs:]
	}
}

func (m *model) resolveJobIndex(fallback int, name string) int {
	if fallback >= 0 && fallback < len(m.jobs) {
		return fallback
	}

	if index, ok := m.indexByJob[name]; ok {
		return index
	}

	return fallback
}

func summarizeStates(jobs []jobProgress) (success, failed, skipped, running int) {
	for _, job := range jobs {
		switch job.State {
		case jobSuccess:
			success++
		case jobFailed:
			failed++
		case jobSkipped:
			skipped++
		case jobRunning:
			running++
		}
	}

	return success, failed, skipped, running
}

func progressPercent(state jobState, frame int) float64 {
	switch state {
	case jobPending:
		return 0
	case jobRunning:
		phase := frame % 6
		return 0.2 + float64(phase)*0.1
	case jobSuccess, jobFailed, jobSkipped:
		return 1
	default:
		return 0
	}
}

func renderBar(percent float64, width int) string {
	switch {
	case percent < 0:
		percent = 0
	case percent > 1:
		percent = 1
	}

	filled := int(percent * float64(width))
	if filled > width {
		filled = width
	}

	if filled < 0 {
		filled = 0
	}

	return fmt.Sprintf("[%s%s]", strings.Repeat("=", filled), strings.Repeat("-", width-filled))
}

func renderStatus(job *jobProgress) string {
	switch job.State {
	case jobPending:
		return styleMuted.Render("待機中")
	case jobRunning:
		return styleInfo.Render("実行中")
	case jobSuccess:
		return styleSuccess.Render("成功")
	case jobSkipped:
		return styleWarn.Render("スキップ")
	case jobFailed:
		if job.Err == "" {
			return styleError.Render("失敗")
		}

		return styleError.Render("失敗: " + truncate(job.Err, 40))
	default:
		return styleMuted.Render("不明")
	}
}

func renderLog(entry logEntry) string {
	prefix := styleMuted.Render(entry.At.Format("15:04:05")) + " "

	switch entry.Level {
	case logInfo:
		return prefix + styleInfo.Render(entry.Message)
	case logWarn:
		return prefix + styleWarn.Render(entry.Message)
	case logError:
		return prefix + styleError.Render(entry.Message)
	default:
		return prefix + entry.Message
	}
}

func renderDuration(duration time.Duration) string {
	if duration <= 0 {
		return styleMuted.Render("-")
	}

	return styleMuted.Render(duration.Round(time.Millisecond).String())
}

func tailLogs(logs []logEntry, maxLines int) []logEntry {
	if len(logs) <= maxLines {
		return logs
	}

	return logs[len(logs)-maxLines:]
}

func tickCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(at time.Time) tea.Msg {
		return tickMsg(at)
	})
}

func truncate(s string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}

	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}

	if maxChars <= 1 {
		return "…"
	}

	return string(runes[:maxChars-1]) + "…"
}
