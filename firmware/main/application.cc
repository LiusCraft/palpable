#include "application.h"
#include "assets.h"
#include "assets/lang_config.h"
#include "board.h"
#include "display.h"
#include "mcp_server.h"
#include "palpable_schema.h"
#include "system_info.h"

#include <driver/gpio.h>
#include <esp_log.h>
#include <esp_system.h>
#include <esp_timer.h>
#include <algorithm>
#include <cmath>
#include <cstdlib>
#include <ctime>
#include <utility>

namespace {
constexpr EventBits_t kTasks = 1 << 0;
constexpr EventBits_t kAudio = 1 << 1;
constexpr EventBits_t kIncoming = 1 << 2;
constexpr EventBits_t kNetwork = 1 << 3;
constexpr EventBits_t kLost = 1 << 4;
constexpr EventBits_t kEvents = kTasks | kAudio | kIncoming | kNetwork | kLost;
int64_t Milliseconds() { return esp_timer_get_time() / 1000; }
const char* Text(const cJSON* root, const char* key) {
    auto value = palpable::schema::Item(root, key);
    return cJSON_IsString(value) ? value->valuestring : "";
}
}  // namespace

Application::Application() {
    events_ = xEventGroupCreate();
    incoming_ = xQueueCreate(32, sizeof(Incoming*));
    configASSERT(events_ && incoming_);
#if CONFIG_USE_DEVICE_AEC
    aec_mode_ = kAecOnDeviceSide;
#endif
    client_ = std::make_unique<palpable::Client>(palpable::Callbacks{
        .send_text = [this](const std::string& text) {
            return websocket_ && websocket_->IsConnected() && websocket_->Send(text);
        },
        .send_audio = [this](const palpable::Bytes& bytes) {
            return websocket_ && websocket_->IsConnected() &&
                   websocket_->Send(bytes.data(), bytes.size(), true);
        },
        .message = [this](const cJSON* root) { HandleMessage(root); },
        .audio = [this](const palpable::Bytes& bytes, int sample_rate, int frame_duration) {
            auto packet = std::make_unique<AudioStreamPacket>();
            packet->sample_rate = sample_rate;
            packet->frame_duration = frame_duration;
            packet->payload = bytes;
            audio_service_.PushPacketToDecodeQueue(std::move(packet), false);
        },
        .flush_audio = [this]() { audio_service_.ResetDecoder(); },
    });
}

