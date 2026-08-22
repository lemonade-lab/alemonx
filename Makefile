.PHONY: help dev bundle-resources build test test-agent test-all test-sqlite test-space format lint dev-fe build-frontend test-sse verify-sse release-check docker-build docker-buildx docker-buildx-push docker-base-build docker-base-buildx docker-base-buildx-push docker-up docker-down docker-logs yunzai-resources docker-yunzai-build docker-yunzai-buildx docker-yunzai-buildx-push docker-yunzai-up docker-yunzai-down docker-yunzai-logs

.DEFAULT_GOAL := help

ALX_RUNTIME_BASE ?= ccr.ccs.tencentyun.com/ningmengchongshui/alemonbase:latest
YUNZAI_LOADER_SOURCE ?= ../alemonjs-load-yunzai
YUNZAI_IMAGE ?= alemonx-yunzai:local
YUNZAI_BASE_IMAGE ?= alemonx:local

help: ## Show available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

dev: ## Start the setup guide (replaces an older local ALemonX backend)
	@set -e; \
	port=17390; \
	pids="$$(lsof -tiTCP:$$port -sTCP:LISTEN 2>/dev/null || true)"; \
	for pid in $$pids; do \
		command="$$(ps -p "$$pid" -o comm= | xargs basename)"; \
		if [ "$$command" != "alemonx" ] && [ "$$command" != "app" ]; then \
			echo "端口 $$port 已被 $$command (PID $$pid) 占用；为避免误杀，未停止该进程。"; \
			exit 1; \
		fi; \
		echo "停止旧 ALemonX 后端（PID $$pid，端口 $$port）…"; \
		kill "$$pid"; \
	done; \
	for attempt in 1 2 3 4 5; do \
		[ -z "$$(lsof -tiTCP:$$port -sTCP:LISTEN 2>/dev/null || true)" ] && break; \
		sleep 1; \
	done; \
	if [ -n "$$(lsof -tiTCP:$$port -sTCP:LISTEN 2>/dev/null || true)" ]; then \
		echo "旧 ALemonX 后端未能在预期时间内退出，取消启动。"; \
		exit 1; \
	fi; \
	go run .

