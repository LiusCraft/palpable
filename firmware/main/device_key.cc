#include "device_key.h"
#include "settings.h"

#include <esp_log.h>
#include <esp_random.h>
#include <mbedtls/base64.h>
#include <mbedtls/ecdsa.h>
#include <mbedtls/pk.h>
#include <mbedtls/sha256.h>

#include <utility>
#include <vector>

#define TAG "DeviceKey"

namespace {
constexpr const char* kNamespace = "palpable";
constexpr const char* kKey = "device_key";

// 硬件 RNG；射频开启前也有 bootloader 播种的熵，足够生成密钥与签名随机数。
int Random(void*, unsigned char* buffer, size_t length) {
    esp_fill_random(buffer, length);
    return 0;
}

std::string Base64(const unsigned char* data, size_t length, bool url) {
    std::vector<unsigned char> out(4 * ((length + 2) / 3) + 1);
    size_t written = 0;
    if (mbedtls_base64_encode(out.data(), out.size(), &written, data, length) != 0) return "";
    std::string text(reinterpret_cast<char*>(out.data()), written);
    if (url) {
        for (auto& c : text) {
            if (c == '+') c = '-';
            else if (c == '/') c = '_';
        }
        while (!text.empty() && text.back() == '=') text.pop_back();
    }
    return text;
}
}  // namespace

struct DeviceKey::Impl {
    mbedtls_pk_context pk;
    Impl() { mbedtls_pk_init(&pk); }
    ~Impl() { mbedtls_pk_free(&pk); }
};

DeviceKey::DeviceKey(std::unique_ptr<Impl> impl, std::string public_key)
    : impl_(std::move(impl)), public_key_(std::move(public_key)) {}

DeviceKey::~DeviceKey() = default;

std::unique_ptr<DeviceKey> DeviceKey::LoadOrCreate() {
    auto impl = std::make_unique<Impl>();
    std::string pem = Settings(kNamespace, false).GetString(kKey);
    bool loaded = false;
    if (!pem.empty()) {
        loaded = mbedtls_pk_parse_key(&impl->pk, reinterpret_cast<const unsigned char*>(pem.c_str()),
                                      pem.size() + 1, nullptr, 0, Random, nullptr) == 0 &&
                 mbedtls_pk_get_type(&impl->pk) == MBEDTLS_PK_ECKEY &&
                 mbedtls_pk_ec(impl->pk)->MBEDTLS_PRIVATE(grp).id == MBEDTLS_ECP_DP_SECP256R1;
        if (!loaded) {
            // 不悄悄换钥：已登记的设备换了钥匙会被服务端拒绝，必须让人看见。
            ESP_LOGE(TAG, "Stored device key is unreadable; refusing to replace it");
            return nullptr;
        }
    }
    if (!loaded) {
        if (mbedtls_pk_setup(&impl->pk, mbedtls_pk_info_from_type(MBEDTLS_PK_ECKEY)) != 0 ||
            mbedtls_ecp_gen_key(MBEDTLS_ECP_DP_SECP256R1, mbedtls_pk_ec(impl->pk), Random, nullptr) != 0) {
            ESP_LOGE(TAG, "Failed to generate device key");
            return nullptr;
        }
        unsigned char buffer[512] = {};
        if (mbedtls_pk_write_key_pem(&impl->pk, buffer, sizeof(buffer)) != 0) {
            ESP_LOGE(TAG, "Failed to encode device key");
            return nullptr;
        }
        Settings(kNamespace, true).SetString(kKey, reinterpret_cast<char*>(buffer));
        ESP_LOGI(TAG, "Generated new device key");
    }

    // mbedtls_pk_write_pubkey_der 从缓冲区末尾往前写。
    unsigned char der[160] = {};
    const int length = mbedtls_pk_write_pubkey_der(&impl->pk, der, sizeof(der));
    if (length <= 0) {
        ESP_LOGE(TAG, "Failed to encode public key");
        return nullptr;
    }
    auto public_key = Base64(der + sizeof(der) - length, length, false);
    if (public_key.empty()) return nullptr;
    return std::unique_ptr<DeviceKey>(new DeviceKey(std::move(impl), std::move(public_key)));
}

std::string DeviceKey::Sign(const std::string& message) const {
    unsigned char hash[32];
    if (mbedtls_sha256(reinterpret_cast<const unsigned char*>(message.data()), message.size(), hash, 0) != 0) {
        return "";
    }
    auto* keypair = mbedtls_pk_ec(impl_->pk);
    mbedtls_mpi r, s;
    mbedtls_mpi_init(&r);
    mbedtls_mpi_init(&s);
    unsigned char signature[64];
    const bool ok =
        mbedtls_ecdsa_sign(&keypair->MBEDTLS_PRIVATE(grp), &r, &s, &keypair->MBEDTLS_PRIVATE(d), hash,
                           sizeof(hash), Random, nullptr) == 0 &&
        mbedtls_mpi_write_binary(&r, signature, 32) == 0 && mbedtls_mpi_write_binary(&s, signature + 32, 32) == 0;
    mbedtls_mpi_free(&r);
    mbedtls_mpi_free(&s);
    return ok ? Base64(signature, sizeof(signature), true) : "";
}