void Application::Initialize() {
    auto& board = Board::GetInstance();
    SetDeviceState(kDeviceStateStarting);
    board.GetDisplay()->SetupUI();
    auto& assets = Assets::GetInstance();
    if (assets.partition_valid()) assets.Apply();

    AudioServiceCallbacks callbacks;
    callbacks.on_send_queue_available = [this]() { xEventGroupSetBits(events_, kAudio); };
    callbacks.on_wake_word_detected = [this](const std::string&) {
        Schedule([this]() {
            client_->Wake("voice");
            audio_service_.EnableWakeWordDetection(true);
        });
    };
    audio_service_.SetCallbacks(callbacks);
    audio_service_.Initialize(board.GetAudioCodec());
    audio_service_.Start();
    auto& mcp = McpServer::GetInstance();
    mcp.AddCommonTools();
    if (board.GetLed()->SupportsProtocolColor()) {
        mcp.AddTool("self.light.set", "给小伙伴换一个灯光颜色。",
            PropertyList({Property("color", kPropertyTypeString).SetMaxLength(7),
                          Property("pattern", kPropertyTypeString, std::string("solid"))}),
            [this](const PropertyList& properties) -> ToolResult {
                auto color = properties["color"].value<std::string>();
                auto pattern = properties["pattern"].value<std::string>();
                if (!palpable::schema::MatchColor(color.c_str()) ||
                    (pattern != "solid" && pattern != "breathe" && pattern != "pulse")) {
                    return std::unexpected("灯光参数不合适，请换一个颜色试试。");
                }
                light_color_ = color;
                light_pattern_ = pattern;
                next_light_at_ = 0;
                return true;
            });
    }
#if CONFIG_PALPABLE_HAPTIC_GPIO >= 0
    mcp.AddTool("self.haptic.pulse", "让小伙伴轻轻振动一下。",
        PropertyList({Property("pattern", kPropertyTypeString, std::string("tap"))}),
        [this](const PropertyList& properties) -> ToolResult {
            auto pattern = properties["pattern"].value<std::string>();
            if (pattern != "tap" && pattern != "double_tap" && pattern != "long") {
                return std::unexpected("振动参数不合适，请再试一次。");
            }
            haptic_pattern_ = pattern;
            haptic_start_ = Milliseconds();
            return true;
        });
#endif

#if CONFIG_PALPABLE_HAPTIC_GPIO >= 0
    gpio_config_t config = {};
    config.pin_bit_mask = 1ULL << CONFIG_PALPABLE_HAPTIC_GPIO;
    config.mode = GPIO_MODE_OUTPUT;
    ESP_ERROR_CHECK(gpio_config(&config));
    ESP_ERROR_CHECK(gpio_set_level(static_cast<gpio_num_t>(CONFIG_PALPABLE_HAPTIC_GPIO), 0));
#endif

    board.SetNetworkEventCallback([this](NetworkEvent event, const std::string&) {
        if (event == NetworkEvent::Connected) {
            network_up_ = true;
            xEventGroupSetBits(events_, kNetwork);
        } else if (event == NetworkEvent::Disconnected || event == NetworkEvent::Scanning ||
                   event == NetworkEvent::WifiConfigModeEnter) {
            network_up_ = false;
            xEventGroupSetBits(events_, kNetwork);
        }
    });
    board.StartNetwork();
    board.GetDisplay()->UpdateStatusBar(true);
}

void Application::Schedule(std::function<void()>&& callback) {
    {
        std::lock_guard<std::mutex> lock(tasks_mutex_);
        tasks_.push_back(std::move(callback));
    }
    xEventGroupSetBits(events_, kTasks);
}

void Application::Run() {
    vTaskPrioritySet(nullptr, 10);
    int64_t status_at = 0;
    while (true) {
        auto bits = xEventGroupWaitBits(events_, kEvents, pdTRUE, pdFALSE, pdMS_TO_TICKS(20));
        if (bits & kTasks) {
            std::deque<std::function<void()>> tasks;
            {
                std::lock_guard<std::mutex> lock(tasks_mutex_);
                tasks.swap(tasks_);
            }
            for (auto& task : tasks) task();
        }
        if (bits & kNetwork) {
            CloseLink();
            retry_seconds_ = 1;
            retry_at_ = Milliseconds();
        }
        if (connection_lost_.exchange(false)) Retry();

        Incoming* raw = nullptr;
        while (xQueueReceive(incoming_, &raw, 0) == pdTRUE) {
            std::unique_ptr<Incoming> frame(raw);
            if (frame->generation != generation_.load()) continue;
            if (frame->binary) {
                client_->ReceiveAudio(palpable::Bytes(frame->data.begin(), frame->data.end()));
            } else if (!client_->ReceiveText(frame->data)) {
                ESP_LOGW("Palpable", "Rejected invalid server message");
            }
        }
        while (auto packet = audio_service_.PopPacketFromSendQueue()) {
            if (client_->Ready() && (aec_mode_ == kAecOnDeviceSide || audio_service_.IsPlaybackIdle())) {
                client_->Microphone(packet->payload, packet->voice_active);
            } else {
                client_->SuspendMicrophone();
            }
        }
        talking_ = client_->Talking();
        if (client_->Failed()) Retry();
        if (websocket_ && !client_->Ready() && Milliseconds() >= hello_deadline_) Retry();
        if (network_up_ && !websocket_ && Milliseconds() >= retry_at_) Connect();
        TickHaptic();
        TickLight();
        if (Milliseconds() >= status_at) {
            Board::GetInstance().GetDisplay()->UpdateStatusBar();
            status_at = Milliseconds() + 1000;
        }
    }
}

