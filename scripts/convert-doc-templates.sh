#!/usr/bin/env bash
# 批量把 .doc 模板转换成 .docx，并用灵感自带的 DOCX 引擎做「模板体检」。
#
# 用途：历史 .doc 模板导入灵感之前的一次性转换。
# 转换由本机已安装的 LibreOffice 完成（不联网、不上传任何内容）。
#
# 用法：
#   scripts/convert-doc-templates.sh <输入目录> [输出目录]
#   scripts/convert-doc-templates.sh --inspector <灵感可执行文件> <输入目录>
#
# 退出码：0 全部成功；1 有文件失败或体检发现问题；2 环境不满足（缺转换器/缺输入）。

set -uo pipefail

INSPECTOR=""
if [[ "${1:-}" == "--inspector" ]]; then
  INSPECTOR="${2:-}"
  shift 2
fi
IN_DIR="${1:-}"
OUT_DIR="${2:-}"

if [[ -z "$IN_DIR" ]]; then
  echo "用法: $0 [--inspector <灵感可执行文件>] <输入目录> [输出目录]" >&2
  exit 2
fi
if [[ ! -d "$IN_DIR" ]]; then
  echo "输入目录不存在: $IN_DIR" >&2
  exit 2
fi
if [[ -z "$OUT_DIR" ]]; then
  OUT_DIR="$IN_DIR/converted-docx"
fi
mkdir -p "$OUT_DIR"

# ---------- 定位转换器 ----------
find_soffice() {
  local c
  for c in soffice libreoffice \
           "/Applications/LibreOffice.app/Contents/MacOS/soffice" \
           "/usr/lib/libreoffice/program/soffice" \
           "/c/Program Files/LibreOffice/program/soffice.exe" \
           "/mnt/c/Program Files/LibreOffice/program/soffice.exe"; do
    if command -v "$c" >/dev/null 2>&1 || [[ -x "$c" ]]; then
      printf '%s\n' "$c"
      return 0
    fi
  done
  return 1
}
SOFFICE="$(find_soffice || true)"
if [[ -z "$SOFFICE" ]]; then
  cat >&2 <<'EOF'
未找到 LibreOffice（无法完成 .doc → .docx 转换）。

安装方式：
  macOS   brew install --cask libreoffice
  Windows  https://www.libreoffice.org/download/
  Linux   sudo apt install libreoffice-writer

替代方案：直接在 Word / WPS 中打开 .doc → 另存为 → Word 文档 (*.docx)。
EOF
  exit 2
fi

# 独立的用户配置目录，避免与正在运行的 LibreOffice 抢锁
LO_PROFILE="${TMPDIR:-/tmp}/lo-profile-inspiration-convert"
# 注意：LibreOffice 只认单横线的 -env:Name=Value；写成 --env: 会报 "Invalid URL" /
# "File URL host must be localhost or empty on darwin"。
LO_ARGS=(--headless "-env:UserInstallation=file://$LO_PROFILE" --convert-to docx --outdir "$OUT_DIR")

# ---------- 定位体检程序（可选） ----------
if [[ -z "$INSPECTOR" ]]; then
  for c in "./build/bin/灵感.app/Contents/MacOS/灵感" "./build/bin/灵感" "./build/bin/灵感.exe"; do
    if [[ -x "$c" ]]; then INSPECTOR="$c"; break; fi
  done
fi

echo "转换器   : $SOFFICE"
echo "体检程序 : ${INSPECTOR:-未找到（跳过模板体检）}"
echo "输入目录 : $IN_DIR"
echo "输出目录 : $OUT_DIR"
echo

ok=0
bad=0
failed=0
FAILED_FILES=()
PROBLEM_FILES=()

shopt -s nullglob
files=("$IN_DIR"/*.doc "$IN_DIR"/*.DOC)
if [[ ${#files[@]} -eq 0 ]]; then
  echo "输入目录下没有 .doc 文件" >&2
  exit 2
fi

field_count() {
  # 从 --inspect 的 JSON 中数出识别到的字段数；没有 python3 时返回 ?
  python3 -c 'import json,sys;d=json.load(sys.stdin);print(len(d.get("fieldUsage") or {}))' 2>/dev/null || echo "?"
}

start=$(date +%s)
for f in "${files[@]}"; do
  base="$(basename "$f")"
  name="${base%.*}"
  out="$OUT_DIR/$name.docx"
  printf '%-44s ' "$base"

  if ! "$SOFFICE" "${LO_ARGS[@]}" "$f" >/dev/null 2>&1 || [[ ! -f "$out" ]]; then
    echo "转换失败"
    FAILED_FILES+=("$base")
    failed=$((failed + 1))
    continue
  fi

  size=$(wc -c <"$out" | tr -d ' ')
  if [[ -z "$INSPECTOR" ]]; then
    echo "已转换（${size} 字节）"
    ok=$((ok + 1))
    continue
  fi

  if report="$("$INSPECTOR" --inspect "$out" 2>&1)"; then
    count="$(printf '%s' "$report" | field_count)"
    if [[ "$count" == "0" ]]; then
      # 没有识别到任何 {{占位符}}：要么源文件本来就不是合同模板，要么占位符写法不对
      echo "已转换（${size} 字节）但未识别到任何占位符，需人工确认源文件"
      PROBLEM_FILES+=("$base")
      bad=$((bad + 1))
    else
      echo "已转换（${size} 字节）｜识别字段 ${count} 个"
      ok=$((ok + 1))
    fi
  else
    echo "已转换（${size} 字节）但模板体检有问题："
    printf '%s\n' "$report" | sed 's/^/    /'
    PROBLEM_FILES+=("$base")
    bad=$((bad + 1))
  fi
done
end=$(date +%s)

echo
echo "===== 汇总（耗时 $((end - start)) 秒）====="
echo "成功       : $ok"
echo "需人工检查 : $bad ${PROBLEM_FILES[*]:-}"
echo "失败       : $failed ${FAILED_FILES[*]:-}"
echo
echo '重要：转换只是"能读进来"的第一步。请在 Word / WPS 中打开转换后的 .docx，'
echo '      确认版式、页眉页脚、表格、文本框与原件一致后，再导入灵感使用。'

if [[ $failed -eq 0 && $bad -eq 0 ]]; then
  exit 0
fi
exit 1
