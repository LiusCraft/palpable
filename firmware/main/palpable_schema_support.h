#pragma once

#include <cJSON.h>
#include <cmath>
#include <cstddef>
#include <cstring>

namespace palpable::schema {
inline const cJSON* Item(const cJSON* root, const char* key) {
    return cJSON_GetObjectItemCaseSensitive(root, key);
}
inline bool StringEquals(const cJSON* value, const char* text) {
    return cJSON_IsString(value) && std::strcmp(value->valuestring, text) == 0;
}
inline bool UniqueKeys(const cJSON* root) {
    for (auto child = root->child; child; child = child->next) {
        for (auto other = child->next; other; other = other->next) {
            if (std::strcmp(child->string, other->string) == 0) return false;
        }
    }
    return true;
}
inline size_t Utf8Length(const char* text) {
    size_t count = 0;
    for (auto p = reinterpret_cast<const unsigned char*>(text); *p; ++p) {
        if ((*p & 0xc0) != 0x80) ++count;
    }
    return count;
}
inline bool LowerHex(char ch) {
    return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f');
}
inline bool MatchMac(const char* text) {
    if (std::strlen(text) != 17) return false;
    for (size_t i = 0; i < 17; ++i) {
        if (i % 3 == 2 ? text[i] != ':' : !LowerHex(text[i])) return false;
    }
    return true;
}
inline bool MatchColor(const char* text) {
    if (std::strlen(text) != 7 || text[0] != '#') return false;
    for (size_t i = 1; i < 7; ++i) {
        if (!LowerHex(text[i]) && !(text[i] >= 'A' && text[i] <= 'F')) return false;
    }
    return true;
}
inline bool MatchSkin(const char* text) {
    auto length = std::strlen(text);
    if (length < 1 || length > 32) return false;
    for (size_t i = 0; i < length; ++i) {
        auto ch = text[i];
        if (!(ch >= 'a' && ch <= 'z') && !(ch >= '0' && ch <= '9') && ch != '_' && ch != '-') return false;
    }
    return true;
}
}  // namespace palpable::schema