void Application::Connect() {
    const std::string url = CONFIG_PALPABLE_WS_URL;
    if (!(url.starts_with("ws://") || url.starts_with("wss://"))) {
        Board::GetInstance().GetDisplay()->SetStatus("请先设置可感服务地址");
        retry_at_ = Milliseconds() + 30000;
        return;
    }
    // wss 的证书校验依赖系统时间；时间没同步前先等 SNTP，避免反复 TLS 失败。
    if (time(nullptr) < 1700000000) {
        retry_at_ = Milliseconds() + 1000;
        return;
    }
    const auto generation = ++generation_;
    connection_lost_ = false;
    SetDeviceState(kDeviceStateConnecting);
    auto network = Board::GetInstance().GetNetwork();
    websocket_ = network ? network->CreateWebSocket(1) : nullptr;
    if (!websocket_) {
        Retry();
        return;
    }
    websocket_->OnData([this, generation](const char* data, size_t len, bool binary) {
        if (generation != generation_.load()) return;
        if (len > (binary ? 4096U : 16384U)) {
            connection_lost_ = true;
            xEventGroupSetBits(events_, kLost);
            return;
        }
        auto frame = std::make_unique<Incoming>(Incoming{generation, binary, std::string(data, len)});
        auto raw = frame.get();
        if (xQueueSend(incoming_, &raw, 0) == pdTRUE) {
            frame.release();
            xEventGroupSetBits(events_, kIncoming);
        } else if (!binary) {
            // Control messages cannot be dropped while continuing the same stream.
            connection_lost_ = true;
            xEventGroupSetBits(events_, kLost);
        }
    });
    std::weak_ptr<WebSocket> weak = websocket_;
    websocket_->OnPing([this, weak, generation](const char* data, size_t len) {
        if (len > 125) return;
        Schedule([this, weak, generation, payload = std::string(data, len)]() {
            if (generation != generation_.load()) return;
            if (auto socket = weak.lock()) socket->SendPong(payload.data(), payload.size());
        });
    });
    websocket_->OnDisconnected([this, generation]() {
        if (generation != generation_.load()) return;
        connection_lost_ = true;
        xEventGroupSetBits(events_, kLost);
    });
    if (!websocket_->Connect(url.c_str())) {
        Retry();
        return;
    }
    std::vector<std::string> capabilities = {"mic", "speaker", "wake", "mcp"};
    if (Board::GetInstance().GetLed()->SupportsProtocolColor()) capabilities.push_back("light");
#if CONFIG_PALPABLE_HAPTIC_GPIO >= 0
    capabilities.push_back("haptic");
#endif
#if CONFIG_PALPABLE_LOCATION_SENSOR
    capabilities.push_back("location");
#endif
#if CONFIG_PALPABLE_PROXIMITY_SENSOR
    capabilities.push_back("proximity");
#endif
    hello_deadline_ = Milliseconds() + 10000;
    if (!client_->Open(SystemInfo::GetMacAddress(), CONFIG_PALPABLE_SKIN, capabilities)) Retry();
}

void Application::CloseLink() {
    ++generation_;
    audio_service_.EnableVoiceProcessing(false);
    audio_service_.EnableWakeWordDetection(false);
    client_->Close();
    websocket_.reset();
    Incoming* raw = nullptr;
    while (xQueueReceive(incoming_, &raw, 0) == pdTRUE) delete raw;
    while (audio_service_.PopPacketFromSendQueue()) {}
    talking_ = false;
    active_source_.clear();
    channel_name_.clear();
    channel_reported_ = false;
    light_state_ = "idle";
    light_color_.clear();
    light_pattern_ = "solid";
    haptic_start_ = 0;
    TickHaptic();
    SetDeviceState(kDeviceStateIdle);
}

