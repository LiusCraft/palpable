#!/usr/bin/env python3
"""Prepare a pinned XiaoZhi hardware tree and apply the Palpable application."""
import argparse
import io
import json
from pathlib import Path
import shutil
import subprocess
import tarfile

from gen_schema import generate

FIRMWARE = Path(__file__).resolve().parents[1]
ROOT = FIRMWARE.parent
LOCK = json.loads((FIRMWARE / "upstream.lock.json").read_text())
DESTINATION = FIRMWARE / ".build/xiaozhi"


def run(*args, cwd=None):
    return subprocess.check_output(args, cwd=cwd, text=True).strip()


def replace(path, old, new):
    text = path.read_text()
    if text.count(old) != 1:
        raise RuntimeError(f"Pinned upstream changed: expected one match in {path}: {old!r}")
    path.write_text(text.replace(old, new))


def source_tree(local):
    source = local or FIRMWARE / ".cache/xiaozhi"
    if not source.exists():
        source.mkdir(parents=True)
        run("git", "init", "--quiet", str(source))
        run("git", "-C", str(source), "remote", "add", "origin", LOCK["repository"])
    if not local:
        exists = subprocess.run(["git", "-C", str(source), "cat-file", "-e", LOCK["revision"] + "^{commit}"],
                                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        if exists.returncode:
            subprocess.check_call(["git", "-C", str(source), "fetch", "--depth=1", "origin", LOCK["revision"]])
    else:
        revision = run("git", "-C", str(source), "rev-parse", "HEAD")
        if revision != LOCK["revision"]:
            raise RuntimeError(f"Source must be at {LOCK['revision']}, got {revision}")
    return source


def prepare(local):
    revision_file = DESTINATION / ".palpable-revision"
    if revision_file.exists() and revision_file.read_text().strip() != LOCK["revision"]:
        raise RuntimeError("Upstream revision changed; back up sdkconfig and remove .build/xiaozhi before preparing")
    source = source_tree(local)
    # Export committed files only; a supplied checkout's credentials and local files stay out.
    archive = subprocess.check_output(["git", "-C", str(source), "archive", LOCK["revision"]])
    DESTINATION.mkdir(parents=True, exist_ok=True)
    with tarfile.open(fileobj=io.BytesIO(archive)) as tree:
        for member in tree.getmembers():
            path = DESTINATION / member.name
            if not path.resolve().is_relative_to(DESTINATION.resolve()):
                raise RuntimeError(f"Unsafe archive path: {member.name}")
            if not (member.isfile() or member.isdir()):
                raise RuntimeError(f"Unsupported archive entry: {member.name}")
            if member.isdir():
                path.mkdir(parents=True, exist_ok=True)
            else:
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(tree.extractfile(member).read())
                path.chmod(member.mode)
    main = DESTINATION / "main"
    for path in (FIRMWARE / "main").iterdir():
        if path.is_file():
            shutil.copy2(path, main / path.name)
    boards = FIRMWARE / "main" / "boards"
    if boards.exists():
        shutil.copytree(boards, main / "boards", dirs_exist_ok=True)
    (main / "palpable_schema.h").write_text(generate(json.loads((ROOT / "protocol/palpable.schema.json").read_text())))

    replace(DESTINATION / "CMakeLists.txt", 'set(PROJECT_VER "2.5.3")', 'set(PROJECT_VER "0.1.0")')
    replace(DESTINATION / "CMakeLists.txt", "project(xiaozhi)", "project(palpable)")
    with (DESTINATION / "sdkconfig.defaults").open("a") as defaults:
        defaults.write("\n" + (FIRMWARE / "sdkconfig.defaults").read_text())
    replace(main / "Kconfig.projbuild", '    default "https://api.tenclass.net/xiaozhi/ota/"', '    default ""')
    cmake = main / "CMakeLists.txt"
    for name in ["protocols/protocol.cc", "protocols/mqtt_protocol.cc", "protocols/websocket_protocol.cc", "ota.cc"]:
        replace(cmake, f'            "{name}"\n', "")
    replace(cmake, '            "application.cc"', '            "application.cc"\n            "palpable_client.cc"')
    with (main / "Kconfig.projbuild").open("a") as config:
        config.write('\nrsource "Kconfig.palpable"\n')
    replace(main / "Kconfig.projbuild",
            '    config BOARD_TYPE_BREAD_COMPACT_WIFI\n'
            '        bool "Bread Compact Wi-Fi (面包板)"\n'
            '        depends on IDF_TARGET_ESP32S3\n',
            '    config BOARD_TYPE_BREAD_COMPACT_WIFI\n'
            '        bool "Bread Compact Wi-Fi (面包板)"\n'
            '        depends on IDF_TARGET_ESP32S3\n'
            '    config BOARD_TYPE_PALPABLE_WGH_C2\n'
            '        bool "Palpable WGH-C2 (ESP32-S3 1.54 LCD + ES8311)"\n'
            '        depends on IDF_TARGET_ESP32S3\n')
    replace(cmake, 'elseif(CONFIG_BOARD_TYPE_WAVESHARE_ESP32_S3_TOUCH_LCD_1_54)\n',
            'elseif(CONFIG_BOARD_TYPE_PALPABLE_WGH_C2)\n'
            '    set(BOARD_DIR "palpable/wgh-c2")\n'
            '    set(BUILTIN_TEXT_FONT font_noto_sans_basic_20_4)\n'
            '    set(BUILTIN_ICON_FONT font_material_symbols_20_4)\n'
            '    set(DEFAULT_EMOJI_COLLECTION noto-color-emoji_64)\n'
            'elseif(CONFIG_BOARD_TYPE_WAVESHARE_ESP32_S3_TOUCH_LCD_1_54)\n')
    # The protocol has raw Opus only; upstream timestamp-based server AEC cannot be used.
    replace(main / "Kconfig.projbuild", '    bool "Enable Server-Side AEC (Unstable)"',
            '    bool "Enable Server-Side AEC (unsupported by Palpable)"\n    depends on n')
    replace(main / "protocols/protocol.h", "    uint32_t media_position_ms = 0;",
            "    uint32_t media_position_ms = 0;\n    bool voice_active = false;")
    audio = main / "audio/audio_service.cc"
    replace(audio, '#include "audio_service.h"', '#include "audio_service.h"\n#include "palpable_client.h"')
    replace(audio, "            packet->timestamp = task.timestamp;",
            "            packet->timestamp = task.timestamp;\n"
            "            packet->voice_active = palpable::Client::IsSpeech(\n"
            "                task.pcm.data(), task.pcm.size(), CONFIG_PALPABLE_VAD_THRESHOLD);")

    led = main / "led/led.h"
    replace(led, "    virtual void OnStateChanged() = 0;", "    virtual void OnStateChanged() = 0;\n"
            "    virtual bool SupportsProtocolColor() const { return false; }\n"
            "    virtual void SetProtocolColor(unsigned char r, unsigned char g, unsigned char b) {}")
    for name, available, body in [
        ("single_led", "led_strip_ != nullptr", "SetColor(r, g, b); TurnOn();"),
        ("circular_strip", "led_strip_ != nullptr", "SetAllColor({r, g, b});"),
    ]:
        replace(main / f"led/{name}.h", "    void OnStateChanged() override;",
                "    void OnStateChanged() override;\n"
                f"    bool SupportsProtocolColor() const override {{ return {available}; }}\n"
                f"    void SetProtocolColor(unsigned char r, unsigned char g, unsigned char b) override {{ {body} }}")
    wifi = main / "boards/common/wifi_board.cc"
    replace(wifi, 'config.ssid_prefix = "Xiaozhi";', 'config.ssid_prefix = "Palpable";')
    replace(wifi, "config.show_ota_config = true;", "config.show_ota_config = false;")
    replace(wifi, "config.show_sleep_config = true;", "config.show_sleep_config = false;")
    # Keep remote diagnostics from logging child speech, tool arguments or photos.
    mcp = main / "mcp_server.cc"
    replace(mcp, 'ESP_LOGE(TAG, "Failed to parse MCP message: %s", message.c_str());',
            'ESP_LOGE(TAG, "Failed to parse MCP message");')
    (DESTINATION / ".palpable-revision").write_text(LOCK["revision"] + "\n")
    print(DESTINATION)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path, help="Existing checkout at the pinned revision (offline)")
    args = parser.parse_args()
    prepare(args.source.resolve() if args.source else None)


if __name__ == "__main__":
    main()
