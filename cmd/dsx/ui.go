package main

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

type tuiRequestSource int

const (
	tuiSourceNone tuiRequestSource = iota
	tuiSourceFlag
	tuiSourceConfig
)

type tuiRequest struct {
	Requested bool
	Source    tuiRequestSource
}

func resolveTUIRequest(configDefault, tuiFlagChanged, tuiFlagValue, noTUIFlagChanged, noTUIFlagValue bool) (tuiRequest, error) {
	if tuiFlagChanged && tuiFlagValue && noTUIFlagChanged && noTUIFlagValue {
		return tuiRequest{}, fmt.Errorf("--tui と --no-tui は同時指定できません")
	}

	if noTUIFlagChanged && noTUIFlagValue {
		return tuiRequest{Requested: false, Source: tuiSourceFlag}, nil
	}

	if tuiFlagChanged {
		return tuiRequest{Requested: tuiFlagValue, Source: tuiSourceFlag}, nil
	}

	if configDefault {
		return tuiRequest{Requested: true, Source: tuiSourceConfig}, nil
	}

	return tuiRequest{Requested: false, Source: tuiSourceNone}, nil
}

func resolveTUIEnabled(request tuiRequest) (enabled bool, warning string) {
	return resolveTUIEnabledByTerminal(request, isTerminal(os.Stdout), isTerminal(os.Stderr))
}

func resolveTUIEnabledByTerminal(request tuiRequest, stdoutTTY, stderrTTY bool) (enabled bool, warning string) {
	if !request.Requested {
		return false, ""
	}

	if stdoutTTY && stderrTTY {
		return true, ""
	}

	return false, buildTUIUnavailableWarning(request.Source)
}

func isTerminal(file *os.File) bool {
	fd := file.Fd()
	if fd > uintptr(^uint(0)>>1) {
		return false
	}

	return term.IsTerminal(int(fd))
}

func printTUIWarning(warning string) {
	if warning == "" {
		return
	}

	fmt.Fprintln(os.Stderr, warning)
}

func buildTUIUnavailableWarning(source tuiRequestSource) string {
	switch source {
	case tuiSourceFlag:
		return "⚠️  --tui は対話端末でのみ有効です。通常表示で続行します。"
	case tuiSourceConfig:
		return "⚠️  設定 (ui.tui) により TUI が有効ですが、対話端末ではないため通常表示で続行します。"
	default:
		return "⚠️  TUI 進捗表示は対話端末でのみ有効です。通常表示で続行します。"
	}
}

func buildNoTargetTUIMessage(request tuiRequest, commandName string) string {
	if !request.Requested {
		return ""
	}

	switch request.Source {
	case tuiSourceFlag:
		return fmt.Sprintf("ℹ️  --tui が指定されましたが、%s の対象が0件のため TUI は起動しません。", commandName)
	case tuiSourceConfig:
		return fmt.Sprintf("ℹ️  設定 (ui.tui) により TUI が有効ですが、%s の対象が0件のため TUI は起動しません。", commandName)
	default:
		return fmt.Sprintf("ℹ️  %s の対象が0件のため TUI は起動しません。", commandName)
	}
}

func printNoTargetTUIMessage(request tuiRequest, commandName string) {
	message := buildNoTargetTUIMessage(request, commandName)
	if message == "" {
		return
	}

	fmt.Fprintln(os.Stderr, message)
}
