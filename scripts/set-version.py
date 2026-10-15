#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""把版本号写入 wails.json（供 macOS Info.plist 与 Windows 资源/安装器使用）。

用法：
    python3 scripts/set-version.py v1.2.3      # 写入 1.2.3
    python3 scripts/set-version.py 1.2.3       # 同上

版本号必须与 GitHub Release 标签一致，保证「应用内可见版本 = Release 标签 = 产物文件名」。
"""
import json
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
WAILS_JSON = os.path.join(ROOT, "wails.json")


def normalize(version: str) -> str:
    v = version.strip()
    v = v[1:] if v.startswith("v") else v
    if not re.fullmatch(r"\d+\.\d+\.\d+", v):
        raise SystemExit(f"版本号格式不合法：{version}（应为 vX.Y.Z 或 X.Y.Z）")
    return v


def main() -> int:
    if len(sys.argv) != 2:
        raise SystemExit(__doc__)
    version = normalize(sys.argv[1])
    with open(WAILS_JSON, encoding="utf-8") as f:
        cfg = json.load(f)
    cfg.setdefault("info", {})["productVersion"] = version
    with open(WAILS_JSON, "w", encoding="utf-8") as f:
        json.dump(cfg, f, ensure_ascii=False, indent=2)
        f.write("\n")
    print(f"wails.json 版本已设置为 {version}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
