# 家长端小程序

前端 · `miniprogram/` · 依赖：家长端 REST（[parentapi](parentapi.md)）

## 边界

**职责**：面向真实家长的微信小程序。登录、绑定与解绑玩偶、交友放行与解除、屏蔽别家小伙伴、交友方式与小伙伴发言设置、今日回顾、法律文本、注销账号。

**不负责**：任何业务规则。谁能成为好友、谁能听见谁、绑定码是否有效，都由服务端裁决；小程序只发请求、展示结果、把错误翻成家长看得懂的话。

## 技术选型

| 项 | 选型 | 原因 |
| --- | --- | --- |
| 框架 | 原生小程序 + TypeScript（`strict`），开发者工具内置 TS 编译 | 无跨端需求；少一层编译链，与 TDesign 的原生组件直接对接 |
| 组件库 | TDesign 小程序 `tdesign-miniprogram`（锁定版本） | 腾讯官方维护、2026 年仍在活跃发版；Vant Weapp 最后一次发版在 2024 年底 |
| 主题 | 覆盖 TDesign 的 CSS 变量（`app.wxss`）：暖奶油底、珊瑚橙主色、棕色描边、贴纸投影、大圆角 | 儿童绘本画风，又不改组件源码 |
| 插画 | `assets/` 下的扁平 SVG（每种皮套一个头像、彩蛋兜底、空状态插画），无滤镜 | `<image>` 渲染 SVG 稳定；包体小 |
| 包管理 | pnpm，`nodeLinker: hoisted` | 开发者工具"构建 npm"不认 pnpm 符号链接 |

## 页面

| 页面 | 内容 | 调用 |
| --- | --- | --- |
| `pages/home` | 问候、名下的小伙伴卡片（在线、等你同意的角标、起名阶段）、绑定入口；首次打开先弹隐私同意 | `GET /api/kids`、`GET /api/me` |
| `pages/bind` | 6 位绑定码格子、必选的监护人同意、单独的声音登记开关 | `POST /api/bindings` |
| `pages/kid` | 三个标签：今天（所在频道、遇到的朋友、分享、对话）、朋友（等你同意 / 等对方家长 / 好朋友）、设置（交友方式、小伙伴发言、声音登记、解绑） | 孩子相关的全部接口 |
| `pages/me` | 称呼、法律文本、联系我们（`open-type="contact"`）、退出登录、注销账号 | `/api/me`、`DELETE /api/session` |
| `pages/legal` | 用户协议、隐私政策、儿童个人信息保护规则 | — |

## 登录与会话

```text
getToken():  本地令牌离过期超过 1 天 → 直接用；否则 signIn()
signIn():    wx.login → POST /api/session {code} → 存 {token, expiresAt}；并发调用共用同一次登录
call():      带 Bearer 发请求；401 → 清本地令牌、signIn()、重试一次
```

开发版可在 `config.ts` 的 `develop.devToken` 填服务端 `demo.parent_token`，跳过微信登录联调；体验版与正式版不读这个字段。

## 隐私

- 首次打开（或 `brand.policyVersion` 变化后）先弹隐私同意，**同意前不登录、不请求服务端**。
- "同意"按钮带 `open-type="agreePrivacyAuthorization"`（经 TDesign 的 `t-id` 落到原生按钮），一次点击同时完成我们的同意记录与微信平台的隐私授权；平台在调用隐私接口（如复制群口令）时要求授权，由当前页面上的 `privacy-sheet` 接住 `wx.onNeedPrivacyAuthorization`。
- 不获取手机号、头像、昵称、位置；称呼由家长自愿填写。`sitemap.json` 禁止收录。
- 法律文本在 `legal/documents.ts`，内容依据服务端实际处理的数据写成，**上线前须法务审定**并在 `config.ts` 的 `brand` 填入运营主体与联系方式。

## 约束

- 页面可见文字都来自 `utils/copy.ts`；WXML 不能调用函数，带参数的文案在 `.ts` 里算好再 `setData`（`texts()` 去掉函数）。
- 项目根是 `miniprogram/`（有 `project.config.json` 的那层），源码在 `src/`。开发者工具必须导入项目根：导入 `src/` 会自动生成一份不开 TypeScript 编译的配置，运行时报 `module 'components/x/index.js' is not defined`。
- `pnpm check` = `tsc --noEmit` + `scripts/lint.mjs`（JSON 可解析、WXML 用到的组件已注册、相对路径的组件文件存在、模板里不调函数、引用的资源存在、文案不索要个人信息）。
- `pnpm build:npm` 用 `miniprogram-ci` 构建 `miniprogram_npm`，不开开发者工具也能打包。
