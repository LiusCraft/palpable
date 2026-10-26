#pragma once

#include <freertos/FreeRTOS.h>
#include <freertos/event_groups.h>
#include <freertos/queue.h>
#include <freertos/task.h>
#include <atomic>
#include <deque>
#include <functional>
#include <memory>
#include <mutex>
#include <string>
#include <string_view>

#include "audio_service.h"
#include "device_state.h"
#include "palpable_client.h"
#include <web_socket.h>

enum AecMode { kAecOff, kAecOnDeviceSide, kAecOnServerSide };

class Application {
public:
    static Application& GetInstance() {
        static Application instance;
        return instance;
    }
    Application(const Application&) = delete;
    Application& operator=(const Application&) = delete;
    void Initialize();
    void Run();
    DeviceState GetDeviceState() const { return state_.load(); }
    bool SetDeviceState(DeviceState state);
    bool IsVoiceDetected() const { return talking_.load(); }
    void Schedule(std::function<void()>&& callback);
    void Alert(const char* status, const char* message, const char* emotion = "",
               const std::string_view& sound = "");
    void DismissAlert();
    void ToggleChatState();
    void StartListening();
    void StopListening();
    void WakeWordInvoke(const std::string& wake_word);
    void AbortSpeaking(AbortReason reason);
    void Reboot();
    bool UpgradeFirmware(const std::string& url, const std::string& version = "");
    bool CanEnterSleepMode();
    void SendMcpMessage(const std::string& payload);
    void RegisterMcpBroadcastCallback(std::function<void(const std::string&)> callback);
    void SetAecMode(AecMode mode);
    AecMode GetAecMode() const { return aec_mode_.load(); }
    void PlaySound(const std::string_view& sound);
    AudioService& GetAudioService() { return audio_service_; }
    void ResetProtocol();
    void ReportLocation(double lat, double lng);
    void ReportProximity(std::vector<palpable::Peer> peers);

private:
    Application();
    struct Incoming {
        uint32_t generation;
        bool binary;
        std::string data;
    };
    EventGroupHandle_t events_;
    QueueHandle_t incoming_;
    std::mutex tasks_mutex_;
    std::deque<std::function<void()>> tasks_;
    std::shared_ptr<WebSocket> websocket_;
    std::unique_ptr<palpable::Client> client_;
    AudioService audio_service_;
    std::atomic<DeviceState> state_{kDeviceStateUnknown};
    std::atomic<AecMode> aec_mode_{kAecOff};
    std::atomic<bool> talking_{false};
    std::atomic<bool> network_up_{false};
    std::atomic<bool> connection_lost_{false};
    std::atomic<uint32_t> generation_{0};
    std::function<void(const std::string&)> mcp_broadcast_callback_;
    int retry_seconds_ = 1;
    int64_t retry_at_ = 0;
    int64_t hello_deadline_ = 0;
    int64_t haptic_start_ = 0;
    std::string haptic_pattern_;
    std::string light_state_ = "idle";
    std::string light_color_;
    std::string light_pattern_ = "solid";
    int64_t next_light_at_ = 0;
    std::string active_source_;
    std::string channel_name_;
    bool channel_reported_ = false;
    int64_t last_location_at_ = 0;
    int64_t last_proximity_at_ = 0;

    void Connect();
    void CloseLink();
    void Retry();
    void HandleMessage(const cJSON* root);
    void ApplyLight(const char* state);
    void TickLight();
    void TickHaptic();
};

class TaskPriorityReset {
public:
    explicit TaskPriorityReset(BaseType_t priority) : original_(uxTaskPriorityGet(nullptr)) {
        vTaskPrioritySet(nullptr, priority);
    }
    ~TaskPriorityReset() { vTaskPrioritySet(nullptr, original_); }
private:
    BaseType_t original_;
};
