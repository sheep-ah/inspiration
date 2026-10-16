#!/usr/bin/env bash
#
# 把 .app 打包成 macOS 用户"双击挂载 → 拖入「应用程序」"可用的 dmg。
#
# 为什么不能直接 hdiutil create -srcfolder App.app：
#   那样产出的 dmg 根目录里只有一个 .app，没有任何安装指引。macOS 不会把 .app
#   自动安装到 /Applications，用户双击的结果只是"从只读卷里运行"，看起来像
#   "打开了但没安装"。标准做法是在卷里放一个指向 /Applications 的符号链接。
#
# 用法：make-dmg.sh <灵感.app> <输出.dmg> [卷名]
# 打包后会挂载自检（应用存在、可执行文件存在、签名可用、/Applications 别名存在），
# 任一项不满足就删掉产物并以非 0 退出——避免再次发布不带安装指引的 dmg。

set -euo pipefail

if [ "$#" -lt 2 ]; then
  echo "用法：$0 <灵感.app> <输出.dmg> [卷名]" >&2
  exit 2
fi

APP_BUNDLE="$1"
OUT_DMG="$2"
VOLNAME="${3:-灵感}"

APP_NAME="$(basename "$APP_BUNDLE")"

if [ ! -d "$APP_BUNDLE" ]; then
  echo "✘ 找不到应用包：$APP_BUNDLE" >&2
  exit 1
fi
if [ ! -f "$APP_BUNDLE/Contents/Info.plist" ]; then
  echo "✘ 不是有效的 .app 包（缺 Contents/Info.plist）：$APP_BUNDLE" >&2
  exit 1
fi

EXEC_NAME="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleExecutable' "$APP_BUNDLE/Contents/Info.plist" 2>/dev/null || echo "$(basename "$APP_BUNDLE" .app)")"

STAGING="$(mktemp -d "${TMPDIR:-/tmp}/inspiration-dmg-stage-XXXXXX")"
MOUNTPOINT="$(mktemp -d "${TMPDIR:-/tmp}/inspiration-dmg-mnt-XXXXXX")"
MOUNTED=0

cleanup() {
  if [ "$MOUNTED" = "1" ]; then
    hdiutil detach "$MOUNTPOINT" >/dev/null 2>&1 || diskutil unmount force "$MOUNTPOINT" >/dev/null 2>&1 || true
  fi
  rm -rf "$STAGING" "$MOUNTPOINT"
}
trap cleanup EXIT

fail() {
  echo "✘ $1" >&2
  rm -f "$OUT_DMG"
  exit 1
}

# 1) 准备卷内容：应用本体 + 指向 /Applications 的别名（拖拽目标）
ditto "$APP_BUNDLE" "$STAGING/$APP_NAME"
ln -s /Applications "$STAGING/Applications"

mkdir -p "$(dirname "$OUT_DMG")"

# 2) 压缩为只读 dmg
hdiutil create -volname "$VOLNAME" -srcfolder "$STAGING" -ov -format UDZO "$OUT_DMG" >/dev/null || fail "hdiutil 创建 dmg 失败"

# 3) 挂载自检：确保用户拿到的是"可拖入应用程序"的包，而不是一个孤零零的 .app
hdiutil attach -nobrowse -readonly -mountpoint "$MOUNTPOINT" "$OUT_DMG" >/dev/null || fail "无法挂载刚生成的 dmg 进行自检"
MOUNTED=1

[ -d "$MOUNTPOINT/$APP_NAME" ] || fail "dmg 里缺少 $APP_NAME"
[ -x "$MOUNTPOINT/$APP_NAME/Contents/MacOS/$EXEC_NAME" ] || fail "dmg 里的应用缺少可执行文件 Contents/MacOS/$EXEC_NAME"
[ -f "$MOUNTPOINT/$APP_NAME/Contents/Info.plist" ] || fail "dmg 里的应用缺少 Info.plist"

LINK_TARGET="$(readlink "$MOUNTPOINT/Applications" 2>/dev/null || true)"
[ "$LINK_TARGET" = "/Applications" ] || fail "dmg 里缺少指向 /Applications 的安装别名（readlink 得到：${LINK_TARGET:-空}）"

# 应用包从只读卷里仍应能通过签名校验（当前为 adhoc 自签名）
if command -v codesign >/dev/null 2>&1; then
  codesign --verify --deep --strict "$MOUNTPOINT/$APP_NAME" >/dev/null 2>&1 \
    || fail "dmg 里的 $APP_NAME 签名校验失败"
fi

hdiutil detach "$MOUNTPOINT" >/dev/null || fail "dmg 自检后无法卸载"
MOUNTED=0

SIZE="$(du -h "$OUT_DMG" | cut -f1 | tr -d ' ')"
echo "✔ dmg 已生成并通过自检：${OUT_DMG}（${SIZE}，卷名「${VOLNAME}」，含 ${APP_NAME} + Applications 别名）"