bundle-resources: ## Install the embedded Yarn package before building
	@set -e; \
	for dir in resources/packages/*; do \
		[ -f "$$dir/package.json" ] || continue; \
		[ -f "$$dir/package-lock.json" ] || { echo "缺少 $$dir/package-lock.json：请先执行 npm install --package-lock-only 并提交锁文件" >&2; exit 1; }; \
		echo "Bundling Yarn in $$dir"; \
		(cd "$$dir" && npm ci --no-bin-links --ignore-scripts --no-audit --no-fund); \
	done

build: bundle-resources build-fe ## Build the production binary
	go build -o app .

test: ## Run Go tests
	go test ./internal/...

test-space: ## Fail early when the Go test temporary volume is too small
	@tmp="$${ALX_TEST_TMPDIR:-$$(mktemp -d)}"; \
	min="$${ALX_TEST_MIN_KB:-1048576}"; \
	available="$$(df -Pk "$$tmp" | awk 'NR==2 {print $$4}')"; \
	if [ -z "$$available" ] || [ "$$available" -lt "$$min" ]; then \
		echo "测试临时目录空间不足：$$tmp 仅剩 $${available:-0} KiB，需要至少 $$min KiB。设置 ALX_TEST_TMPDIR 指向有足够空间的卷。"; \
		exit 1; \
	fi

test-agent: test-space ## Run Agent package tests
	@tmp="$${ALX_TEST_TMPDIR:-$$(mktemp -d)}"; \
	GOTMPDIR="$$tmp" GOCACHE="$${GOCACHE:-$$(mktemp -d)}" go test ./internal/agent ./internal/web

test-all: test-space ## Run the complete Go test suite with injectable test storage
	@tmp="$${ALX_TEST_TMPDIR:-$$(mktemp -d)}"; \
	ALX_TEST_CACHE_DIR="$${ALX_TEST_CACHE_DIR:-$$(mktemp -d)}" GOTMPDIR="$$tmp" GOCACHE="$${GOCACHE:-$$(mktemp -d)}" go test ./...

test-sqlite: test-space ## Run the complete suite using the production SQLite repository
	@tmp="$${ALX_TEST_TMPDIR:-$$(mktemp -d)}"; \
	ALX_OPS_STORAGE=sqlite ALX_OPS_SQLITE_PATH="$$tmp/ops.db" ALX_TEST_CACHE_DIR="$${ALX_TEST_CACHE_DIR:-$$(mktemp -d)}" GOTMPDIR="$$tmp" GOCACHE="$${GOCACHE:-$$(mktemp -d)}" go test ./...

format: ## Format Go files
	go fmt ./...

lint: ## Run Go vet and frontend lint
	GOCACHE="$${GOCACHE:-$$(mktemp -d)}" go vet ./internal/...
	cd frontend && yarn lint

dev-fe: ## Start the Vite development server
	cd frontend && yarn dev

build-fe: ## Build the frontend into dist/
	cd frontend && yarn build

build-frontend: build-fe ## Alias for the release gate

test-sse: ## Run Chromium multi-tab SSE coordination tests
	cd frontend && yarn test:sse

verify-sse: ## Run the SSE reliability gate
	ALX_TEST_CACHE_DIR="$${ALX_TEST_CACHE_DIR:-$$(mktemp -d)}" go test ./...
	go test ./internal/web -run '^$$' -bench BenchmarkOperationOutputBatching -benchmem -benchtime=100x -count=1
	cd frontend && yarn lint
	cd frontend && yarn build
	$(MAKE) test-sse
	git diff --check

release-check: ## Run the publishability gate
	$(MAKE) test-agent
	$(MAKE) test-all
	$(MAKE) test-sqlite
	@tmp="$${ALX_TEST_TMPDIR:-$$(mktemp -d)}"; \
	GOTMPDIR="$$tmp" GOCACHE="$${GOCACHE:-$$(mktemp -d)}" go test -race ./internal/agent ./internal/web
	cd frontend && yarn lint
	$(MAKE) build-frontend
	git diff --check

docker-base-build: ## Build the runtime base image locally
	./scripts/docker-base-build.sh

docker-base-buildx: ## Validate the multi-architecture alemonbase image
	./scripts/docker-base-buildx.sh

docker-base-buildx-push: ## Build and publish the multi-architecture alemonbase image
	ALX_BASE_PUSH=1 ./scripts/docker-base-buildx.sh

docker-build: ## Build the local Docker image
	docker build --build-arg ALX_RUNTIME_BASE=$(ALX_RUNTIME_BASE) --build-arg VERSION=$$(git describe --tags --always --dirty 2>/dev/null || echo dev) -t alemonx:local .

docker-buildx: ## Manually validate or publish the multi-architecture Docker image
	./scripts/docker-buildx.sh

docker-buildx-push: ## Manually validate or publish the multi-architecture Docker image
	ALX_PUSH=1 ./scripts/docker-buildx.sh

yunzai-resources: ## Package the prebuilt Yunzai loader and refresh local resource checksums
	sh ./scripts/docker-yunzai-package-loader.sh "$(YUNZAI_LOADER_SOURCE)" .resources/alemonjs-load-yunzai.tar.gz
	cd .resources && shasum -a 256 Miao-Yunzai-master.zip miao-plugin-master.zip alemonjs-load-yunzai.tar.gz > yunzai-resources.sha256

docker-yunzai-build: yunzai-resources docker-build ## Build the local Yunzai image without cloning Yunzai repositories
	# Docker Desktop BuildKit resolves an unqualified local base through a registry.
	# Use the native image store here so the just-built alemonx:local is reusable.
	DOCKER_BUILDKIT=0 docker build --build-arg BASE_IMAGE=$(YUNZAI_BASE_IMAGE) -f Dockerfile.yunzai -t $(YUNZAI_IMAGE) .

docker-yunzai-buildx: yunzai-resources ## Validate the multi-architecture Yunzai image build
	sh ./scripts/docker-yunzai-buildx.sh

docker-yunzai-buildx-push: yunzai-resources ## Build and push the multi-architecture Yunzai image
	YUNZAI_PUSH=1 sh ./scripts/docker-yunzai-buildx.sh