void Application::Retry() {
    CloseLink();
    retry_at_ = Milliseconds() + retry_seconds_ * 1000;
    retry_seconds_ = std::min(retry_seconds_ * 2, 30);
    Board::GetInstance().GetDisplay()->SetStatus("小伙伴正在重新连接");
}

void Application::HandleMessage(const cJSON* root) {
    const std::string type = Text(root, "type");
    auto display = Board::GetInstance().GetDisplay();
    if (type == "hello.ack") {
        retry_seconds_ = 1;
        Board::GetInstance().SetPowerSaveLevel(PowerSaveLevel::PERFORMANCE);
        audio_service_.EnableVoiceProcessing(true);
        audio_service_.EnableWakeWordDetection(true);
        ApplyLight("idle");
        display->SetStatus("小伙伴准备好啦");
    } else if (type == "light") {
        light_color_ = Text(root, "color");
        light_pattern_ = Text(root, "pattern");
        if (light_pattern_.empty()) light_pattern_ = "solid";
        ApplyLight(Text(root, "state"));
    } else if (type == "speaker.state") {
        const std::string source = Text(root, "source");
        if (std::string(Text(root, "state")) == "start") {
            active_source_ = source;
            SetDeviceState(kDeviceStateSpeaking);
            if (source == "peer") display->SetChatMessage("assistant", Text(root, "name"));
        } else if (active_source_ == source) {
            active_source_.clear();
            SetDeviceState(kDeviceStateListening);
        }
    } else if (type == "haptic") {
        haptic_pattern_ = Text(root, "pattern");
        haptic_start_ = Milliseconds();
    } else if (type == "sound") {
        const std::string cue = Text(root, "name");
        if (cue == "error" || cue == "busy") PlaySound(Lang::Sounds::OGG_EXCLAMATION);
        else PlaySound(Lang::Sounds::OGG_POPUP);
    } else if (type == "channel.state") {
        channel_reported_ = std::string(Text(root, "event")) != "left";
        channel_name_ = channel_reported_ ? Text(root, "name") : "";
        display->ShowNotification(channel_reported_ ? channel_name_ : "已经离开频道啦");
    } else if (type == "friend.state") {
        const std::string event = Text(root, "event");
        display->ShowNotification(event == "pending" ? "正在等家长确认小伙伴" :
                                  event == "confirmed" ? "交到新朋友啦" : "好友关系已经更新啦");
    } else if (type == "presence") {
        display->ShowNotification(std::string(Text(root, "event")) == "join" ?
                                  "有小伙伴来啦" : "小伙伴离开啦");
    } else if (type == "mcp") {
        McpServer::GetInstance().ParseMessage(palpable::schema::Item(root, "payload"));
    } else if (type == "error") {
        display->ShowNotification("小伙伴遇到一点问题，请稍后再试");
        ESP_LOGW("Palpable", "Server error code: %s", Text(root, "code"));
    }
}

bool Application::SetDeviceState(DeviceState state) {
    state_ = state;
    Board::GetInstance().GetLed()->OnStateChanged();
    return true;
}

void Application::ApplyLight(const char* state) {
    light_state_ = state;
    SetDeviceState(light_state_ == "speaking" || light_state_ == "peer_talking" ?
                   kDeviceStateSpeaking : light_state_ == "listening" ?
                   kDeviceStateListening : kDeviceStateIdle);
    next_light_at_ = 0;
}

