#include "palpable_client.h"

#include <iostream>
#include <string>

int main() {
    palpable::Client client(palpable::Callbacks{
        .send_text = [](const std::string& text) {
            std::cout << text << std::endl;
            return true;
        },
        .send_audio = [](const palpable::Bytes&) {
            std::cout << "audio" << std::endl;
            return true;
        },
    });
    if (!client.Open("02:ca:fe:00:00:01", "dragon", {"mic", "speaker", "wake"})) return 1;
    std::string ack;
    if (!std::getline(std::cin, ack) || !client.ReceiveText(ack) || !client.Ready()) return 2;
    if (!client.Wake("touch") || !client.Microphone({0xf8, 0xff, 0xfe}, true)) return 3;
    client.SuspendMicrophone();
    return client.Failed() ? 4 : 0;
}
