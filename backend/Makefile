# CosTalk Backend Makefile

# 变量定义
APP_NAME := costalk
API_DIR := api
PROTO_DIR := proto
CMD_DIR := cmd
PKG_DIR := pkg
BUILD_DIR := build
GOCTL := goctl

# Go 相关变量
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)
CGO_ENABLED := 0

# 版本信息
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date '+%Y-%m-%d %H:%M:%S')
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# 编译标志
LDFLAGS := -X 'main.Version=$(VERSION)' \
           -X 'main.BuildTime=$(BUILD_TIME)' \
           -X 'main.Commit=$(COMMIT)' \
           -s -w

.PHONY: help gen clean build run test docker deps fmt lint

# 默认目标
all: gen build

# 帮助信息
help:
	@echo "CosTalk Backend Build Commands:"
	@echo ""
	@echo "  gen      - 生成 go-zero 代码（基于 API 定义）"
	@echo "  build    - 编译应用程序"
	@echo "  run      - 运行应用程序"
	@echo "  test     - 运行测试"
	@echo "  clean    - 清理生成的文件"
	@echo "  deps     - 下载依赖"
	@echo "  fmt      - 格式化代码" 
	@echo "  lint     - 代码检查"
	@echo "  docker   - 构建 Docker 镜像"
	@echo ""

# 生成 go-zero 代码
gen:
	@echo "==> 生成 go-zero API 代码..."
	@if [ ! -f "$(API_DIR)/$(APP_NAME).api" ]; then \
		echo "错误: $(API_DIR)/$(APP_NAME).api 文件不存在"; \
		exit 1; \
	fi
	cd $(API_DIR) && $(GOCTL) api go -api $(APP_NAME).api -dir .. --style goZero
	@echo "==> 代码生成完成"

# 下载依赖
deps:
	@echo "==> 下载 Go 依赖..."
	go mod tidy
	go mod download

# 编译
build: deps
	@echo "==> 编译应用程序..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(APP_NAME) \
		$(APP_NAME).go

# 运行
run: gen
	@echo "==> 运行应用程序..."
	go run $(APP_NAME).go -f etc/$(APP_NAME)-api.yaml

# 运行测试
test:
	@echo "==> 运行测试..."
	go test -v ./...

# 基准测试
benchmark:
	@echo "==> 运行基准测试..."
	go test -bench=. -benchmem ./...

# 格式化代码
fmt:
	@echo "==> 格式化代码..."
	go fmt ./...
	goimports -w .

# 代码检查
lint:
	@echo "==> 代码检查..."
	golangci-lint run

# 清理
clean:
	@echo "==> 清理生成的文件..."
	rm -rf $(BUILD_DIR)
	rm -rf internal
	rm -rf types
	rm -f $(APP_NAME).go

# 重新生成（先清理再生成）
regen: clean gen

# 开发模式（监听文件变化自动重启）
dev:
	@echo "==> 开发模式启动..."
	@which air > /dev/null || (echo "请安装 air: go install github.com/cosmtrek/air@latest" && exit 1)
	air

# 安装开发工具
install-tools:
	@echo "==> 安装开发工具..."
	go install github.com/zeromicro/go-zero/tools/goctl@latest
	go install github.com/cosmtrek/air@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Docker 构建
docker:
	@echo "==> 构建 Docker 镜像..."
	docker build -t $(APP_NAME):$(VERSION) .
	docker tag $(APP_NAME):$(VERSION) $(APP_NAME):latest

# Docker 运行
docker-run:
	@echo "==> 运行 Docker 容器..."
	docker run -p 8888:8888 $(APP_NAME):latest

# 查看版本信息
version:
	@echo "App Name: $(APP_NAME)"
	@echo "Version: $(VERSION)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Commit: $(COMMIT)"
	@echo "GOOS: $(GOOS)"
	@echo "GOARCH: $(GOARCH)"

# 生产环境部署准备
deploy-prep: clean gen test build
	@echo "==> 生产环境部署准备完成"
	@echo "==> 二进制文件: $(BUILD_DIR)/$(APP_NAME)"