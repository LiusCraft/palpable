.PHONY: gen fmt-check build lint test vet cover check dev-server dev-web

# PR 要求的后端单测覆盖率下限
MIN_COVERAGE ?= 90

# 由 protocol/palpable.schema.json 生成前端协议类型
gen:
	cd web && pnpm run gen:protocol

fmt-check:
	@files="$$(gofmt -l server)"; \
	if [ -n "$$files" ]; then \
		echo "以下 Go 文件未经过 gofmt:"; \
		echo "$$files"; \
		exit 1; \
	fi

build:
	cd server && go build ./...
	cd web && pnpm run build

lint:
	cd web && pnpm run lint

test:
	cd server && go test ./...

vet:
	cd server && go vet ./...

# 统计 server/internal 的覆盖率；低于 MIN_COVERAGE 则失败。
# cmd/palpabled 只是装配入口，不纳入统计。
cover:
	cd server && go test -covermode=atomic -coverpkg=./internal/... -coverprofile=coverage.out ./internal/...
	cd server && ../.github/scripts/check-coverage.sh coverage.out $(MIN_COVERAGE)

# 每次改动后的完整检查
check: gen fmt-check lint vet cover build

dev-server:
	cd server && go run ./cmd/palpabled

dev-web:
	cd web && pnpm run dev
