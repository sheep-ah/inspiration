# DOCX 引擎评估 POC（阶段 1 证据）

本目录是阶段 1「DOCX 处理方案评估」的可复现证据：

- `make_corpus.py` — 生成测试语料（python-docx + lxml 直接改写 XML，构造 Word 会产生的 run 拆分、表格、页眉页脚、脚注、VML/DrawingML 文本框、内容控件等结构）。
- `corpus/` — 生成的 8 个真实 DOCX 文件（语料本身已用 LibreOffice 打开验证有效）。
- `go-eval/` — Go 侧对照程序：自研引擎 + 三个第三方库（`godocx`、`go-docx`、`nguyenthenguyen/docx`）。
  - `go-eval/engine/engine.go` — **自研引擎原型**（阶段 2 将迁入 `internal/docx`）。
- `verify.py` — 与 Go 实现完全独立的校验：按段落拼接 `w:t` 文本核对字段次数、检查残留占位符、逐 zip 条目字节比对、LibreOffice→PDF 渲染检查。
- `out/`、`../../docs/phase1/evidence/` — 运行产物与原始结果（`report.json`、`verify.json`、`verify_out.txt`）。

## 语料说明

| 文件 | 关键结构 |
|---|---|
| `template_simple.docx` | 仅正文，占位符在单个 `w:t` 内 |
| `template_structures.docx` | 正文 + 表格 + 页眉 + 页脚 + 脚注 + VML 文本框 + DrawingML 文本框 + SDT + 图形前后文本（23 个占位符） |
| `template_split_runs.docx` | 同上，但占位符被拆成 2–3 个 run/`w:t`，其中一段带加粗格式 |
| `template_unknown.docx` | 含 `{{不存在字段}}`（必须阻止生成） |
| `template_unclosed.docx` | `{{联系` 与 `人姓名}}` 分属两个段落（必须阻止生成） |
| `template_unsafe.docx` | 同一段落内被 `<w:br/>` 打断的占位符（必须阻止生成） |
| `template_broken_zip.docx` | 非 zip 内容 |
| `template_broken_xml.docx` | 合法 zip，`document.xml` 被截断 |

## 复现

```bash
cd poc/docx-eval
python3 -m pip install python-docx lxml pypdf
python3 make_corpus.py                    # 生成 corpus/
cd go-eval
go run . ../corpus ../out                 # 四家实现对照，输出 out/report.json
cd ..
python3 verify.py corpus out              # 独立校验，输出 out/verify.json
```

渲染校验需要 LibreOffice（`brew install --cask libreoffice`），脚本会以 headless 模式转换 PDF 并用 pypdf 抽取文本。

## 结论摘要（详见 `docs/phase1/docx-evaluation.md`）

| 方案 | 结果 |
|---|---|
| `nguyenthenguyen/docx` | 只能整串替换：跨 run 片段全部漏替；脚注/文本框不处理 |
| `lukasjarosch/go-docx` | 对 `{{x}}` 解析错误（0 个占位符被替换），跨片段模板 panic |
| `gomutex/godocx` | 无占位符替换 API；“打开→另存”会重写 `document.xml`/`styles.xml` 等部件 |
| **自研引擎** | 23/23 占位符正确替换（含跨 run 片段、表格、页眉页脚、脚注、两个文本框、SDT）；未修改的 zip 条目字节完全一致；未知/未闭合/被打断三类问题阻止生成且不产出文件；LibreOffice 打开渲染通过 |
