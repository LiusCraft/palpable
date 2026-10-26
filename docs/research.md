# 功能对标调研

> 按可感的功能逐项对标：谁做过、有什么事实、对我们意味着什么。市场规模与商业模式先例在 [product.md §6.7](product.md#67-市场与竞品调研)，这里不重复。
> 检索于 2026-10。**事实来自检索结果摘要，只有少数来源打开了原文核对**（Malwarebytes 对 Bondu 的报道、科技日报对国标的报道、Common Sense Media 页面、Tonies 新闻稿）；用户自述、单一媒体、转引的数据已标注。引用前请回到原文。没查到的写在[缺口](#缺口)，不推测。
> 「可感」描述对照 [tasks.md](tasks.md) 与 [模块文档](modules/README.md)。

## 总览

| 功能 | 市场验证 | 最直接的对标 | 可感相对的差异 | 待办 |
| --- | --- | --- | --- | --- |
| [贴近即连](#1-贴近即连加好友) | 强 | 小天才「碰一碰」 | 不绑品牌、不依赖手机 | 实测 BLE 阈值；开放互通的口径 |
| [好友与家长控制](#2-好友与家长控制) | 强，且被监管关注 | 小天才微聊（家长审核） | 默认贴近即生效 + 通知 | **决策：默认是否改为先确认** |
| [AI 对话与内容安全](#3-ai-对话与内容安全) | 市场在涨，安全事件频发 | Curio、Miko | 社交边界在服务端规则里 | 补内容兜底、话题屏蔽、红队测试 |
| [家长回顾与数据安全](#4-家长回顾与数据安全) | 中 | Curio 对话转写 | 含「遇到哪些朋友」 | 保存期限、鉴权、第三方安全测试 |
| [区域频道 / 对讲](#5-区域频道与对讲) | 中（海外，孩子↔家长） | Relay | 孩子之间 + AI + 家长可控 | 访谈验证中国场景 |
| [皮套](#6-皮套与持续消费) | 强（形态） | Tonies 角色卡、Yoto 卡 | 皮套 = 外观 + 个性，不是内容 | 定价与复购验证 |
| [订阅](#7-会员订阅) | 中 | Miko Max、Yoto Club | 好友对讲不耗 AI 成本 | 续费透明度、成本数据 |
| [声纹登记](#8-声纹登记与儿童个人信息) | 弱（未找到同类） | — | — | **法务评估** |

---

## 1. 贴近即连（加好友）

可感：两台设备持续靠近 1.5 秒（强度 ≥ 0.8）生成好友请求，震动与提示音确认（[relation](modules/relation.md)）。

| 事实 | 来源 |
| --- | --- |
| 小天才 2016 年上线「碰一碰加好友」：两个孩子把表盘凑近就能加好友 | 检索摘要；相关报道：[钛媒体](https://www.tmtpost.com/6655066.html) |
| 加好友后需家长端同意 | 抖音用户自述，**弱来源**，未经官方核实 |
| 2026-10 因「只有同品牌能碰一碰」登上热搜；客服理由：开放第三方社交会带来陌生人添加、不良信息与诈骗风险 | [华尔街见闻](https://wallstreetcn.com/articles/3784044) |
| 2025-12-02 市场监管总局发布强制国标《儿童手表安全技术要求》，2027-01-01 起实施。腾讯新闻称标准号 GB 46859—2025，第 4.18 项允许不同品牌手表交换电话号码和昵称，且交换功能受管理端管理（标准号与条款号仅见于单一媒体） | [科技日报](https://www.stdaily.com/web/gdxw/2026-06/22/content_535646.html)（已核对发布时间与实施日期）、[腾讯新闻](https://news.qq.com/rain/a/20260710A031MA00) |

**结论**
- 「靠近即连」是被家长和孩子接受多年的机制，不是新概念；可感的差异不在机制，而在**不绑品牌、不需要手机生态**。
- 「封闭社交」正被推向开放，同时要求**开放也必须受家长管理端控制**。这与可感的做法（好友关系由服务端裁决、家长可解除）方向一致，可以作为对外叙事的支撑；但国标针对儿童手表，是否适用于可感需法务确认，不能写成「符合国标」。
- 将来和别的品牌互通，应走与国标同样的「交换昵称 + 家长管理」模式，不开放陌生人搜索。

**待办**：BLE 强度阈值（0.8）与 1.5 秒在 Web 里是由距离换算出的，真实硬件的误触发与漏触发要实测。

## 2. 好友与家长控制

可感：默认贴近即生效（`ConfirmedBy = auto`）并通知家长，家长可随时解除；`demo.friend_needs_approval` 可改为先确认（[relation](modules/relation.md)、[parentapi](modules/parentapi.md)）。

| 事实 | 来源 |
| --- | --- |
| 小天才自研「微聊」有家长审核与敏感词屏蔽，官方以此解释不对外开放 | [华尔街见闻](https://wallstreetcn.com/articles/3784044) |
| 国标要求家长可管理应用使用时长、禁用应用，支付受家长端管控 | [科技日报](https://www.stdaily.com/web/gdxw/2026-06/22/content_535646.html) |

**结论与需要你决策的点**
- 行业惯例是**家长参与好友建立**。可感默认「先生效、后通知」更顺滑，但在家长看来是更宽松的默认值，也是评审最可能追问的一点。
- 已有开关可切到「先确认」。要么路演时明确说出默认值与理由（孩子在公园当场连不上会放弃）并展示家长可随时解除；要么把默认改为先确认。这是产品决策，不是技术问题。

## 3. AI 对话与内容安全

可感：免按键、风格智能体、提示词带安全规则（不问地址、学校、电话）；陌生人屏蔽在服务端（[voice](modules/voice.md)、[device](modules/device.md)）。

| 事实 | 来源 |
| --- | --- |
| PIRG《Trouble in Toyland》（2025-11）：FoloToy 的 AI 熊 Kumma 会谈性话题，并回答哪里能找到刀、药、火柴；OpenAI 因此切断其模型访问 | [Storyboard18](https://www.storyboard18.com/digital/openai-cuts-off-toymaker-after-ai-teddy-bear-gives-hazardous-advice-to-children-84388.htm)、[KAIT8](https://www.kait8.com/2025/12/12/consumer-safety-report-warns-disturbing-responses-ai-powered-toys/) |
| 国标要求儿童手表的语音问答使用人工审核过的固定知识库，只展示有利于身心健康的内容（针对手表，不直接适用于可感，但体现监管取向） | [科技日报](https://www.stdaily.com/web/gdxw/2026-06/22/content_535646.html) |
| Curio：家长端可看对话转写、可屏蔽话题；转写 90 天自动删除；使用云端大模型 | [Curio 博客](https://heycurio.com/blog/inside-curio-s-app-connected-ai-toys-how-the-technology-works)、[Spielwarenmesse](https://www.spielwarenmesse.de/en/mag/toy-market-news/ai-toys-monitor-children-with-technology-from-microsoft-openai-and-epic-games) |
| Miko 3：kidSAFE+ COPPA 认证，摄像头物理遮挡；不订阅时家长控制有限 | [Gadgeteer 评测](https://the-gadgeteer.com/2025/03/27/miko-3-the-ai-powered-learning-robot-review-a-small-tablet-dressed-as-a-robot/) |
| 儿童语音识别错误率据称比成人高 2–5 倍（转引，原文未核实） | [Unite.AI](https://www.unite.ai/why-general-purpose-speech-ai-falls-short-for-children/) |
| Common Sense Media 建议 5 岁及以下不使用 AI 玩具 | [Common Sense Media](https://www.commonsensemedia.org/research/ai-in-the-toy-box-how-parents-view-ai-enabled-toys-for-young-children) |

**结论**
- 对手的短板正是可感的卖点：**社交边界由服务端规则保证，不靠提示词**。
- 但开放式 AI 对话的内容安全，可感目前也只靠提示词，与被曝光的玩具处在同一层级。
- 缺口：没有输出过滤层、没有话题屏蔽、没有第三方红队测试。

**待办**：①对话内容兜底；②家长端话题 / 关键词屏蔽；③上线前红队测试，用 PIRG 的方式自测；④3–5 岁用户的范围决策（见 [product §6.8](product.md#68-对产品与路演的启示)）。

## 4. 家长回顾与数据安全

可感：今日回顾 = 遇到的好友 + 分享 + 聊天原话；聊天不入库，实时从灵矽拉取，并按 MAC 过滤避免混入别的孩子对话（[history](modules/history.md)）。

| 事实 | 来源 |
| --- | --- |
| Bondu（AI 毛绒玩具）的后台控制台未鉴权，任何 Gmail 账号都能读到约 5 万条孩子的聊天转写，含名字、生日、家庭信息；披露后数小时内修复 | [Malwarebytes，2026-02-03](https://www.malwarebytes.com/?p=372083)（已核对） |
| 美国参议员致函 FoloToy 等，询问是否保存孩子与玩具的对话及保存多久 | [参议员来信](https://www.blackburn.senate.gov/services/files/A10571B2-A8B9-41E0-964F-9EC00C728B9F)（检索摘要） |
| 国产「小智」故事机：家长可通过公众号查看最近 7 天聊天记录 | 检索摘要，旧评测 |
| 另有中文博客称某国产 AI 玩具云端约 1300 万条对话记录可被未授权访问 | 二手，**未核实原始披露，不要引用** |

**结论**
- 家长回看对话是该品类的**标配功能**，同时也是最大的数据泄露面。可感的设计选择（自己不存聊天、`parentapi` 越权一律返回 404）方向正确。
- 但聊天原话与引导期音频仍保存在灵矽侧（音频约 60 天），保存期限与删除机制要写进对外的隐私说明（[linx-integration](linx-integration.md#产品决定)）。

**待办**：保存期限与删除流程；`parentapi` 现在用演示固定令牌，换成真实登录前不要对外开放；上线前第三方安全测试。

## 5. 区域频道与对讲

可感：进入公园后询问是否加入区域频道，只有好友互通，频道内一次一人说话（[channel](modules/channel.md)、[voice](modules/voice.md)）。

| 事实 | 来源 |
| --- | --- |
| Relay（美国）：无屏对讲机，孩子按键说话，家长用手机 App 回复；无短信、无摄像头、无网页。早期报道：设备 49.99 美元 + 每月 9.99 美元（约 2019 年，现价需核实） | [Macaroni Kid](https://aurora.macaronikid.com/articles/5d5b75260fdde975e5654832/relay_-the-alternative-to-cell-phones-for-kids)、[JohnnyJet](https://johnnyjet.com/relay-smartphone-alternative-kids/)（检索摘要） |
| 家长选对讲机的常见理由：孩子还没手机、目的地信号差、人多时防走散 | [Snooper 博客](https://gb.snooper.eu/blog/how-walkie-talkies-can-help-to-keep-your-children-safe/)，海外育儿文章，**弱证据** |
| 家长顾虑：陌生人是否能听到对话；年龄小的孩子使用能力差异大 | [家长记录](https://www.jefftk.com/p/walkie-talkies)，个人博客 |

**结论**
- 「无屏 + 家长可控 + 月费」的儿童对讲有先例（Relay），但它是**孩子 ↔ 家长**，可感是**孩子 ↔ 孩子**，再加 AI。Relay 证明家长接受这种形态，不能证明孩子之间的对讲有需求。
- **中国场景的走散 / 联络痛点没有找到可用的一手数据**，不能写进路演当作已验证。

**待办**：夏令营、研学机构、小区家长的访谈；试点中记录好友对讲时长占比（[product §10](product.md#10-成功指标假设)）。

## 6. 皮套与持续消费

可感：同一内核换皮套，对应风格智能体（[device](modules/device.md)）。

| 事实 | 来源 |
| --- | --- |
| Tonies 单个角色卡零售约 £14.99（英）/ 19.94–29.94 美元（美），联名 Disney 角色同区间 | [RNIB 店](https://shop.rnib.org.uk/collections/toniebox-2?page=2)、[Walmart](https://www.walmart.com/brand/tonies/10008651)（零售快照，含促销） |
| Tonies 2025 年收入 6.303 亿欧元，角色卡约占 74%；全年约 260 万台盒子、4330 万个角色卡 | [公司新闻稿](https://www.mynewsdesk.com/us/tonies/pressreleases/tonies-continues-profitable-growth-with-record-results-in-2025-expects-strong-momentum-for-full-year-2026-expansion-of-ecosystem-around-toniebox-2-proves-a-global-success-3442746) |
| Yoto 卡 9.99–44.99 美元；Yoto Club 约 99 美元 / 年，含每月两张卡 | [JustBeeBlog 评测](https://www.justbeeblog.com/yoto-player-review-is-it-worth-it/) |

**结论**
- 「一个核心 + 不断购买的小件」被 Tonies 验证为主要收入来源。可感的皮套对应角色卡，**参考价位 20–30 美元**（折合人民币需按本地购买力重估）。
- 差异：Tonies 的角色卡是**内容**，可感的皮套是**外观 + 个性**，不是内容载体，复购动机可能弱于「一张新故事」。内容包（故事、游戏）可能更像角色卡，需要验证。

**待办**：试点中记录皮套复购与内容包转化；皮套与内容包的成本测算。

## 7. 会员订阅

| 事实 | 来源 |
| --- | --- |
| Miko 3 的 Max 订阅：月付 14.99 美元或年付 89–99 美元（来源之间不一致）；不订阅时内容与家长控制受限；有用户抱怨自动续费不通知（用户投诉，未核实） | [Gadgeteer](https://the-gadgeteer.com/2025/03/27/miko-3-the-ai-powered-learning-robot-review-a-small-tablet-dressed-as-a-robot/)、[Kimola 评论分析](https://kimola.com/reports/insightful-miko-3-robot-feedback-analysis-report-amazon-en-us-141886) |
| Curio：基础功能不订阅可用，订阅解锁新功能 | [Spielwarenmesse](https://www.spielwarenmesse.de/en/mag/toy-market-news/ai-toys-monitor-children-with-technology-from-microsoft-openai-and-epic-games) |
| Yoto Club 英国新档：£3.99/月或 £29.99/年 | [Star 软文](https://www.thestar.co.uk/recommended/yoto-club-screen-free-audio-entertainment-available-anytime-aff-8746686)，推广内容 |

**结论**
- 同类产品年费区间约 **30–100 美元**，可作为可感会员定价假设的上下界（[product §6.3](product.md#63-为什么单位经济有机会成立)）。
- 订阅的最大信任风险是**续费不透明**。续费提醒与一键取消应作为产品要求。
- 可感的差异：好友对讲不经过 AI，没有 AI 成本，所以「基础连接免费、AI 增强付费」比 Miko「不订阅功能受限」更好讲，但需要成本数据支撑。

## 8. 声纹登记与儿童个人信息

可感：引导期经监护人同意后登记声纹，只存 `speaker_id`；引导智能体在灵矽侧保留音频（[device](modules/device.md)、[linx-integration](linx-integration.md#产品决定)）。

| 事实 | 来源 |
| --- | --- |
| 《个人信息保护法》将不满十四周岁未成年人的个人信息列为敏感个人信息；处理者知道或应当知道时应取得监护人同意 | [中伦解读](https://www.zhonglun.com/research/articles/8222.html)、[Allbright](https://www.allbrightlaw.com/CN/10475/4a9fa4e63788afcf.aspx)（检索摘要，条文号未能确认） |
| 《儿童个人信息网络保护规定》2019-10-01 起施行；终止服务时应停止收集并删除儿童个人信息 | [chinalawtranslate](https://www.chinalawtranslate.com/childrenspersonalinfoonline/) |

**结论**
- 儿童声纹同时是生物识别与未成年人个人信息，是可感里**合规风险最高的一项**，且**没有找到做同类功能的对标产品**。
- 它不在演示主线，建议路演当作「可选能力」，不作为卖点。
- 声纹与音频的具体条款（单独同意、保存期限、删除）没有查到原文，不能代替法务判断。

**待办**：法务评估；决定是否保留「引导智能体存音频」这个例外。

---

## 缺口

| 缺口 | 为什么重要 | 建议补法 |
| --- | --- | --- |
| 国内儿童语音实测：误唤醒、打断、孩子口齿不清时的识别 | 免按键对话是核心体验，VAD 阈值目前是手调的 | 找真实儿童做 10–20 人的小规模实测 |
| 国内同类产品（火火兔、小度、讯飞等）的家长端功能与隐私条款 | 国内评审会对标这些 | 查各家隐私政策与家长端截图 |
| 中国家长对「孩子之间的无屏对讲」的需求与付费意愿 | 整个定位的核心假设 | 访谈 / 预售页 |
| 4 位码建群的同类用法的用户体验 | 语音建群是否可用 | 未检索 |
| 个人信息保护法与声纹相关条款的原文 | 合规 | 法务 |
| 真实硬件成本（BOM）与目标售价 | 单位经济 | 供应商报价 |
