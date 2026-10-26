#include "palpable_client.h"
#include "palpable_schema.h"

#include <memory>
#include <utility>

namespace palpable {
namespace {
using Json = std::unique_ptr<cJSON, decltype(&cJSON_Delete)>;

cJSON* Message(const char* type) {
    auto root = cJSON_CreateObject();
    cJSON_AddStringToObject(root, "type", type);
    return root;
}

const char* Text(const cJSON* root, const char* key) {
    auto value = schema::Item(root, key);
    return cJSON_IsString(value) ? value->valuestring : "";
}

cJSON* AudioParams() {
    auto params = cJSON_CreateObject();
    cJSON_AddStringToObject(params, "format", "opus");
    cJSON_AddNumberToObject(params, "sample_rate", 16000);
    cJSON_AddNumberToObject(params, "channels", 1);
    cJSON_AddNumberToObject(params, "frame_duration", 60);
    return params;
}
}  // namespace

Client::Client(Callbacks callbacks) : callbacks_(std::move(callbacks)) {}

bool Client::Open(const std::string& mac, const std::string& skin,
                  const std::vector<std::string>& capabilities) {
    Close();
    auto root = Message("hello");
    cJSON_AddStringToObject(root, "device_id", mac.c_str());
    cJSON_AddStringToObject(root, "skin", skin.c_str());
    cJSON_AddStringToObject(root, "firmware", "palpable-esp32-0.1");
    cJSON_AddItemToObject(root, "audio_params", AudioParams());
    auto caps = cJSON_AddArrayToObject(root, "capabilities");
    for (const auto& capability : capabilities) {
        cJSON_AddItemToArray(caps, cJSON_CreateString(capability.c_str()));
    }
    return Send(root, true);
}

void Client::Close() {
    ready_ = false;
    failed_ = false;
    talking_ = false;
    silence_frames_ = 0;
    ai_sample_rate_ = 16000;
    ai_frame_duration_ = 60;
    source_.clear();
    preroll_.clear();
    if (callbacks_.flush_audio) callbacks_.flush_audio();
}

bool Client::Send(cJSON* value, bool handshake) {
    Json root(value, cJSON_Delete);
    if ((!handshake && !ready_) || failed_ || !schema::Message(root.get())) return false;
    std::unique_ptr<char, decltype(&cJSON_free)> text(cJSON_PrintUnformatted(root.get()), cJSON_free);
    if (!text || !callbacks_.send_text || !callbacks_.send_text(text.get())) {
        failed_ = true;
        return false;
    }
    return true;
}

bool Client::ReceiveText(const std::string& text) {
    if (text.size() > 16384 || text.find('\0') != std::string::npos) return false;
    const char* end = nullptr;
    Json root(cJSON_ParseWithLengthOpts(text.c_str(), text.size() + 1, &end, true), cJSON_Delete);
    if (!root || !schema::Message(root.get())) return false;
    auto type = Text(root.get(), "type");
    if (std::strcmp(type, "hello.ack") == 0) {
        if (ready_) return false;
        ready_ = true;
    } else {
        if (!ready_ && std::strcmp(type, "error") != 0) return false;
        if (std::strcmp(type, "audio.downlink") == 0) {
            ai_sample_rate_ = schema::Item(root.get(), "sample_rate")->valueint;
            ai_frame_duration_ = schema::Item(root.get(), "frame_duration")->valueint;
        } else if (std::strcmp(type, "speaker.state") == 0) {
            const auto source = Text(root.get(), "source");
            if (schema::StringEquals(schema::Item(root.get(), "state"), "start")) {
                if (source_ != source && callbacks_.flush_audio) callbacks_.flush_audio();
                source_ = source;
            } else if (source_ == source) {
                if (source_ == "peer" && callbacks_.flush_audio) callbacks_.flush_audio();
                source_.clear();
            }
        } else if (std::strcmp(type, "light") != 0 && std::strcmp(type, "haptic") != 0 &&
                   std::strcmp(type, "sound") != 0 && std::strcmp(type, "presence") != 0 &&
                   std::strcmp(type, "channel.state") != 0 && std::strcmp(type, "friend.state") != 0 &&
                   std::strcmp(type, "mcp") != 0 && std::strcmp(type, "error") != 0) {
            return false;
        }
    }
    if (callbacks_.message) callbacks_.message(root.get());
    return true;
}

void Client::ReceiveAudio(const Bytes& bytes) {
    if (!ready_ || bytes.empty() || bytes.size() > 4096 || !callbacks_.audio) return;
    const bool peer = source_ == "peer";
    callbacks_.audio(bytes, peer ? 16000 : ai_sample_rate_, peer ? 60 : ai_frame_duration_);
}

bool Client::Wake(const char* source) {
    auto root = Message("wake");
    cJSON_AddStringToObject(root, "source", source);
    return Send(root);
}

bool Client::Mcp(const std::string& payload) {
    if (payload.size() > 16000 || payload.find('\0') != std::string::npos) return false;
    auto value = cJSON_ParseWithLengthOpts(payload.c_str(), payload.size() + 1, nullptr, true);
    if (!cJSON_IsObject(value)) {
        cJSON_Delete(value);
        return false;
    }
    auto root = Message("mcp");
    cJSON_AddItemToObject(root, "payload", value);
    return Send(root);
}

bool Client::Location(double lat, double lng) {
    auto root = Message("location");
    cJSON_AddNumberToObject(root, "lat", lat);
    cJSON_AddNumberToObject(root, "lng", lng);
    return Send(root);
}

bool Client::Proximity(const std::vector<Peer>& peers) {
    if (peers.size() > 16) return false;
    auto root = Message("proximity");
    auto list = cJSON_AddArrayToObject(root, "peers");
    for (const auto& peer : peers) {
        auto item = cJSON_CreateObject();
        cJSON_AddStringToObject(item, "device_id", peer.device_id.c_str());
        cJSON_AddNumberToObject(item, "strength", peer.strength);
        cJSON_AddItemToArray(list, item);
    }
    return Send(root);
}

bool Client::MicState(const char* state) {
    auto root = Message("mic.state");
    cJSON_AddStringToObject(root, "state", state);
    return Send(root);
}

bool Client::SendAudio(const Bytes& bytes) {
    if (!callbacks_.send_audio || !callbacks_.send_audio(bytes)) {
        failed_ = true;
        return false;
    }
    return true;
}

bool Client::Microphone(const Bytes& opus, bool speech) {
    if (!ready_ || failed_ || opus.empty() || opus.size() > 4096) return false;
    if (!talking_ && !speech) {
        preroll_.push_back(opus);
        if (preroll_.size() > 3) preroll_.pop_front();
        return true;
    }
    if (!talking_) {
        if (!MicState("start")) return false;
        talking_ = true;
        for (const auto& frame : preroll_) {
            if (!SendAudio(frame)) return false;
        }
        preroll_.clear();
    }
    if (speech) silence_frames_ = 8;
    if (!SendAudio(opus)) return false;
    if (!speech && --silence_frames_ <= 0) SuspendMicrophone();
    return !failed_;
}

void Client::SuspendMicrophone() {
    if (talking_ && ready_ && !failed_) MicState("stop");
    talking_ = false;
    silence_frames_ = 0;
    preroll_.clear();
}

bool Client::IsSpeech(const int16_t* samples, size_t count, int threshold) {
    if (!samples || count == 0 || threshold <= 0) return false;
    uint64_t energy = 0;
    for (size_t i = 0; i < count; ++i) {
        const int64_t sample = samples[i];
        energy += static_cast<uint64_t>(sample * sample);
    }
    return energy / count >= static_cast<uint64_t>(threshold) * threshold;
}
}  // namespace palpable
