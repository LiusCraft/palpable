.PHONY: gen build lint test vet cover check dev-server dev-web

# 由 protocol/palpable.schema.json 生成前端协议类型
gen:
	cd web && pnpm run gen:protocol

build:
	cd server && go build ./...
	cd web && pnpm run build

lint:
	cd web && pnpm run lint

test:
	cd server && go test ./...

vet:
	cd server && go vet ./...

# 报告写入 server/coverage.out（已被 .gitignore 忽略），末行即总覆盖率
cover:
	cd server && go test -covermode=atomic -coverprofile=coverage.out ./...
	cd server && go tool cover -func=coverage.out | tail -1

# 每次改动后的完整检查
check: gen lint vet test build

dev-server:
	cd server && go run ./cmd/palpabled

dev-web:
	cd web && pnpm run dev
