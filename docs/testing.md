# 测试与质量检查

> 只记怎么验证、门槛是什么、缺什么。各模块的验收条件在 [tasks.md](tasks.md)；模块设计见 [modules/](modules/README.md)。

## 检查命令

| 命令 | 内容 |
| --- | --- |
| `make check` | 协议类型生成物一致性、`gofmt`、前端 lint、`go vet`、后端单测与覆盖率、前后端构建 |
| `make cover` | 后端单测覆盖率，低于门槛失败 |
| `make gen-check` | 重新生成 TS 协议类型并检查与提交一致 |

Windows 没有 `make` 时，在 `web`、`server` 目录分别执行 Makefile 中对应的命令。

## 分层

| 层 | 内容 | 位置 |
| --- | --- | --- |
| 单元测试 | 各模块用 `store.NewMemory()` 与 `linxapi.Fake` 替身，不依赖网络 | `server/internal/*/*_test.go` |
| 仓储契约测试 | 内存实现与 SQLite 跑同一套用例 | `server/internal/store` |
| 协议契约测试 | `protocol/examples/` 的合法样例必须被 Go 解码，`invalid-*` 必须被拒绝，Go 编码的下行消息必须通过 schema | `server/internal/protocol` |
| 浏览器冒烟 | 演示主线（开箱 → 认主 → 公园 → 好友 → 互相听见）端到端 | `web/tests/demo-smoke.spec.ts` |

## 门槛

- 后端 `server/internal` 单测覆盖率 ≥ 90%，`cmd/palpabled` 不计入；`make cover` 与 CI 都执行。
- CI 在 `main` 上把覆盖率写入 `badges` 分支的 `coverage.svg`；PR 的报告从 Actions 的 `go-coverage` artifact 下载。
- 语音链路的验收条件见 [tasks.md 语音验收](tasks.md#语音验收s5)。

## 已知缺口

- 多人语音端到端延迟没有实测数据；服务端内部延迟只在每个发言结束时以 debug 日志输出。
- 前端没有单元测试。
- 浏览器音频仅在 Chromium 系验证。
