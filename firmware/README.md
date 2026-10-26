# 可感 ESP32 固件

基于 [78/xiaozhi-esp32](https://github.com/78/xiaozhi-esp32) 的硬件层，应用层实现 [可感设备协议](../protocol/README.md)。设备只连接可感 Go 网关 `/ws`；好友、频道、认主、家长控制与灵矽会话由服务端处理。

上游版本锁在 [upstream.lock.json](upstream.lock.json)，硬件驱动保留其 [MIT 许可证](LICENSE.xiaozhi)。`scripts/prepare.py` 导出锁定提交，覆盖 `main/` 中的可感应用，补充 PCM 语音标记与 RGB 灯驱动接口，生成 `.build/xiaozhi/`。上游源码与编译产物不入库。修改应放在本目录的源文件或准备脚本中；生成目录中同名源码会在准备时重建，`sdkconfig` 和构建缓存保留。更新上游版本前备份 `sdkconfig`，清理 `.build/xiaozhi/`，再重新准备。

## 构建与烧录

需要 Python 3.9+、Git、CMake，以及 **ESP-IDF 6.0.1+**。上游建议 `release/v6.1`；ESP32-S31 必须使用 6.1+。ESP-IDF 5.x 不适用于这里锁定的版本。

在仓库根目录执行：

```sh
. /path/to/esp-idf/export.sh
./firmware/build.sh set-target esp32s3
./firmware/build.sh menuconfig
./firmware/build.sh build
./firmware/build.sh -p /dev/ttyUSB0 flash monitor
```

`build.sh` 会先准备上游，再把参数原样交给 `idf.py`。首次联网下载上游及 IDF 组件。已有锁定提交的本地仓库时，可单独执行 `python3 firmware/scripts/prepare.py --source /path/to/xiaozhi-esp32`，然后在 `firmware/.build/xiaozhi/` 直接运行 `idf.py`。

在 `menuconfig` 中配置：

| 菜单 | 配置 | 含义 |
| --- | --- | --- |
| Xiaozhi Assistant | Target Board | 选择真实板型；I2S、屏幕、按键引脚沿用该板驱动 |
| Xiaozhi Assistant | 板级显示 / Flash / PSRAM 选项 | 与实际硬件匹配；WGH-C2 的 ST7789 240×240 已在板级 `config.h` 固定 |
| Xiaozhi Assistant | Wake Word Type | 使用有模型支持的本地唤醒；S3 可选 AFE Wake Word，唤醒词由所选模型决定 |
| Xiaozhi Assistant | Use Audio Processor | S3/P4 等可使用 AFE 降噪与设备侧 AEC；AEC 需硬件参考通道 |
| Palpable device | Palpable server WebSocket URL | 例如 `ws://192.168.1.100:8080/ws`，必须包含 `/ws`；留空时不连接 |
| Palpable device | Device skin | `dragon` / `rabbit` / `bear` / `cat`，由服务端决定风格 |
| Palpable device | Microphone RMS threshold | 16 位 PCM 的语音检测阈值，按麦克风与外壳校准 |
| Palpable device | Haptic motor driver GPIO | 默认 `-1` 不启用；需选择未占用的输出引脚并接马达驱动电路 |

当前实机使用仓库内的 `Palpable WGH-C2` 板型（`main/boards/palpable/wgh-c2/`，引脚沿用 Waveshare ESP32-S3-Touch-LCD-1.54）：ST7789 240×240 SPI 屏、ES8311 音频、按键 PWR=5 / VOL+=4 / VOL-=0。menuconfig 里选 Target Board → **Palpable WGH-C2**，不要再选 bread-compact-wifi。屏幕和背光已验证；ES8311 目前未在 I2C 0x18 扫到，固件暂用 `NoAudioCodec`，音频待查。其他板型仍可通过原有板级菜单选择；构建与实机验收范围见 [任务表](../docs/tasks.md#固件)。

首次启动沿用小智的热点/BluFi 配网方式。Wi-Fi 连上后自动连接可感服务，不经过小智云激活或固件更新服务。服务端照常负责在灵矽开通设备，灵矽 token、API Key 和 OTA 配置只放在服务端。固件无需这些密钥。

`wss://` 的证书校验依赖系统时间。固件启动时先用编译时间兜底，Wi-Fi 起来后通过 SNTP 校正；连接超时后按 1、2、4、8、16、30 秒退避重试。

默认麦克风持续采集，通过 VAD 自动发送语音。板上原有对话/触摸按键作为 `wake{source:touch}`，语音唤醒发送 `wake{source:voice}`。没有设备侧 AEC 时，在扬声器播放期间暂不上行麦克风，避免把播放声音重新送回服务端；触摸唤醒仍然可用。实时双向语音需在支持参考通道的板上启用设备侧 AEC。可感音频是无附加头的 Opus，不能使用小智的服务器侧 AEC 时间戳协议。

## 当前实机状态

- 屏幕：ST7789 240×240 初始化、LVGL、背光和状态显示已验证。
- 音频：I2C 扫描暂未发现 ES8311（0x18），当前使用 `NoAudioCodec`；屏幕和网络不受影响，音频待查供电或 I2C 引脚。
- 双设备联调、GPS/BLE/NFC 和 AEC 仍未验收。

## 代码边界

| 文件 | 职责 |
| --- | --- |
| `main/palpable_client.*` | 无 ESP-IDF 依赖的协议客户端；握手、上行、下行参数与音源、VAD 发言边界 |
| `main/application.*` | 小智板级接口、音频、网络与可感客户端的组装；主任务串行处理协议事件 |
| `scripts/gen_schema.py` | 从 `protocol/palpable.schema.json` 生成 cJSON 校验器；新增未支持的 schema 约束会使生成失败 |
| `scripts/prepare.py` | 锁定并准备上游，替换应用层，移除小智云传输与 OTA 的构建源文件 |
| `tests/` | 主机协议测试与真实 Go 网关连接测试 |

传输回调只入有界队列，不执行工具或控制硬件；旧连接的回调由连接代次隔离。握手 10 秒未完成会断开并重试，重连间隔为 1、2、4、8、16、30 秒。每次重新连接恢复默认下行参数并丢弃旧会话的排队音频。

语音检测在编码前标记每个 PCM 帧，标记随 Opus 包一起进入发送队列。协议客户端保留 180ms 预录和 480ms 尾静音，使 `mic.state start → 音频帧 → mic.state stop` 在同一任务中严格有序。设备不决定音频去 AI 还是频道。

AI 的下行参数取自 `audio.downlink`；好友语音固定为协议上行的 16kHz / 60ms。切换来源时清理解码队列，AI 正常说完时保留尾音，好友结束时清理排队音频。

RGB `SingleLed` / `CircularStrip` 支持协议灯色与 solid / breathe / pulse；没有 RGB 支持的板不声明 `light`，保留板上连接状态指示。振动只在配置马达驱动 GPIO 后声明。`sound` 使用本地短提示音，`presence`、`channel.state` 与 `friend.state` 只显示服务端通知，不做好友或频道判断。MCP 保留板级硬件工具，并按硬件能力注册 `self.light.set`、`self.haptic.pulse`；不注册云端升级等用户管理工具。上游应用兼容接口 `UpgradeFirmware` 返回失败，固件通过 `idf.py flash` 更新。

## 接入定位与靠近传感器

没有指定 GPS、BLE 或 NFC 模组，默认不声明 `location`、`proximity`，也不上报模拟数据。板级传感器接好后启用相应 `PALPABLE_*_SENSOR` 选项，在驱动的采样回调中调用：

```cpp
Application::GetInstance().ReportLocation(latitude, longitude);
Application::GetInstance().ReportProximity({{peer_mac, normalized_strength}});
```

位置最多每秒上报一次；靠近扫描最多每 500ms 一次。靠近回调必须持续提交完整扫描列表，没有设备时也要提交空列表；强度范围是 `[0, 1]`。好友防抖、建立关系和陌生设备屏蔽均由服务端决定。只允许从服务端下行通知中展示其他设备的信息。

## 验证

无需 ESP-IDF 即可执行协议核心与网关测试，需要 C++20 编译器、CMake、Python 和 Go：

```sh
make firmware-test
make firmware-smoke
```

主机测试默认拉取锁定版本的 cJSON；离线时可提供现有源码目录：

```sh
make firmware-smoke FIRMWARE_CMAKE_ARGS=-DCJSON_SOURCE_DIR=/path/to/cJSON
```

`firmware-test` 对照协议的合法/非法样例，并检查握手、未知字段和类型、VAD 帧顺序、断线清理、AI/好友采样率切换、MCP 与传感器上行校验。`firmware-smoke` 临时启动使用内存数据库、无灵矽配置的 Go 网关，让同一份 C++ 客户端完成 WebSocket 握手和语音上行；结束时关闭临时进程，不访问演示业务捷径。CI 执行这两类测试。

实机验收需用两台设备检查：Wi-Fi 配网、掉线恢复、起名与交棒、AI 语音、唤醒、VAD 阈值、提示灯/振动，以及传感器接入后的入区、靠近与好友音频。验收结果统一记录在任务表。
