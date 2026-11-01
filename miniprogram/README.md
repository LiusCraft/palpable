# 可感 · 家长端小程序

面向真实家长的微信小程序：微信登录、绑定玩偶、交友放行、今日回顾、家长控制。设计见 [docs/modules/miniprogram.md](../docs/modules/miniprogram.md)，接口见 [parentapi](../docs/modules/parentapi.md)。

## 本地开发

```bash
cd miniprogram
pnpm install          # nodeLinker: hoisted，开发者工具的"构建 npm"才认得
pnpm check            # tsc + WXML/JSON 静态检查
```

用微信开发者工具导入 **`miniprogram/` 这一层**（有 `project.config.json` 的目录，AppID 已写在里面），不要导入 `src/`：那样开发者工具会另生成一份不开 TypeScript 编译的配置，组件全部报 `module ... is not defined`。导入后先点"工具 → 构建 npm"生成 `src/miniprogram_npm`（TDesign 组件），之后改了依赖再构建一次。

**连本地服务端：**

1. 服务端 `server/config.yaml` 填 `wechat.app_id` 与 `wechat.secret`（小程序后台 → 开发管理 → 开发设置），`make dev-server`。
2. 开发者工具 → 详情 → 本地设置，勾选"不校验合法域名"；`src/config.ts` 的 `develop.apiBase` 默认 `http://127.0.0.1:8080`，真机预览时改成电脑的局域网 IP。
3. 没有 AppSecret 时，可在 `src/config.ts` 的 `develop.devToken` 填服务端 `demo.parent_token`，跳过微信登录、以演示家长身份联调（只在开发版生效，**不要提交非空值**）。

绑定流程要演示时，服务端打开 `demo.manual_bind: true`，Web 模拟器会显示并播报 6 位绑定码。

## 上线清单

| 项 | 在哪做 | 状态 |
| --- | --- | --- |
| 服务端 HTTPS 域名，并在小程序后台登记为 request 合法域名 | 小程序后台 → 开发管理 → 服务器域名 | ⬜ |
| `config.ts` 的 `trial` / `release` 地址换成真实域名 | 本仓库 | ⬜ |
| 生产 `server/config.yaml`：填 `wechat.*`；`demo.*` 全部留空（尤其 `parent_token`、`guardian`、`world`）；`auth.enrollment: manifest` | 服务器 | ⬜ |
| `config.ts` 的 `brand`：运营主体全称、注册地址、客服邮箱 | 本仓库 | ⬜ |
| 法律文本（`legal/documents.ts`）法务审定 | 法务 | ⬜ |
| 小程序后台《用户隐私保护指引》：声明收集"微信用户标识"与"剪切板"（复制群口令），与隐私政策一致 | 小程序后台 → 设置 → 服务内容声明 | ⬜ |
| 服务类目与资质（AI 陪伴 / 智能硬件配套工具；涉及生成式 AI 的备案与未成年人保护要求以平台审核为准） | 小程序后台 → 设置 → 基本设置 | ⬜ |
| 客服：开通小程序客服，"我的 → 联系我们"才有人接 | 小程序后台 → 功能 → 客服 | ⬜ |
| 交友请求订阅消息（S19 / P2） | 本仓库 + 小程序后台申请模板 | ⬜ |
| 真机走查：iOS / Android 各一台，覆盖首次打开、绑定、交友放行、解绑、注销 | 开发者工具预览 | ⬜ |
