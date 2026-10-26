#!/bin/sh
set -eu
firmware_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
if ! command -v idf.py >/dev/null 2>&1; then
    echo "请先加载 ESP-IDF >= 6.0.1 的 export.sh，再执行此脚本。" >&2
    exit 1
fi
python3 "$firmware_dir/scripts/prepare.py"
cd "$firmware_dir/.build/xiaozhi"
if [ "$#" -eq 0 ]; then
    set -- build
fi
exec idf.py "$@"
