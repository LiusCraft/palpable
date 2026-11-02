#pragma once

#include <memory>
#include <string>

// 设备身份密钥（ECDSA P-256），见 docs/trust.md §2。私钥只在设备 NVS 里，不离开设备。
class DeviceKey {
public:
    // 读取 NVS 里的私钥；首次上电时生成并保存。失败返回 nullptr。
    static std::unique_ptr<DeviceKey> LoadOrCreate();
    ~DeviceKey();

    // base64(SPKI DER)，放进 hello.pubkey。
    const std::string& PublicKey() const { return public_key_; }
    // ECDSA-P256-SHA256 签名，返回 r‖s 的 base64url（无填充）；失败返回空串。
    std::string Sign(const std::string& message) const;

private:
    struct Impl;
    explicit DeviceKey(std::unique_ptr<Impl> impl, std::string public_key);
    std::unique_ptr<Impl> impl_;
    std::string public_key_;
};
