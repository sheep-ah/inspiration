#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""生成 DOCX 测试语料库。

语料库用于验证占位符替换方案在真实 Word 结构下的行为：
  - 正文 / 表格 / 页眉 / 页脚 / 文本框(VML) / 脚注 / 内容控件(SDT)
  - 占位符被 Word 拆成多个 w:t 文本片段（含跨片段且片段内格式不同）
  - 同一字段重复出现
  - 未知占位符
  - 中文、特殊字符、前导零、超长文本
输出目录: corpus/
"""
import copy
import os
import shutil
import sys
import zipfile

from docx import Document
from docx.oxml.ns import qn
from lxml import etree

OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "corpus")
W = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

FIELDS = [
    ("联系人姓名", "张三"),
    ("法人代表", "李四"),
    ("实际经营地址", "北京市海淀区中关村大街1号院3号楼5层501室"),
    ("联系人邮箱", "zhangsan@example.com"),
    ("营业执照名称", "示例科技（北京）有限公司"),
    ("纳税人识别号", "91110108MA01ABCD2X"),
    ("电话", "010-88886666"),
    ("开户银行", "中国工商银行股份有限公司北京中关村支行"),
    ("账户号", "0001234567890123456"),
    ("开票地址", "北京市海淀区中关村大街1号院3号楼5层"),
]


def ensure(path):
    os.makedirs(path, exist_ok=True)


def build_base_doc():
    """基础模板：正文 + 表格 + 页眉 + 页脚 + 内容控件 + VML 文本框。"""
    doc = Document()
    doc.add_heading("灵感合同模板（测试语料）", level=1)

    p = doc.add_paragraph()
    p.add_run("甲方（联系人）：")
    p.add_run("{{联系人姓名}}")
    p.add_run("    法定代表人：")
    p.add_run("{{法人代表}}")

    p = doc.add_paragraph()
    p.add_run("实际经营地址：{{实际经营地址}}")

    p = doc.add_paragraph()
    p.add_run("营业执照名称：")
    p.add_run("{{营业执照名称}}")
    p.add_run("（纳税人识别号：{{纳税人识别号}}）")

    p = doc.add_paragraph()
    p.add_run("联系人邮箱：{{联系人邮箱}}    电话：{{电话}}")

    p = doc.add_paragraph()
    p.add_run("开户银行：{{开户银行}}    账户号：{{账户号}}    开票地址：{{开票地址}}")

    # 重复占位符：同一字段出现多次，必须全部替换
    p = doc.add_paragraph()
    p.add_run("重复字段检查：{{联系人姓名}} / {{联系人姓名}}")

    table = doc.add_table(rows=3, cols=3)
    table.style = "Table Grid"
    cells = [
        "字段", "值", "备注",
        "联系人姓名", "{{联系人姓名}}", "表格内占位符",
        "账户号", "{{账户号}}", "表格内前导零检查",
    ]
    for i, val in enumerate(cells):
        table.cell(i // 3, i % 3).text = val

    add_drawingml_textbox(doc)

    # 页眉 / 页脚
    section = doc.sections[0]
    section.header.paragraphs[0].text = "页眉：{{营业执照名称}} / {{联系人姓名}}"
    footer = section.footer.paragraphs[0]
    footer.text = "页脚：{{联系人邮箱}} 电话 {{电话}}"

    return doc


def add_drawingml_textbox(doc):
    """DrawingML(wps) 文本框，且同一段落内图形前后都有文本。"""
    xml = """
    <w:p xmlns:w="{W}" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"
         xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"
         xmlns:wps="http://schemas.microsoft.com/office/word/2010/wordprocessingShape">
      <w:r><w:t xml:space="preserve">图形前：</w:t></w:r>
      <w:r><w:drawing>
        <wp:inline distT="0" distB="0" distL="0" distR="0">
          <wp:extent cx="2743200" cy="457200"/>
          <wp:docPr id="10" name="DrawingML文本框"/>
          <a:graphic>
            <a:graphicData uri="http://schemas.microsoft.com/office/word/2010/wordprocessingShape">
              <wps:wsp>
                <wps:spPr>
                  <a:xfrm><a:off x="0" y="0"/><a:ext cx="2743200" cy="457200"/></a:xfrm>
                  <a:prstGeom prst="rect"><a:avLst/></a:prstGeom>
                </wps:spPr>
                <wps:txbx><w:txbxContent>
                  <w:p><w:r><w:t xml:space="preserve">DrawingML文本框内：{{电话}}</w:t></w:r></w:p>
                </w:txbxContent></wps:txbx>
                <wps:bodyPr/>
              </wps:wsp>
            </a:graphicData>
          </a:graphic>
        </wp:inline>
      </w:drawing></w:r>
      <w:r><w:t xml:space="preserve">图形后：{{法人代表}}</w:t></w:r>
    </w:p>
    """.replace("{W}", W)
    doc.element.body.append(etree.fromstring(xml))
    return doc


def inject_sdt_and_textbox(doc):
    """正文中插入内容控件(SDT)与 VML 文本框，其中包含占位符。"""
    body = doc.element.body
    nsmap = {
        "w": W,
        "v": "urn:schemas-microsoft-com:vml",
        "o": "urn:schemas-microsoft-com:office:office",
    }
    sdt_xml = """
    <w:p xmlns:w="{w}">
      <w:sdt>
        <w:sdtPr><w:alias w:val="正文SDT"/><w:tag w:val="sdt-1"/></w:sdtPr>
        <w:sdtContent>
          <w:r><w:t xml:space="preserve">内容控件内：</w:t></w:r>
          <w:r><w:t>{{联系人姓名}}</w:t></w:r>
        </w:sdtContent>
      </w:sdt>
    </w:p>
    """.replace("{w}", W)
    body.append(etree.fromstring(sdt_xml))

    textbox_xml = """
    <w:p xmlns:w="{w}" xmlns:v="urn:schemas-microsoft-com:vml" xmlns:o="urn:schemas-microsoft-com:office:office">
      <w:r>
        <w:pict>
          <v:shape id="TextBox1" type="#_x0000_t202" style="width:300pt;height:40pt">
            <v:textbox>
              <w:txbxContent>
                <w:p><w:r><w:t xml:space="preserve">文本框内:{{开票地址}}</w:t></w:r></w:p>
              </w:txbxContent>
            </v:textbox>
          </v:shape>
        </w:pict>
      </w:r>
    </w:p>
    """.replace("{w}", W)
    body.append(etree.fromstring(textbox_xml))
    return doc


def split_runs_in_paragraph(p, old, pieces):
    """把包含 old 的 w:t 拆成多个片段（模拟 Word 因 rsid/拼写检查/输入法产生的 run 切分）。

    pieces: [(文本, 是否加粗), ...]，拼接结果等于 old。
    """
    if hasattr(p, "_element"):  # docx.text.paragraph.Paragraph
        p = p._element
    for t in p.iter(qn("w:t")):
        if t.text and old in t.text:
            run = t.getparent()
            parent = run.getparent()
            idx = list(parent).index(run)
            run_text = t.text
            prefix, suffix = run_text.split(old, 1)
            # 原 run 只保留前缀
            t.text = prefix
            t.set(qn("xml:space"), "preserve")
            new_runs = []
            for text, bold in pieces:
                new_run = copy.deepcopy(run)
                for child in list(new_run):
                    if child.tag != qn("w:rPr"):
                        new_run.remove(child)
                if bold:
                    rpr = new_run.find(qn("w:rPr"))
                    if rpr is None:
                        rpr = etree.SubElement(new_run, qn("w:rPr"))
                    etree.SubElement(rpr, qn("w:b"))
                nt = etree.SubElement(new_run, qn("w:t"))
                nt.text = text
                nt.set(qn("xml:space"), "preserve")
                new_runs.append(new_run)
            # 后缀 run
            if suffix:
                tail_run = copy.deepcopy(run)
                for child in list(tail_run):
                    if child.tag != qn("w:rPr"):
                        tail_run.remove(child)
                tt = etree.SubElement(tail_run, qn("w:t"))
                tt.text = suffix
                tt.set(qn("xml:space"), "preserve")
                new_runs.append(tail_run)
            for offset, nr in enumerate(new_runs):
                parent.insert(idx + 1 + offset, nr)
            return True
    return False


def make_split_doc(base_builder):
    """占位符跨 run 片段版本。"""
    doc = base_builder()
    split_plan = [
        ("{{联系人姓名}}", [("{{联系", False), ("人姓", True), ("名}}", False)]),
        ("{{法人代表}}", [("{{法人", False), ("代", False), ("表}}", True)]),
        ("{{实际经营地址}}", [("{{实际", False), ("经营", True), ("地址}}", False)]),
        ("{{纳税人识别号}}", [("{{纳税", False), ("人识别号}}", False)]),
        ("{{账户号}}", [("{{账户", False), ("号}}", False)]),
        ("{{开票地址}}", [("{{开票", False), ("地", True), ("址}}", False)]),
    ]
    for p in doc.paragraphs:
        for old, pieces in split_plan:
            split_runs_in_paragraph(p, old, pieces)
    for table in doc.tables:
        for row in table.rows:
            for cell in row.cells:
                for p in cell.paragraphs:
                    for old, pieces in split_plan:
                        split_runs_in_paragraph(p, old, pieces)
    for section in doc.sections:
        for part in (section.header, section.footer):
            for p in part.paragraphs:
                for old, pieces in split_plan:
                    split_runs_in_paragraph(p, old, pieces)
    return doc


def add_unsafe_placeholder(doc):
    """占位符中间插入 <w:br/>：同一段落内文本可拼出完整占位符，但被换行元素打断。"""
    p = doc.add_paragraph()
    p.add_run("跨行占位符：{{联系")
    run = p.add_run("")
    br = etree.SubElement(run._element, qn("w:br"))
    br.tail = None
    p.add_run("人姓名}}")
    return doc


def add_unknown_placeholder(doc):
    p = doc.add_paragraph()
    p.add_run("未知字段：{{不存在字段}}")
    return doc


def add_cross_paragraph_placeholder(doc):
    """占位符被 <w:br/> 打断，跨越两个 w:p —— 无法安全替换，必须被检测并阻止生成。"""
    p1 = doc.add_paragraph()
    p1.add_run("跨段落：{{联系")
    p2 = doc.add_paragraph()
    p2.add_run("人姓名}}")
    return doc


def add_footnote_part(docx_path, footnote_text):
    """向已保存的 docx 增加脚注部件，并在正文插入脚注引用。"""
    with zipfile.ZipFile(docx_path) as z:
        names = z.namelist()
        contents = {n: z.read(n) for n in names}

    footnotes = """<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:footnotes xmlns:w="{w}">
  <w:footnote w:type="separator" w:id="-1"><w:p><w:r><w:separator/></w:r></w:p></w:footnote>
  <w:footnote w:type="continuationSeparator" w:id="0"><w:p><w:r><w:continuationSeparator/></w:r></w:p></w:footnote>
  <w:footnote w:id="1"><w:p><w:pPr><w:pStyle w:val="FootnoteText"/></w:pPr>
    <w:r><w:rPr><w:rStyle w:val="FootnoteReference"/></w:rPr><w:footnoteRef/></w:r>
    <w:r><w:t xml:space="preserve">脚注占位符：{text}</w:t></w:r></w:p></w:footnote>
