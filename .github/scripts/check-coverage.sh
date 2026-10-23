#!/usr/bin/env bash
# 检查 Go 单测覆盖率是否达到门槛：check-coverage.sh <coverage-profile> <min-percent>
set -euo pipefail

profile=${1:?usage: check-coverage.sh <coverage-profile> <min-percent>}
min=${2:?usage: check-coverage.sh <coverage-profile> <min-percent>}

total=$(go tool cover -func="$profile" | awk '/^total:/ {sub(/%$/, "", $NF); print $NF}')
if [ -z "$total" ]; then
  echo "无法从 ${profile} 读取总覆盖率" >&2
  exit 1
fi

printf '总覆盖率 %s%%（要求 ≥ %s%%）\n' "$total" "$min"
awk -v t="$total" -v m="$min" 'BEGIN { exit !(t + 0 >= m + 0) }' || {
  echo "覆盖率未达到要求" >&2
  exit 1
}
