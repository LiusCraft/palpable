#!/usr/bin/env bash
# 生成 shields 风格的覆盖率徽章（SVG 输出到 stdout），入参为百分比数字，例如 75.1。
set -euo pipefail

coverage=${1:?usage: coverage-badge.sh <percent>}

# 颜色阈值与 shields.io 的默认区间一致
color=$(awk -v c="$coverage" 'BEGIN {
  if (c < 50) print "#e05d44"
  else if (c < 80) print "#dfb317"
  else print "#4c1"
}')

label=coverage
value="${coverage}%"
label_width=62
# 11px Verdana 下按每字符约 7px 估算右半部分宽度
value_width=$(awk -v n="${#value}" 'BEGIN { printf "%d", n * 7 + 10 }')
total_width=$((label_width + value_width))
value_x=$((label_width + value_width / 2))

cat <<EOF
<svg xmlns="http://www.w3.org/2000/svg" width="${total_width}" height="20" role="img" aria-label="${label}: ${value}">
  <title>${label}: ${value}</title>
  <linearGradient id="s" x2="0" y2="100%">
    <stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
    <stop offset="1" stop-opacity=".1"/>
  </linearGradient>
  <clipPath id="r"><rect width="${total_width}" height="20" rx="3" fill="#fff"/></clipPath>
  <g clip-path="url(#r)">
    <rect width="${label_width}" height="20" fill="#555"/>
    <rect x="${label_width}" width="${value_width}" height="20" fill="${color}"/>
    <rect width="${total_width}" height="20" fill="url(#s)"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" font-size="11">
    <text x="$((label_width / 2))" y="15" fill="#010101" fill-opacity=".3">${label}</text>
    <text x="$((label_width / 2))" y="14">${label}</text>
    <text x="${value_x}" y="15" fill="#010101" fill-opacity=".3">${value}</text>
    <text x="${value_x}" y="14">${value}</text>
  </g>
</svg>
EOF
