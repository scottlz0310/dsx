#!/bin/sh
# gofmt でフォーマットされていないファイルを検出します。
# 使用法: sh scripts/gofmt-check.sh <file1> [file2] ...
if [ "$#" -eq 0 ]; then
    exit 0
fi

stdout_file="$(mktemp)"
stderr_file="$(mktemp)"
trap 'rm -f "$stdout_file" "$stderr_file"' EXIT HUP INT TERM

if ! gofmt -l "$@" >"$stdout_file" 2>"$stderr_file"; then
    if [ -s "$stderr_file" ]; then
        cat "$stderr_file" >&2
    else
        printf '%s\n' 'gofmt の実行に失敗しました。' >&2
    fi
    exit 1
fi

output="$(cat "$stdout_file")"
if [ -n "$output" ]; then
    printf '%s\n' "$output"
    exit 1
fi

exit 0
