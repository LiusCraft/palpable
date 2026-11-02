.PHONY: enroll-device gen gen-check fmt-check build lint test vet cover check miniprogram-check dev-server dev-web dev-admin dev-admin-web firmware-test firmware-smoke

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

gen-check: gen
	@git diff --exit-code -- web/src/shared/protocol.gen.ts

build:
	cd server && go build ./...
	cd web && pnpm run build
	cd admin && pnpm run build

lint:
	cd web && pnpm run lint
	cd admin && pnpm run lint

test:
	cd server && go test ./...

vet:
	cd server && go vet ./...

# 统计 server/internal 的覆盖率；低于 MIN_COVERAGE 则失败。
# cmd/palpabled 只是装配入口，不纳入统计。
cover:
	cd server && go test -covermode=atomic -coverpkg=./internal/... -coverprofile=coverage.out ./internal/...
	cd server && ../.github/scripts/check-coverage.sh coverage.out $(MIN_COVERAGE)

# 家长端小程序：tsc + WXML/JSON 静态检查
miniprogram-check:
	cd miniprogram && pnpm check

# 每次改动后的完整检查
check: gen-check fmt-check lint vet cover build miniprogram-check

# manifest 模式下登记一台设备（如 Web 模拟器）的公钥：MAC 与 PUBKEY 取自 palpabled 的
# "device authentication failed" 日志。令牌从 server/config.yaml 的 ops.token 读取，不回显。
enroll-device:
	@test -n "$(MAC)" && test -n "$(PUBKEY)" || { echo 'usage: make enroll-device MAC=aa:bb:cc:dd:ee:ff PUBKEY=<base64>'; exit 1; }
	@TOKEN=$$(awk '/^ops:/{f=1;next} f&&/^[^ ]/{f=0} f&&$$1=="token:"{print $$2}' server/config.yaml | tr -d '"'); \
	test -n "$$TOKEN" || { echo 'ops.token missing in server/config.yaml'; exit 1; }; \
	curl -sS -w 'HTTP %{http_code}\n' -X PUT -H "Authorization: Bearer $$TOKEN" -H 'Content-Type: application/json' \
		-d '{"public_key":"$(PUBKEY)"}' "http://127.0.0.1:8080/internal/devices/$(MAC)/key"

dev-server:
	cd server && go run ./cmd/palpabled

dev-web:
	cd web && pnpm run dev

# 厂家后台：后端（:8090，读 server/admin.config.yaml）与页面（:5174，/api 代理到后端）
dev-admin:
	cd server && go run ./cmd/palpable-admin

dev-admin-web:
	cd admin && pnpm run dev

# 不依赖 ESP-IDF；可用 -DCJSON_SOURCE_DIR=... 离线运行。
firmware-test:
	cmake -S firmware/tests -B firmware/.build/host $(FIRMWARE_CMAKE_ARGS)
	cmake --build firmware/.build/host
	ctest --test-dir firmware/.build/host --output-on-failure

firmware-smoke: firmware-test
	cd server && go build -o ../firmware/.build/palpabled ./cmd/palpabled
	python3 firmware/tests/gateway_smoke.py --server-bin firmware/.build/palpabled
