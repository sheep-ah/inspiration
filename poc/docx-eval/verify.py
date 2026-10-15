#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""独立校验：读取各家产物，检查替换结果、格式保留、以及 LibreOffice 可打开性。

这是与 Go 引擎完全独立的实现（Python + lxml + LibreOffice + pypdf），
避免"用被测实现验证被测实现"。
"""
import json
import os
import re
import subprocess
import sys
import zipfile

from lxml import etree
from pypdf import PdfReader

W = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
FIELD_VALUES = {
    "联系人姓名": "张三 & 李四",
    "法人代表": "李四<测试>",
    "实际经营地址": "北京市海淀区中关村大街1号院3号楼5层501室（临街商铺，靠近地铁站A口，方位说明很长的中文地址文本用于验证长文本换行与排版是否正常）",
    "联系人邮箱": "zhangsan@example.com",
    "营业执照名称": "示例科技（北京）有限公司",
    "纳税人识别号": "91110108MA01ABCD2X",
    "电话": "010-88886666",
    "开户银行": "中国工商银行股份有限公司北京中关村支行",
    "账户号": "0001234567890123456",
    "开票地址": "上海市浦东新区世纪大道100号A座20层",
}

WML_ROOTS = {"document", "hdr", "ftr", "footnotes", "endnotes", "comments"}


def wml_parts(path):
    out = []
    with zipfile.ZipFile(path) as z:
        for name in z.namelist():
            if not name.startswith("word/") or not name.endswith(".xml"):
                continue
            try:
                root = etree.fromstring(z.read(name))
            except Exception:
                continue
            tag = etree.QName(root).localname
            if tag in WML_ROOTS:
                out.append(name)
    return out


def paragraphs_text(path, part):
    """返回 [(段落文本, 段落内所有 w:t 文本片段列表)]"""
    with zipfile.ZipFile(path) as z:
        data = z.read(part)
    root = etree.fromstring(data)
    res = []
    for p in root.iter("{%s}p" % W):
        frags = []
        for t in p.iter("{%s}t" % W):
            frags.append(t.text or "")
        res.append(("".join(frags), frags))
    return res


def part_bytes(path):
    with zipfile.ZipFile(path) as z:
        return {n: z.read(n) for n in z.namelist()}


def analyze(path):
    """全面分析一个产物。"""
    info = {"parts": {}, "changed_parts": [], "values_ok": {}, "leftover": [], "unknown_left": False}
    total_expected = {}
    for name, val in FIELD_VALUES.items():
        total_expected[name] = 0
    try:
        parts = wml_parts(path)
    except Exception as e:
        return {"error": "无法打开: %s" % e}
    for part in parts:
        try:
            paras = paragraphs_text(path, part)
        except Exception as e:
            info["parts"][part] = {"error": str(e)}
            continue
        found_values = {}
        leftovers = []
        for text, frags in paras:
            for m in re.finditer(r"\{\{([^{}]*)\}\}", text):
                leftovers.append((part, m.group(0)))
            for name, val in FIELD_VALUES.items():
                c = text.count(val)
                if c:
                    found_values[name] = found_values.get(name, 0) + c
                    total_expected[name] += c
        # 未知占位符是否仍存在
        if any("不存在字段" in t for t, _ in paras):
            info["unknown_left"] = True
        info["parts"][part] = {"values": found_values, "leftovers": leftovers}
        info["leftover"].extend(leftovers)
    info["values_ok"] = total_expected
    return info


def template_expectation(tpl_path):
    """模板中每个字段应出现的次数（以 {{字段}} 出现次数计）。"""
    exp = {k: 0 for k in FIELD_VALUES}
    for part in wml_parts(tpl_path):
        for text, _ in paragraphs_text(tpl_path, part):
            for m in re.finditer(r"\{\{([^{}]*)\}\}", text):
                name = m.group(1).strip()
                if name in exp:
                    exp[name] += 1
    return exp


def part_diff(tpl_path, out_path):
    a = part_bytes(tpl_path)
    b = part_bytes(out_path)
    changed = []
    for name in a:
        if name not in b:
            changed.append("丢失:" + name)
        elif a[name] != b[name]:
            changed.append(name)
    for name in b:
        if name not in a:
            changed.append("新增:" + name)
    return changed


def lo_text(path, workdir):
    """LibreOffice 转 PDF 后用 pypdf 提文本：验证文件可被真实文字处理软件打开并渲染。"""
    os.makedirs(workdir, exist_ok=True)
    env = dict(os.environ)
    env["PATH"] = "/opt/homebrew/bin:" + env.get("PATH", "")
    try:
        subprocess.run(
            ["soffice", "--headless", "-env:UserInstallation=file:///tmp/lo-profile",
             "--convert-to", "pdf", "--outdir", workdir, path],
            check=True, capture_output=True, timeout=180, env=env)
    except Exception as e:
        return None, "LibreOffice 打开/转换失败: %s" % e
    pdf = os.path.join(workdir, os.path.splitext(os.path.basename(path))[0] + ".pdf")
    if not os.path.exists(pdf):
        return None, "未生成 PDF"
    try:
        reader = PdfReader(pdf)
        text = "\n".join((pg.extract_text() or "") for pg in reader.pages)
    except Exception as e:
        return None, "PDF 解析失败: %s" % e
    return text, None


def main():
    corpus, outdir = sys.argv[1], sys.argv[2]
    tpls = ["template_simple.docx", "template_structures.docx", "template_split_runs.docx"]
    expect = {t: template_expectation(os.path.join(corpus, t)) for t in tpls}

    results = []
    for f in sorted(os.listdir(outdir)):
        if not f.endswith(".docx"):
            continue
        if f.startswith("engine__blocked"):
            continue
        parts = f.split("__")
        if len(parts) != 2:
            continue
        adapter, tpl = parts
        path = os.path.join(outdir, f)
        tpl_path = os.path.join(corpus, tpl)
        a = analyze(path)
        a["adapter"] = adapter
        a["template"] = tpl
        a["expected"] = expect[tpl]
        a["changed_parts"] = part_diff(tpl_path, path)
        # 渲染验证
        work = os.path.join(outdir, "lo", adapter + "__" + tpl)
        text, err = lo_text(path, work)
        a["lo_error"] = err
        if text is not None:
            a["lo_has_placeholder"] = bool(re.search(r"\{\{[^{}]*\}\}", text))
            a["lo_values"] = {k: text.count(v) for k, v in FIELD_VALUES.items() if text.count(v)}
        results.append(a)

    with open(os.path.join(outdir, "verify.json"), "w") as fh:
        json.dump(results, fh, ensure_ascii=False, indent=1)

    # 打印汇总
    for a in results:
        print("=" * 100)
        print("产物: %s__%s" % (a["adapter"], a["template"]))
        if "error" in a:
            print("  ", a["error"])
            continue
        exp = {k: v for k, v in a["expected"].items() if v}
        got = {k: v for k, v in a["values_ok"].items() if v}
        print("   模板占位符应替换:", exp)
        print("   实际检索到值出现:", got)
        miss = {k: got.get(k, 0) - v for k, v in exp.items() if got.get(k, 0) != v}
        print("   数量不一致字段:", miss if miss else "无")
        print("   残留占位符:", a["leftover"] if a["leftover"] else "无")
        print("   未知占位符仍在:", a["unknown_left"])
        print("   相对模板变化的部件:", a["changed_parts"])
        print("   LibreOffice: ", a["lo_error"] or ("可打开；渲染文本中残留占位符=%s；渲染命中值=%s" % (a.get("lo_has_placeholder"), a.get("lo_values"))))


if __name__ == "__main__":
    main()