void Application::TickLight() {
    if (!client_->Ready() || Milliseconds() < next_light_at_) return;
    next_light_at_ = Milliseconds() + 50;
    const char* default_color = light_state_ == "listening" ? "#4caf50" :
                                light_state_ == "thinking" ? "#ffb74d" :
                                light_state_ == "speaking" ? "#64b5f6" :
                                light_state_ == "peer_talking" ? "#ba68c8" :
                                light_state_ == "muted" ? "#ef5350" : "#000000";
    const char* color = light_color_.empty() ? default_color : light_color_.c_str();
    auto rgb = std::strtoul(color + 1, nullptr, 16);
    double brightness = 0.15;
    if (light_pattern_ == "breathe") brightness *= 0.15 + 0.85 * (1 + std::sin(Milliseconds() / 600.0)) / 2;
    if (light_pattern_ == "pulse" && (Milliseconds() / 250) % 2) brightness = 0;
    Board::GetInstance().GetLed()->SetProtocolColor(
        static_cast<uint8_t>(((rgb >> 16) & 0xff) * brightness),
        static_cast<uint8_t>(((rgb >> 8) & 0xff) * brightness),
        static_cast<uint8_t>((rgb & 0xff) * brightness));
}

void Application::TickHaptic() {
#if CONFIG_PALPABLE_HAPTIC_GPIO >= 0
    auto elapsed = Milliseconds() - haptic_start_;
    bool on = haptic_start_ != 0 && (haptic_pattern_ == "long" ? elapsed < 400 :
              haptic_pattern_ == "double_tap" ? elapsed < 100 || (elapsed >= 200 && elapsed < 300) : elapsed < 120);
    gpio_set_level(static_cast<gpio_num_t>(CONFIG_PALPABLE_HAPTIC_GPIO), on ? 1 : 0);
    if (elapsed >= 400) haptic_start_ = 0;
#endif
}

void Application::ToggleChatState() { StartListening(); }
void Application::StartListening() { Schedule([this]() { client_->Wake("touch"); }); }
void Application::StopListening() {}
void Application::WakeWordInvoke(const std::string&) { StartListening(); }
void Application::AbortSpeaking(AbortReason) { StartListening(); }
void Application::ResetProtocol() { Schedule([this]() { Retry(); }); }
void Application::Reboot() { esp_restart(); }
bool Application::UpgradeFirmware(const std::string&, const std::string&) { return false; }
bool Application::CanEnterSleepMode() { return !network_up_ && audio_service_.IsIdle(); }
void Application::SetAecMode(AecMode mode) {
    Schedule([this, mode]() {
        aec_mode_ = mode == kAecOnDeviceSide ? kAecOnDeviceSide : kAecOff;
        audio_service_.EnableDeviceAec(aec_mode_ == kAecOnDeviceSide);
    });
}
void Application::PlaySound(const std::string_view& sound) { audio_service_.PlaySound(sound); }
void Application::SendMcpMessage(const std::string& payload) {
    Schedule([this, payload]() {
        client_->Mcp(payload);
        if (mcp_broadcast_callback_) mcp_broadcast_callback_(payload);
    });
}
void Application::RegisterMcpBroadcastCallback(std::function<void(const std::string&)> callback) {
    mcp_broadcast_callback_ = std::move(callback);
}
void Application::Alert(const char* status, const char* message, const char* emotion, const std::string_view& sound) {
    auto display = Board::GetInstance().GetDisplay();
    display->SetStatus(status);
    display->ShowNotification(message);
    if (emotion && *emotion) display->SetEmotion(emotion);
    if (!sound.empty()) PlaySound(sound);
}
void Application::DismissAlert() { Board::GetInstance().GetDisplay()->SetStatus("小伙伴准备好啦"); }

void Application::ReportLocation(double lat, double lng) {
#if CONFIG_PALPABLE_LOCATION_SENSOR
    Schedule([this, lat, lng]() {
        if (Milliseconds() - last_location_at_ < 1000) return;
        if (client_->Location(lat, lng)) last_location_at_ = Milliseconds();
    });
#endif
}
void Application::ReportProximity(std::vector<palpable::Peer> peers) {
#if CONFIG_PALPABLE_PROXIMITY_SENSOR
    Schedule([this, peers = std::move(peers)]() {
        if (Milliseconds() - last_proximity_at_ < 500) return;
        if (client_->Proximity(peers)) last_proximity_at_ = Milliseconds();
    });
#endif
}
