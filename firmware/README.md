# 可感 ESP32 固件

源码来自 [torbys/xiaozhi-esp32 `codex/yunliao-startup-diagnostics`](https://github.com/torbys/xiaozhi-esp32/tree/codex/yunliao-startup-diagnostics)（开源小智 xiaozhi-esp32 的魔改，提交 a13c0be），整体放在本目录，直接在这里修改和构建，不再有"拉上游再打补丁"的步骤。小智原 README 见 [README.xiaozhi.md](README.xiaozhi.md)，MIT 许可证见 [LICENSE](LICENSE)。

应用层已把小智云协议（OTA 激活、MQTT/WebSocket 云端会话）整体替换为 [可感设备协议](../protocol/README.md)：设备只连接可感 Go 网关 `/ws`；好友、频道、认主、家长控制与灵矽会话全在服务端，固件不含业务判断，也不持有灵矽 token。

## 构建与烧录

需要 **ESP-IDF 5.4.0+**（已用 5.4.1 验证环境）。在本目录执行：

```sh
. /path/to/esp-idf/export.sh
# 选板型：在 sdkconfig.defaults.local（不入库）写 CONFIG_BOARD_TYPE_YUNLIAO_S3=y，
# 并显式加载；仅放在该文件里不会被 IDF 自动读取。
idf.py -D SDKCONFIG_DEFAULTS="sdkconfig.defaults;sdkconfig.defaults.esp32s3;sdkconfig.defaults.local" set-target esp32s3
idf.py build
idf.py -p /dev/ttyUSB0 flash monitor
```

`menuconfig` 的 **Palpable device** 菜单：

| 配置 | 含义 |
| --- | --- |
| Palpable server WebSocket URL | 编译时默认地址，如 `ws://192.168.1.100:8080/ws`，必须含 `/ws`。运行时以配网页「高级 → 自定义 OTA 地址」（NVS `wifi/ota_url`）为准，用来切换环境；两者都空则不连接 |
| Device skin | `dragon` / `rabbit` / `bear` / `cat`，由服务端决定风格 |
| Microphone RMS threshold | 16 位 PCM 的语音检测阈值，按麦克风与外壳校准 |
| Haptic motor driver GPIO | 默认 `-1` 不启用；需接马达驱动电路 |
| Location / Proximity sensor | 板级传感器接好后再开，见下 |

首次启动沿用小智的热点/BluFi 配网（热点前缀 `Palpable`）。`wss://` 依赖系统时间，Wi-Fi 起来后等 SNTP 校时再连；连接失败按 1、2、4、8、16、30 秒退避重试。

## 本目录相对小智分支的改动

| 位置 | 改动 |
| --- | --- |
| `main/palpable_client.*` | 无 ESP-IDF 依赖的协议客户端：握手、上行、下行参数与音源、VAD 发言边界（180ms 预录、480ms 尾静音） |
| `main/application.*` | 重写为可感协议的应用层；主任务串行处理传输队列事件，旧连接回调按连接代次隔离；保留分支自带的 `WAKE` / `VAD` / `RX` 诊断日志 |
| `main/Kconfig.palpable` | 上表配置项 |
| `scripts/gen_schema.py` | 由 `protocol/palpable.schema.json` 生成 cJSON 校验器 `palpable_schema.h`，构建时自动生成 |
| `main/audio/audio_service.*` | 编码前给每帧打 VAD 标记（`voice_active`）；新增 `IsPlaybackIdle()` |
| `main/led/*` | `SupportsProtocolColor` / `SetProtocolColor`，让 `light` 消息驱动 RGB |
| `main/protocols/` | 删除 MQTT / WebSocket 小智协议与 `ota.*`，仅保留 `Protocol` 基类的数据结构 |
| `main/boards/palpable-wgh-c2/` | 自有板型（Waveshare ESP32-S3-Touch-LCD-1.54 引脚，ES8311 暂未识别，用 `NoAudioCodec`） |
| `tests/` | 主机协议测试与真实 Go 网关连接测试 |

可感音频是无附加头的 Opus，不能使用小智的服务器侧 AEC 时间戳协议，故 `USE_SERVER_AEC` 已禁用。没有设备侧 AEC 时，扬声器播放期间暂不上行麦克风；触摸唤醒（`wake{source:touch}`）仍可用，语音唤醒发 `wake{source:voice}`。设备不决定音频去 AI 还是频道。

MCP 保留板级硬件工具，并按硬件能力注册 `self.light.set`、`self.haptic.pulse`；不注册云端升级等用户管理工具，固件通过 `idf.py flash` 更新。

## 接入定位与靠近传感器

没有指定 GPS、BLE 或 NFC 模组，默认不声明 `location`、`proximity`，也不上报模拟数据。板级传感器接好后启用对应 `PALPABLE_*_SENSOR` 选项，在驱动回调中调用：

```cpp
Application::GetInstance().ReportLocation(latitude, longitude);
Application::GetInstance().ReportProximity({{peer_mac, normalized_strength}});
```

位置最多每秒一次；靠近扫描最多每 500ms 一次，必须持续提交完整扫描列表（没有设备时提交空列表），强度范围 `[0, 1]`。好友防抖、建立关系和陌生设备屏蔽均由服务端决定。

## 验证

无需 ESP-IDF，需要 C++20 编译器、CMake、Python 和 Go：

```sh
make firmware-test
make firmware-smoke
```

`firmware-test` 对照协议合法/非法样例检查握手、未知字段、VAD 帧顺序、断线清理、采样率切换和传感器上行校验；`firmware-smoke` 让同一份 C++ 客户端连接临时启动的真实 Go 网关完成握手和语音上行。CI 执行这两项。设备侧代码（`application.*`、板级）需用 ESP-IDF 实际编译，主机测试覆盖不到。

实机验收（Wi-Fi 配网、掉线恢复、起名与交棒、AI 语音、唤醒、VAD 阈值、提示灯/振动、传感器接入后的入区、靠近与好友音频）记录在 [任务表](../docs/tasks.md#固件)。
