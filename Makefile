# DevSync 開発環境Makefile
#
# 使用方法:
#   make help     - 利用可能なコマンド一覧
#   make check    - 全品質チェック実行（CI相当）
#   make test     - テスト実行
#   make coverage - カバレッジレポート生成

.PHONY: help build test test-verbose coverage lint vet fmt check clean install

# デフォルトターゲット
.DEFAULT_GOAL := help

# 変数定義
BINARY_NAME := devsync
COVERAGE_FILE := coverage.out
COVERAGE_HTML := coverage.html
COVERAGE_THRESHOLD := 50
GO_PACKAGES := ./...

# Windows対応
ifeq ($(OS),Windows_NT)
    BINARY_NAME := devsync.exe
    RM := del /Q
    RMDIR := rmdir /S /Q
else
    RM := rm -f
    RMDIR := rm -rf
endif

## help: 利用可能なコマンドを表示
help:
	@echo "DevSync 開発コマンド"
	@echo ""
	@echo "ビルド:"
	@echo "  make build       - バイナリをビルド"
	@echo "  make install     - バイナリをインストール"
	@echo "  make clean       - ビルド成果物を削除"
	@echo ""
	@echo "テスト:"
	@echo "  make test        - 全テストを実行"
	@echo "  make test-verbose- 詳細出力でテスト実行"
	@echo "  make coverage    - カバレッジレポート生成"
	@echo "  make coverage-check - カバレッジ閾値チェック ($(COVERAGE_THRESHOLD)%)"
	@echo ""
	@echo "品質チェック:"
	@echo "  make fmt         - コードフォーマット (gofmt)"
	@echo "  make vet         - 静的解析 (go vet)"
	@echo "  make lint        - リンター実行 (golangci-lint)"
	@echo "  make check       - 全品質チェック (CI相当)"
	@echo ""
	@echo "開発サイクル:"
	@echo "  make dev         - フォーマット→テスト→ビルド"
	@echo "  make pre-commit  - コミット前チェック (fmt + vet + test)"

## build: バイナリをビルド
build:
	go build -o $(BINARY_NAME) ./cmd/devsync

## install: バイナリをインストール
install:
	go install ./cmd/devsync

## clean: ビルド成果物を削除
clean:
	$(RM) $(BINARY_NAME) $(COVERAGE_FILE) $(COVERAGE_HTML)

## test: 全テストを実行
test:
	go test $(GO_PACKAGES) -race -shuffle=on

## test-verbose: 詳細出力でテスト実行
test-verbose:
	go test $(GO_PACKAGES) -race -shuffle=on -v

## coverage: カバレッジレポート生成
coverage:
	go test $(GO_PACKAGES) -race -coverprofile=$(COVERAGE_FILE) -covermode=atomic
	go tool cover -func=$(COVERAGE_FILE)
	go tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo ""
	@echo "HTMLレポート: $(COVERAGE_HTML)"

## coverage-check: カバレッジ閾値チェック
coverage-check: coverage
	@echo ""
	@echo "カバレッジ閾値チェック (最低 $(COVERAGE_THRESHOLD)%)..."
	@go tool cover -func=$(COVERAGE_FILE) | grep total | awk '{print $$3}' | sed 's/%//' | \
		xargs -I {} sh -c 'if [ $$(echo "{} < $(COVERAGE_THRESHOLD)" | bc) -eq 1 ]; then \
			echo "❌ カバレッジが閾値未満: {}% < $(COVERAGE_THRESHOLD)%"; exit 1; \
		else \
			echo "✅ カバレッジOK: {}% >= $(COVERAGE_THRESHOLD)%"; \
		fi'

## fmt: コードフォーマット
fmt:
	gofmt -s -w .
	@echo "✅ フォーマット完了"

## fmt-check: フォーマットチェック（CIモード）
fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "❌ 以下のファイルがフォーマットされていません:"; gofmt -l .; exit 1)
	@echo "✅ フォーマットOK"

## vet: 静的解析
vet:
	go vet $(GO_PACKAGES)
	@echo "✅ go vet 完了"

## lint: リンター実行
lint:
	@which golangci-lint > /dev/null 2>&1 || (echo "golangci-lint をインストールしてください: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; exit 1)
	golangci-lint run $(GO_PACKAGES)
	@echo "✅ lint 完了"

## check: 全品質チェック（CI相当）
check: fmt-check vet test coverage-check
	@echo ""
	@echo "========================================="
	@echo "✅ 全品質チェック完了"
	@echo "========================================="

## dev: 開発サイクル（フォーマット→テスト→ビルド）
dev: fmt test build
	@echo ""
	@echo "✅ 開発ビルド完了: ./$(BINARY_NAME)"

## pre-commit: コミット前チェック
pre-commit: fmt vet test
	@echo ""
	@echo "✅ コミット前チェック完了"
