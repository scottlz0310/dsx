package main

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

func resolveTUIEnabled(requested bool) (enabled bool, warning string) {
	return resolveTUIEnabledByTerminal(requested, isTerminal(os.Stdout), isTerminal(os.Stderr))
}

func resolveTUIEnabledByTerminal(requested, stdoutTTY, stderrTTY bool) (enabled bool, warning string) {
	if !requested {
		return false, ""
	}

	if stdoutTTY && stderrTTY {
		return true, ""
	}

	return false, "⚠️  --tui は対話端末でのみ有効です。通常表示で続行します。"
}

func isTerminal(file *os.File) bool {
	return term.IsTerminal(int(file.Fd()))
}

func printTUIWarning(warning string) {
	if warning == "" {
		return
	}

	fmt.Fprintln(os.Stderr, warning)
}

func buildNoTargetTUIMessage(requested bool, commandName string) string {
	if !requested {
		return ""
	}

	return fmt.Sprintf("ℹ️  --tui が指定されましたが、%s の対象が0件のため TUI は起動しません。", commandName)
}

func printNoTargetTUIMessage(requested bool, commandName string) {
	message := buildNoTargetTUIMessage(requested, commandName)
	if message == "" {
		return
	}

	fmt.Fprintln(os.Stderr, message)
}
