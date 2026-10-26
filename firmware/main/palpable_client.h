#pragma once

#include <cJSON.h>
#include <cstddef>
#include <cstdint>
#include <deque>
#include <functional>
#include <string>
#include <vector>

namespace palpable {
using Bytes = std::vector<uint8_t>;

struct Callbacks {
    std::function<bool(const std::string&)> send_text;
    std::function<bool(const Bytes&)> send_audio;
    std::function<void(const cJSON*)> message;
    std::function<void(const Bytes&, int sample_rate, int frame_duration)> audio;
    std::function<void()> flush_audio;
};

struct Peer {
    std::string device_id;
    double strength;
};

// Call only from the application task; transport and audio workers enqueue events.
class Client {
public:
    explicit Client(Callbacks callbacks);
    bool Open(const std::string& mac, const std::string& skin,
              const std::vector<std::string>& capabilities);
    void Close();
    bool ReceiveText(const std::string& text);
    void ReceiveAudio(const Bytes& bytes);
    bool Ready() const { return ready_; }
    bool Failed() const { return failed_; }
    bool Talking() const { return talking_; }
    bool Wake(const char* source);
    bool Mcp(const std::string& payload);
    bool Location(double lat, double lng);
    bool Proximity(const std::vector<Peer>& peers);
    bool Microphone(const Bytes& opus, bool speech);
    void SuspendMicrophone();
    static bool IsSpeech(const int16_t* samples, size_t count, int threshold);

private:
    Callbacks callbacks_;
    bool ready_ = false;
    bool failed_ = false;
    bool talking_ = false;
    int silence_frames_ = 0;
    int ai_sample_rate_ = 16000;
    int ai_frame_duration_ = 60;
    std::string source_;
    std::deque<Bytes> preroll_;

    bool Send(cJSON* root, bool handshake = false);
    bool SendAudio(const Bytes& bytes);
    bool MicState(const char* state);
};
}  // namespace palpable
