#include "palpable_client.h"
#include "palpable_schema.h"

#include <cassert>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <iterator>
#include <limits>
#include <string>
#include <vector>

namespace {
constexpr const char* kAck = R"({"type":"hello.ack","session_id":"test","audio_params":{"format":"opus","sample_rate":16000,"channels":1,"frame_duration":60}})";

struct Harness {
    std::vector<std::string> sent;
    std::vector<std::string> received;
    int sample_rate = 0;
    int frame_duration = 0;
    int flushes = 0;
    bool send_ok = true;
    palpable::Client client{palpable::Callbacks{
        .send_text = [this](const std::string& text) {
            auto value = cJSON_Parse(text.c_str());
            assert(palpable::schema::Message(value));
            cJSON_Delete(value);
            sent.push_back(text);
            return send_ok;
        },
        .send_audio = [this](const palpable::Bytes& bytes) {
            sent.push_back("audio:" + std::to_string(bytes.front()));
            return send_ok;
        },
        .message = [this](const cJSON* root) {
            received.push_back(palpable::schema::Item(root, "type")->valuestring);
        },
        .audio = [this](const palpable::Bytes&, int rate, int duration) {
            sample_rate = rate;
            frame_duration = duration;
        },
        .flush_audio = [this]() { ++flushes; },
    }};
    void Open() {
        assert(client.Open("aa:bb:cc:dd:ee:01", "dragon", {"mic", "speaker", "wake", "mcp"}));
    }
    void Ready() {
        Open();
        assert(client.ReceiveText(kAck));
        assert(client.Ready());
    }
};

void Fixtures(const char* directory) {
    size_t count = 0;
    for (const auto& entry : std::filesystem::directory_iterator(directory)) {
        if (entry.path().extension() != ".json") continue;
        std::ifstream input(entry.path());
        const std::string text{std::istreambuf_iterator<char>(input), std::istreambuf_iterator<char>()};
        auto value = cJSON_Parse(text.c_str());
        const bool expected = entry.path().filename().string().find("invalid-") != 0;
        assert(palpable::schema::Message(value) == expected);
        cJSON_Delete(value);
        ++count;
    }
    assert(count >= 10);
    std::cout << count << " schema fixtures passed\n";
}

void Handshake() {
    Harness h;
    assert(!h.client.Wake("voice"));
    assert(!h.client.Microphone({1}, true));
    h.client.ReceiveAudio({1});
    assert(h.sample_rate == 0);
    h.Open();
    assert(!h.client.Ready());
    assert(h.sent.size() == 1);
    assert(h.sent[0].find("session_id") == std::string::npos);
    assert(h.sent[0].find("features") == std::string::npos);
    assert(!h.client.ReceiveText(R"({"type":"hello","transport":"websocket"})"));
    assert(!h.client.ReceiveText(R"({"type":"audio.downlink","sample_rate":24000,"frame_duration":60})"));
    assert(h.client.ReceiveText(kAck));
    assert(!h.client.ReceiveText(kAck));
    assert(h.client.Wake("voice"));
    assert(h.client.Wake("touch"));
    assert(!h.client.Wake("button"));
    h.client.Close();
    assert(!h.client.Ready());
    assert(!h.client.Wake("voice"));
    h.Ready();
    h.client.ReceiveAudio({1});
    assert(h.sample_rate == 16000 && h.frame_duration == 60);
    assert(!h.client.Open("AA:BB:CC:DD:EE:01", "dragon", {"mic"}));
    assert(!h.client.Open("aa:bb:cc:dd:ee:01", "dragon", {"mic", "mic"}));
}

void VadOrdering() {
    Harness h;
    h.Ready();
    h.sent.clear();
    for (uint8_t i = 1; i <= 5; ++i) assert(h.client.Microphone({i}, false));
    assert(h.sent.empty());
    assert(h.client.Microphone({6}, true));
    assert(h.sent.size() == 5);
    assert(h.sent[0] == R"({"type":"mic.state","state":"start"})");
    assert(h.sent[1] == "audio:3" && h.sent[2] == "audio:4" && h.sent[3] == "audio:5");
    assert(h.sent[4] == "audio:6");
    for (uint8_t i = 7; i < 14; ++i) {
        assert(h.client.Microphone({i}, false));
        assert(h.client.Talking());
    }
    assert(h.client.Microphone({14}, false));
    assert(!h.client.Talking());
    assert(h.sent[h.sent.size() - 2] == "audio:14");
    assert(h.sent.back() == R"({"type":"mic.state","state":"stop"})");
    h.sent.clear();
    assert(h.client.Microphone({15}, true));
    h.client.SuspendMicrophone();
    assert(h.sent.size() == 3);
    assert(!h.client.Talking());

    const int16_t quiet[] = {0, 100, -100};
    const int16_t speech[] = {1000, -1000, 1000};
    const int16_t extremes[] = {-32768, 32767};
    assert(!palpable::Client::IsSpeech(quiet, 3, 500));
    assert(palpable::Client::IsSpeech(speech, 3, 500));
    assert(palpable::Client::IsSpeech(extremes, 2, 500));
    assert(!palpable::Client::IsSpeech(nullptr, 0, 500));
}

void PlaybackSources() {
    Harness h;
    h.Ready();
    assert(h.client.ReceiveText(R"({"type":"audio.downlink","sample_rate":24000,"frame_duration":40})"));
    assert(h.client.ReceiveText(R"({"type":"speaker.state","state":"start","source":"ai"})"));
    h.client.ReceiveAudio({1});
    assert(h.sample_rate == 24000 && h.frame_duration == 40);
    const int before_peer = h.flushes;
    assert(h.client.ReceiveText(R"({"type":"speaker.state","state":"start","source":"peer","from":"aa:bb:cc:dd:ee:02"})"));
    assert(h.flushes == before_peer + 1);
    h.client.ReceiveAudio({2});
    assert(h.sample_rate == 16000 && h.frame_duration == 60);
    assert(h.client.ReceiveText(R"({"type":"speaker.state","state":"stop","source":"ai"})"));
    h.client.ReceiveAudio({2});
    assert(h.sample_rate == 16000);
    assert(h.client.ReceiveText(R"({"type":"speaker.state","state":"stop","source":"peer"})"));
    assert(h.flushes == before_peer + 2);
    assert(h.client.ReceiveText(R"({"type":"speaker.state","state":"start","source":"ai"})"));
    const int before_stop = h.flushes;
    assert(h.client.ReceiveText(R"({"type":"speaker.state","state":"stop","source":"ai"})"));
    assert(h.flushes == before_stop);
    h.client.ReceiveAudio({3});
    assert(h.sample_rate == 24000);
    h.Ready();
    h.client.ReceiveAudio({4});
    assert(h.sample_rate == 16000 && h.frame_duration == 60);
}

void ValidationAndTools() {
    Harness h;
    h.Ready();
    for (auto text : {"null", "[]", "{}", "{", R"({"type":"unknown"})",
                      R"({"type":"wake","source":"touch"})",
                      R"({"type":"sound","name":"connected","extra":1})",
                      R"({"type":"sound","name":"connected"} {})",
                      R"({"type":"sound","type":"sound","name":"busy"})",
                      R"({"Type":"sound","name":"busy"})",
                      R"({"type":"audio.downlink","sample_rate":24000.5,"frame_duration":60})",
                      R"({"type":"audio.downlink","sample_rate":48000,"frame_duration":60})",
                      R"({"type":"light","state":"idle","color":"#GG0000"})"}) {
        assert(!h.client.ReceiveText(text));
    }
    assert(h.client.ReceiveText(R"({"type":"light","state":"thinking","color":"#Aa00fF","pattern":"breathe"})"));
    assert(h.client.ReceiveText(R"({"type":"haptic","pattern":"double_tap"})"));
    assert(h.client.ReceiveText(R"({"type":"sound","name":"busy"})"));
    assert(h.client.ReceiveText(R"({"type":"channel.state","event":"joined","channel":"zone:park","name":"公园"})"));
    assert(h.client.ReceiveText(R"({"type":"friend.state","event":"pending","device_id":"aa:bb:cc:dd:ee:02"})"));
    assert(h.client.ReceiveText(R"({"type":"presence","event":"join","device_id":"aa:bb:cc:dd:ee:02","channel":"zone:park"})"));
    assert(h.client.ReceiveText(R"({"type":"mcp","payload":{"jsonrpc":"2.0","id":1,"method":"tools/list"}})"));
    assert(h.client.ReceiveText(R"({"type":"error","code":"internal","message":"try again"})"));
    assert(h.client.Mcp(R"({"jsonrpc":"2.0","id":1,"result":{"tools":[]}})"));
    assert(!h.client.Mcp("[]"));
    assert(!h.client.Mcp("{} {}"));
    assert(h.client.Location(31, 121));
    assert(!h.client.Location(91, 121));
    assert(!h.client.Location(std::numeric_limits<double>::infinity(), 121));
    assert(h.client.Proximity({{"aa:bb:cc:dd:ee:02", 0.9}}));
    assert(h.client.Proximity({}));
    assert(!h.client.Proximity({{"aa:bb:cc:dd:ee:02", 1.1}}));
    assert(!h.client.Proximity(std::vector<palpable::Peer>(17, {"peer", 0.5})));
}

void FailureAndReconnect() {
    Harness h;
    h.Ready();
    h.send_ok = false;
    assert(!h.client.Microphone({1}, true));
    assert(h.client.Failed());
    h.send_ok = true;
    h.Ready();
    assert(!h.client.Failed());
    assert(h.client.Microphone({2}, true));
    h.send_ok = false;
    assert(!h.client.Microphone({3}, true));
    assert(h.client.Failed());
    h.client.Close();
    h.send_ok = true;
    h.Ready();
    h.sent.clear();
    assert(h.client.Microphone({4}, true));
    assert(h.sent.size() == 2);
    assert(h.sent[1] == "audio:4");
}
}  // namespace

int main(int argc, char** argv) {
    assert(argc == 2);
    Fixtures(argv[1]);
    Handshake();
    VadOrdering();
    PlaybackSources();
    ValidationAndTools();
    FailureAndReconnect();
    std::cout << "Handshake, VAD ordering, audio switching, validation, MCP and reconnect passed\n";
}