</w:footnotes>
""".format(w=W, text=footnote_text)
    contents["word/footnotes.xml"] = footnotes.encode("utf-8")

    ct = contents["[Content_Types].xml"].decode("utf-8")
    if "footnotes+xml" not in ct:
        ct = ct.replace(
            "</Types>",
            '<Override PartName="/word/footnotes.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footnotes+xml"/></Types>',
        )
    contents["[Content_Types].xml"] = ct.encode("utf-8")

    rels = contents["word/_rels/document.xml.rels"].decode("utf-8")
    if "footnotes.xml" not in rels:
        rels = rels.replace(
            "</Relationships>",
            '<Relationship Id="rIdFootnotes1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footnotes" Target="footnotes.xml"/></Relationships>',
        )
    contents["word/_rels/document.xml.rels"] = rels.encode("utf-8")

    docxml = contents["word/document.xml"].decode("utf-8")
    ref = (
        '<w:r><w:rPr><w:rStyle w:val="FootnoteReference"/></w:rPr>'
        '<w:footnoteReference w:id="1"/></w:r>'
    )
    marker = "重复字段检查："
    pos = docxml.find(marker)
    if pos == -1:
        raise RuntimeError("marker not found")
    # 在含 marker 的段落结束标签前插入脚注引用
    end = docxml.find("</w:p>", pos)
    docxml = docxml[:end] + ref + docxml[end:]
    contents["word/document.xml"] = docxml.encode("utf-8")

    with zipfile.ZipFile(docx_path, "w", zipfile.ZIP_DEFLATED) as z:
        for name, data in contents.items():
            z.writestr(name, data)


def main():
    if os.path.isdir(OUT):
        shutil.rmtree(OUT)
    ensure(OUT)

    # 1) 标准模板（占位符完整，结构齐全）
    doc = build_base_doc()
    inject_sdt_and_textbox(doc)
    p_normal = os.path.join(OUT, "template_structures.docx")
    doc.save(p_normal)
    add_footnote_part(p_normal, "{{电话}}")

    # 2) 占位符跨 run 片段版本（结构同样齐全）
    doc = make_split_doc(build_base_doc)
    inject_sdt_and_textbox(doc)
    p_split = os.path.join(OUT, "template_split_runs.docx")
    doc.save(p_split)
    add_footnote_part(p_split, "{{电话}}")

    # 2b) 必须被阻止生成的三类模板
    doc = build_base_doc()
    add_unknown_placeholder(doc)
    doc.save(os.path.join(OUT, "template_unknown.docx"))

    doc = build_base_doc()
    add_cross_paragraph_placeholder(doc)
    doc.save(os.path.join(OUT, "template_unclosed.docx"))

    doc = build_base_doc()
    add_unsafe_placeholder(doc)
    doc.save(os.path.join(OUT, "template_unsafe.docx"))

    # 3) 简单模板（单 run 占位符，用于基线）
    doc = Document()
    doc.add_paragraph("合同：{{营业执照名称}}")
    doc.add_paragraph("联系人：{{联系人姓名}} 邮箱：{{联系人邮箱}}")
    p_simple = os.path.join(OUT, "template_simple.docx")
    doc.save(p_simple)

    # 4) 损坏文件：非法 zip
    with open(os.path.join(OUT, "template_broken_zip.docx"), "wb") as f:
        f.write(b"this is not a zip file")
    # 5) 损坏文件：合法 zip，document.xml 不是合法 XML
    p_badxml = os.path.join(OUT, "template_broken_xml.docx")
    shutil.copyfile(p_normal, p_badxml)
    with zipfile.ZipFile(p_normal) as z:
        contents = {n: z.read(n) for n in z.namelist()}
    real = contents["word/document.xml"]
    contents["word/document.xml"] = real[: int(len(real) * 0.85)]
    with zipfile.ZipFile(p_badxml, "w", zipfile.ZIP_DEFLATED) as z:
        for name, data in contents.items():
            z.writestr(name, data)

    for name in sorted(os.listdir(OUT)):
        print(name, os.path.getsize(os.path.join(OUT, name)))


if __name__ == "__main__":
    main()
