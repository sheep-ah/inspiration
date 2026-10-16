# 灵感（Inspiration）开发任务入口
#
# 常用：
#   make dev        本地开发（Wails 热重载）
#   make build      当前平台构建（注入版本信息）
#   make test       单元测试 + 端到端测试
#   make lint       格式检查与静态分析
#   make package-windows / package-macos  交叉打包（需 mingw-w64 / NSIS）

SHELL := /bin/bash

PKG      := github.com/sheep-ah/inspiration
BIN      := 灵感
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILT_AT ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -X $(PKG)/internal/version.Version=$(VERSION) \
           -X $(PKG)/internal/version.Commit=$(COMMIT) \
           -X $(PKG)/internal/version.BuildTime=$(BUILT_AT)

.PHONY: help dev build build-all test test-race ci lint fmt vet frontend clean package-windows dmg package-macos-amd64 package-macos-arm64 package-macos-universal selfcheck

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-24s\033[0m %s\n", $$1, $$2}'

dev: ## 本地开发（带热重载）
	wails dev

build: ## 构建当前平台（注入版本号）
	wails build -ldflags "$(LDFLAGS)"

build-all: ## 构建三个目标平台（当前机器交叉编译，仅用于本地验证）
	wails build -platform darwin/arm64 -ldflags "$(LDFLAGS)"
	wails build -platform darwin/amd64 -ldflags "$(LDFLAGS)"
	wails build -platform windows/amd64 -ldflags "$(LDFLAGS)"

selfcheck: build ## 构建后运行无界面自检
	./build/bin/$(BIN).app/Contents/MacOS/$(BIN) --selfcheck --data-dir /tmp/inspiration-selfcheck || \
	./build/bin/$(BIN).exe --selfcheck

test: ## 单元测试 + 端到端测试
	go test ./...

test-race: ## 竞态检测
	go test -race ./...

ci: lint ## 本地复现 CI 的全部检查（推送前跑一次）
	go test ./... -count=1 -timeout 15m
	@# 用 cp1252 模拟 Windows 控制台编码跑版本脚本（非 ASCII 输出会直接崩）。
	@# 注意在临时目录里跑，避免这个检查改写仓库里的 wails.json。
	@printf "  set-version.py (cp1252 控制台): "; \
	tmp=$$(mktemp -d); mkdir -p $$tmp/scripts; \
	cp scripts/set-version.py $$tmp/scripts/; \
	printf '{"info":{"productVersion":"0.0.0"}}' > $$tmp/wails.json; \
	if out=$$(PYTHONIOENCODING=cp1252 python3 $$tmp/scripts/set-version.py 0.1.0 2>&1); then \
		if grep -q '"productVersion": "0.1.0"' $$tmp/wails.json; then echo "✔"; \
		else echo "✘（版本未写入）"; rm -rf $$tmp; exit 1; fi; \
	else echo "✘"; echo "$$out" | tail -5; rm -rf $$tmp; exit 1; fi; \
	rm -rf $$tmp
	@# 以 Linux 视角再跑一次 staticcheck：避免"某个文件只在 macOS 编译，导致 Linux 上判定未使用"
	@printf "  GOOS=linux staticcheck: "; \
	if out=$$(GOOS=linux CGO_ENABLED=0 "$$(command -v staticcheck || echo "$$(go env GOPATH)/bin/staticcheck")" ./... 2>&1); then echo "✔"; \
	else echo "✘"; echo "$$out" | head -10; exit 1; fi
	@# 跨平台测试编译：防止"只在某个平台能编译"的测试代码（CI 上会直接 build failed）
	@for t in "linux amd64" "windows amd64" "darwin amd64"; do set -- $$t; \
		printf "  %s/%s 测试编译: " "$$1" "$$2"; \
		if out=$$(CGO_ENABLED=0 GOOS=$$1 GOARCH=$$2 go test -c -o /dev/null ./... 2>&1); then echo "✔"; \
		else echo "✘"; echo "$$out" | head -10; exit 1; fi; \
	done
	go test -race ./internal/docx/ ./internal/store/ -count=1
	go test ./internal/docx/ -bench . -benchtime 3x -run XXX
	@echo "✔ 本地 CI 检查全部通过"

vet: ## go vet
	go vet ./...

GO_FILES = $$(git ls-files --cached --others --exclude-standard '*.go' | grep -v '^poc/')

fmt: ## 格式化
	@files="$(GO_FILES)"; if [ -z "$$files" ]; then echo "没有需要格式化的 Go 文件"; exit 0; fi; \
	gofmt -w $$files && echo "✔ 已格式化"

lint: vet ## 格式与静态检查（gofmt + go vet + staticcheck，若已安装）
	@files="$(GO_FILES)"; \
	if [ -z "$$files" ]; then echo "（没有 Go 文件）"; else \
	  unformatted=$$(gofmt -l $$files); \
	  if [ -n "$$unformatted" ]; then echo "以下文件未格式化:"; echo "$$unformatted"; exit 1; fi; \
	  echo "✔ gofmt 检查通过（$$(echo $$files | wc -w | tr -d ' ') 个文件）"; \
	fi
	@if command -v staticcheck >/dev/null 2>&1 || [ -x "$$(go env GOPATH)/bin/staticcheck" ]; then \
		sc=$$(command -v staticcheck || echo "$$(go env GOPATH)/bin/staticcheck"); \
		"$$sc" ./... && echo "✔ staticcheck 检查通过"; \
	else echo "（未安装 staticcheck，跳过；安装：go install honnef.co/go/tools/cmd/staticcheck@latest）"; fi

frontend: ## 仅构建前端
	cd frontend && npm ci && npm run build

clean: ## 清理构建产物
	rm -rf build/bin frontend/dist/assets

package-windows: ## Windows x64 NSIS 安装包（需 mingw-w64 + nsis）
	wails build -platform windows/amd64 -nsis -webview2 download -ldflags "$(LDFLAGS)"

dmg: ## 打包当前平台的 dmg（内含「应用程序」拖拽别名，挂载自检通过才产出）
	wails build -ldflags "$(LDFLAGS)"
	bash scripts/make-dmg.sh "build/bin/$(BIN).app" "dist/inspiration_$(VERSION)_darwin_$(shell go env GOARCH).dmg"

package-macos-amd64: ## macOS Intel 打包（.app + dmg）
	wails build -platform darwin/amd64 -ldflags "$(LDFLAGS)"
	bash scripts/make-dmg.sh "build/bin/$(BIN).app" "dist/inspiration_$(VERSION)_darwin_amd64.dmg"

package-macos-arm64: ## macOS Apple Silicon 打包（.app + dmg）
	wails build -platform darwin/arm64 -ldflags "$(LDFLAGS)"
	bash scripts/make-dmg.sh "build/bin/$(BIN).app" "dist/inspiration_$(VERSION)_darwin_arm64.dmg"

package-macos-universal: ## macOS 通用包（仅 .app）
	wails build -platform darwin/universal -ldflags "$(LDFLAGS)"
