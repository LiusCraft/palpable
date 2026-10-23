.PHONY: gen build test vet check dev-server dev-web

# 由 protocol/palpable.schema.json 生成前端协议类型
gen:
	cd web && pnpm run gen:protocol

build:
	cd server && go build ./...
	cd web && pnpm run build

test:
	cd server && go test ./...

vet:
	cd server && go vet ./...

# 每次改动后的完整检查
check: gen vet test build

dev-server:
	cd server && go run ./cmd/palpabled

dev-web:
	cd web && pnpm run dev
