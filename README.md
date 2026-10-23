# palpable
可感是一台以语音为主要交互、外观可被玩偶皮套重新定义的 AI 对讲机。硬件核心保持极简：没有复杂按键，没有需要盯着看的屏幕。用户主要做三个动作：贴近、唤醒、说话。皮套让同一个核心变成不同“角色”。


## 开发

- 后端：`cd server && go run ./cmd/palpabled`（默认 `:8080`，设备端点 `/ws`，健康检查 `/healthz`）
- 前端：`cd web && pnpm install && pnpm dev`（`/ws` 自动代理到本地网关）
- 协议类型生成：`cd web && pnpm run gen:protocol`
- 全量检查：`make check`（包含协议类型生成、前端 lint、Go vet/test 和前后端构建；Windows 没有 make 时，请在 `web`、`server` 目录分别执行对应命令）

文档：[AGENTS.md](AGENTS.md)（规则）、[docs/architecture.md](docs/architecture.md)（架构与模块）、[protocol/README.md](protocol/README.md)（设备协议）。
