#!/bin/sh
# gofmt でフォーマットされていないファイルを検出します。
# 使用法: sh scripts/gofmt-check.sh <file1> [file2] ...
output="$(gofmt -l "$@")"
if [ -n "$output" ]; then
    printf '%s\n' "$output"
    exit 1
fi
