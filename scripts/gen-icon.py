#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""生成「灵感」应用图标（纯几何绘制，无字体依赖，可复现）。

产出：
  build/appicon.png        1024x1024，Wails 用它生成 macOS icns 与 Windows 资源
  build/windows/icon.ico   多尺寸 ICO（16/32/48/64/128/256，PNG 编码帧）

设计：蓝紫渐变圆角方块 + 白色文档纸（右上折角）+ 三条内容行 + 右下角“完成”圆勾。
用法：python3 scripts/gen-icon.py
"""
import os
import struct
import zlib

SIZE = 1024
SS = 4  # 超采样倍数（抗锯齿）

BG_TOP = (37, 99, 235)     # #2563eb
BG_BOTTOM = (96, 165, 250)  # #60a5fa
PAPER = (255, 255, 255, 255)
LINE = (203, 213, 225, 255)  # #cbd5e1
CHECK = (37, 99, 235, 255)

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


def lerp(a, b, t):
    return a + (b - a) * t


def rounded_rect(x, y, rx, ry, rw, rh, radius):
    """点 (x,y) 是否落在圆角矩形内。"""
    if x < rx or y < ry or x >= rx + rw or y >= ry + rh:
        return False
    cx = min(max(x, rx + radius), rx + rw - radius)
    cy = min(max(y, ry + radius), ry + rh - radius)
    dx = x - cx
    dy = y - cy
    return dx * dx + dy * dy <= radius * radius


def circle(x, y, cx, cy, r):
    dx = x - cx
    dy = y - cy
    return dx * dx + dy * dy <= r * r


def ring(x, y, cx, cy, r, width):
    d2 = (x - cx) ** 2 + (y - cy) ** 2
    return (r - width) ** 2 <= d2 <= r * r


def segment_distance(x, y, x1, y1, x2, y2):
    vx, vy = x2 - x1, y2 - y1
    wx, wy = x - x1, y - y1
    L2 = vx * vx + vy * vy
    t = 0.0 if L2 == 0 else max(0.0, min(1.0, (wx * vx + wy * vy) / L2))
    dx, dy = x - (x1 + t * vx), y - (y1 + t * vy)
    return (dx * dx + dy * dy) ** 0.5


def render(size):
    """返回 RGBA 像素列表（size*size）。"""
    n = size * SS
    px = [[(0, 0, 0, 0)] * n for _ in range(n)]

    # 几何参数（相对 1024 基准）
    s = n / 1024.0
    margin = 70 * s
    radius = 210 * s
    paper_x, paper_y = 300 * s, 214 * s
    paper_w, paper_h = 424 * s, 596 * s
    fold = 108 * s
    line_x0, line_x1 = paper_x + 62 * s, paper_x + paper_w - 62 * s
    line_ys = [paper_y + 300 * s, paper_y + 372 * s, paper_y + 444 * s]
    line_h = 34 * s
    check_cx, check_cy, check_r = 726 * s, 745 * s, 148 * s

    for yy in range(n):
        t = yy / float(n - 1)
        bg = (
            int(lerp(BG_TOP[0], BG_BOTTOM[0], t)),
            int(lerp(BG_TOP[1], BG_BOTTOM[1], t)),
            int(lerp(BG_TOP[2], BG_BOTTOM[2], t)),
            255,
        )
        row = px[yy]
        for xx in range(n):
            x, y = xx + 0.5, yy + 0.5
            if not rounded_rect(x, y, margin, margin, n - 2 * margin, n - 2 * margin, radius):
                continue
            color = bg
            # 文档纸
            if rounded_rect(x, y, paper_x, paper_y, paper_w, paper_h, 34 * s):
                color = PAPER
                # 右上折角（折叠后是背景色）
                fx, fy = paper_x + paper_w - fold, paper_y
                if y - fy <= fold and x - fx <= fold and (x - fx) + (fy + fold - y) <= fold:
                    color = bg
                # 内容行
                for ly in line_ys:
                    if line_x0 <= x <= line_x1 and ly <= y <= ly + line_h:
                        color = LINE
            # 右下角勾选圆
            if circle(x, y, check_cx, check_cy, check_r):
                color = PAPER
                w = 30 * s
                if (
                    segment_distance(x, y, check_cx - 62 * s, check_cy + 4 * s, check_cx - 14 * s, check_cy + 52 * s) <= w
                    or segment_distance(x, y, check_cx - 14 * s, check_cy + 52 * s, check_cx + 66 * s, check_cy - 46 * s) <= w
                ):
                    color = CHECK
            row[xx] = color

    # 下采样（box filter）
    out = []
    for y in range(size):
        for x in range(size):
            r = g = b = a = 0
            for dy in range(SS):
                for dx in range(SS):
                    cr, cg, cb, ca = px[y * SS + dy][x * SS + dx]
                    r += cr * ca
                    g += cg * ca
                    b += cb * ca
                    a += ca
            count = SS * SS
            if a == 0:
                out.append((0, 0, 0, 0))
            else:
                out.append((r // a, g // a, b // a, a // count))
    return out


def write_png(path, size, pixels):
    raw = bytearray()
    for y in range(size):
        raw.append(0)  # filter type 0
        for x in range(size):
            raw.extend(pixels[y * size + x])
    def chunk(tag, data):
        c = struct.pack(">I", len(data)) + tag + data
        return c + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)
    png = b"\x89PNG\r\n\x1a\n"
    png += chunk(b"IHDR", struct.pack(">IIBBBBB", size, size, 8, 6, 0, 0, 0))
    png += chunk(b"IDAT", zlib.compress(bytes(raw), 9))
    png += chunk(b"IEND", b"")
    with open(path, "wb") as f:
        f.write(png)
    return len(png)


def write_ico(path, frames):
    """frames: [(size, png_bytes)]，ICO 支持 PNG 编码帧（Vista+）。"""
    count = len(frames)
    header = struct.pack("<HHH", 0, 1, count)
    offset = 6 + 16 * count
    entries = b""
    payload = b""
    for size, data in frames:
        w = 0 if size >= 256 else size
        entries += struct.pack("<BBBBHHII", w, w, 0, 0, 1, 32, len(data), offset)
        payload += data
        offset += len(data)
    with open(path, "wb") as f:
        f.write(header + entries + payload)


def main():
    base = render(SIZE)
    appicon = os.path.join(ROOT, "build", "appicon.png")
    size_bytes = write_png(appicon, SIZE, base)
    print(f"生成 {appicon}（{SIZE}x{SIZE}, {size_bytes} 字节）")

    # 每个尺寸单独渲染（几何清晰，避免放大模糊）
    ico_dir = os.path.join(ROOT, "build", "windows")
    os.makedirs(ico_dir, exist_ok=True)
    frames = []
    for s in (256, 128, 64, 48, 32, 16):
        tmp = os.path.join(ico_dir, f".tmp-{s}.png")
        write_png(tmp, s, render(s))
        with open(tmp, "rb") as f:
            frames.append((s, f.read()))
        os.remove(tmp)
    ico_path = os.path.join(ico_dir, "icon.ico")
    write_ico(ico_path, frames)
    print(f"生成 {ico_path}（{len(frames)} 个尺寸：{[s for s, _ in frames]}）")


if __name__ == "__main__":
    main()
