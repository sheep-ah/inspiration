# DOCX 处理方案评估（阶段 1 交付物）

> 结论基于**真实 DOCX 文件**实测（语料与脚本见 `poc/docx-eval/`），不是文档阅读后的推断。

## 0. 结论

**采用自研 OOXML 引擎**（`archive/zip` + `encoding/xml`，字节级“外科手术式”替换），位置：`internal/docx`（阶段 2 落地）。
Go 生态中现有三个候选库在实测中全部不合格：一个没有替换 API 且会重写模板结构，一个把 `{{x}}` 解析错误并在跨片段模板上 panic，一个只能做整串文本替换（跨片段全漏）。

## 1. 评估方法

### 1.1 语料（`poc/docx-eval/corpus/`，由 `make_corpus.py` 生成）

| 文件 | 内容 | 用途 |
|---|---|---|
| `template_simple.docx` | 仅正文、占位符完整在单个 `w:t` 内 | 基线 |
| `template_structures.docx` | 正文 + 3×3 表格 + 页眉 + 页脚 + 脚注 + VML 文本框 + DrawingML 文本框 + 内容控件(SDT) + 同一段落图形前后文本，23 个占位符 | 结构与覆盖率 |
| `template_split_runs.docx` | 同 structures，但主要占位符被拆成 2–3 个 run/多个 `w:t`（含片段内格式不同的加粗片段） | Word 片段拆分 |
| `template_unknown.docx` | 含 `{{不存在字段}}` | 必须阻止生成 |
| `template_unclosed.docx` | `{{联系` 与 `人姓名}}` 分属两个段落 | 必须阻止生成 |
| `template_unsafe.docx` | 同一段落内 `{{联系` + `<w:br/>` + `人姓名}}` | 必须阻止生成 |
| `template_broken_zip.docx` | 非 zip 字节 | 损坏处理 |
| `template_broken_xml.docx` | 合法 zip，`document.xml` 被截断 | 损坏处理 |

语料本身先用 LibreOffice 打开并转换验证有效（`soffice --headless --convert-to txt`）。

### 1.2 校验（`verify.py`，与 Go 实现完全独立）

1. 用 lxml 解析每个 WML 部件，按段落拼接全部 `w:t` 文本；
2. 核对每个字段**期望出现次数 == 实际出现次数**、是否残留 `{{...}}`；
3. 对比产物与模板的每个 zip 条目字节（判断“格式保留”）；
4. 用 LibreOffice 转 PDF + pypdf 提文本（判断“能否被真实文字处理软件打开”）。

## 2. 实测结果

### 2.1 候选方案结论

| 方案 | 版本 / 许可 | 正文 | 表格 | 页眉/页脚 | 脚注 | 文本框 | 跨 run 片段 | 未知占位符校验 | 结构保真 | 结论 |
|---|---|---|---|---|---|---|---|---|---|---|
| `gomutex/godocx` | v0.1.5 / MIT | 读取 | 读取 | 读取 | 读取 | 读取 | 读取 | 无 API | **差**：open→save 重写 `document.xml`（4610→3615 字节）、`styles.xml`（349458→396415 字节）、`Content_Types`、`rels` | 不合格：无占位符替换 API，且“另存”会改写模板内容 |
| `lukasjarosch/go-docx` | v0.5.0 / MIT | — | — | 仅 header*/footer* | ✗ | ✗ | ✗ | `GetPlaceHoldersList` | 差：对 `{{x}}` 解析错误（日志 `detected nested placeholder ... skipping`），实测 **0 个占位符被替换**；跨片段模板 **panic**（`index out of range [0] with length 0`） | 不合格：功能不匹配 + 崩溃 |
| `nguyenthenguyen/docx` | v0.0.0-20230621 / MIT | ✅ | ✅ | ✅（需单独调用 `ReplaceHeader/ReplaceFooter`） | ✗ | ✗ | ✗ | 无 | 好（仅整串替换） | 不合格：跨 run 片段全部漏替（实测 structures 漏 5 类、split_runs 漏 9 类字段），脚注/文本框不处理，且无法检测未知占位符 |
| **自研引擎（采用）** | 本次 POC / 项目自有 | ✅ | ✅ | ✅ | ✅ | ✅（VML + DrawingML） | ✅ | ✅（未知/未闭合/被打断三类均阻止生成） | **优**：仅 4 个 WML 部件字节变化，其余条目字节完全一致 | **采用** |

### 2.2 自研引擎关键指标（`template_structures.docx` / `template_split_runs.docx`）

| 指标 | 结果 |
|---|---|
| 识别占位符 | 23 个 / 模板（4 个部件：`document.xml`、`header1.xml`、`footer1.xml`、`footnotes.xml`） |
| 替换正确性 | 10 个字段的期望次数与实际次数**全部一致**（如 `联系人姓名` 6 处、`电话` 5 处、`账户号` 2 处，含表格、页眉、页脚、脚注、两个文本框、SDT） |
| 跨 run 片段 | 与未拆分模板结果完全一致（23/23） |
| 残留 `{{...}}` | 0 |
| 格式保留 | 产物与模板相比，仅 4 个 WML 部件字节变化；`styles.xml`、`numbering.xml`、`theme1.xml`、`settings.xml`、`docProps/*`、`customXml/*` 等**字节完全一致** |
| 可打开性 | LibreOffice 26.8.1 headless 全部成功打开并转 PDF，渲染文本中无残留占位符 |
| 损坏文件 | 非 zip → “该文件不是有效的 DOCX（DOCX 必须是 zip 包结构）”；截断 XML → “word/document.xml: XML 解析失败: XML syntax error on line 21: unexpected EOF”；加密/旧 .doc（OLE 头 `D0 CF 11 E0`）→ 明确提示另存为未加密 .docx |
| 阻止生成 | 未知占位符 / 未闭合占位符 / 被 `w:br` 打断的占位符三类模板：`Render` 返回错误且**不产出文件**（实测 `输出文件是否生成=false`） |
| 特殊字符 | `&`、`<`、`>` 正确转义（实测值 `张三 & 李四`、`李四<测试>` 正常写入并渲染） |
| 前导零 | `0001234567890123456` 原样保留（全程字符串处理，无数字转换） |
| 多行值 | 值中的换行转换为 `<w:br/>`，保持在同一 run 内 |

原始数据：`evidence/report.json`（Go 侧）、`evidence/verify.json`、`evidence/verify_out.txt`（独立校验侧）。

## 3. 引擎设计（阶段 2 实现基线）

1. **不做全量 XML 重序列化**：按 token 流解析，记录每个 `w:t` 在部件字节流中的精确区间，只重写发生变化的区间；
2. **以段落为单位拼接文本**：同一 `<w:p>` 内所有 `w:t` 文本按文档顺序拼接后再匹配 `{{字段}}`，因此天然覆盖 Word 把占位符拆成多个 run/多个文本节点的情况；
3. **部件扫描基于命名空间**（`wordprocessingml/2006/main` 的 `document`/`hdr`/`ftr`/`footnotes`/`endnotes`/`comments` 根元素），不依赖 `w:` 前缀，覆盖正文、表格、页眉、页脚、脚注、尾注、批注、各类文本框（VML/DrawingML）与内容控件；
4. **段落栈**：文本框/图形内的嵌套段落独立处理，外层段落图形前后的文本不会被丢弃；
5. **替换落在占位符第一个片段**：值继承占位符首片段的字体格式，后续片段中的占位符字节被删除（与 Word 邮件合并行为一致）；
6. **zip 层**：未修改条目用 `CreateRaw` 按原始压缩数据原样复制；修改条目自行 deflate 后写入，避免 Go `zip.Writer` 默认的 data descriptor（bit 3），保证 OPC 读取器兼容；
7. **默认安全**：`Render` 内部先做一次与 `Inspect` 相同的校验，任何“未知/未闭合/被打断”问题都会阻断生成，防止产出缺字段的合同。

## 4. 已知限制（必须写入用户文档并在界面上提示）

| 限制 | 行为 | 计划 |
|---|---|---|
| 占位符被 `<w:br/>`、`<w:tab/>`、图形、域代码（`w:fldChar`/`w:instrText`）等元素打断 | 校验阶段阻止生成，提示“被换行/制表符等元素打断”，并指出所在部件 | 阶段 2 起：界面高亮提示 + 文档说明如何在 Word 中重排占位符 |
| 占位符跨段落（`{{` 与 `}}` 不在同一段） | 阻止生成，提示“未闭合的占位符” | 同上 |
| `w:t` 用 CDATA 包裹 | 阻止生成并提示用 Word 重新保存 | 保留检测，不做替换（Word 不会生成这种结构） |
| 修订痕迹中被删除的文本（`w:delText`） | 不参与匹配、不替换（正确行为） | 记录为设计说明 |
| 域代码（Field）内部文本 | 不替换（`w:instrText` 视为打断元素） | 文档说明 |
| 加密/密码保护的 DOCX、旧版 `.doc` | 拒绝并给出明确提示 | 上传时同步校验扩展名与 `[Content_Types].xml` 主类型 |
| 宏文件 `.docm` | 不做支持（只有 `.docx` 模板） | 上传时拒绝并提示 |
| 其他 OOXML 结构：图表数据（`charts/*.xml` 中 `a:t`）、SmartArt 文本、艺术字 | 不处理（其文本不是 `w:t`） | 若用户模板确需支持，作为独立需求评估 |
| 图片/版式 | 完全不改动字节 | — |

## 5. 复现步骤

```bash
cd poc/docx-eval
python3 -m pip install python-docx lxml pypdf
python3 make_corpus.py                          # 生成 corpus/
cd go-eval && go run . ../corpus ../out         # 跑四家实现，产出 out/report.json
cd .. && python3 verify.py corpus out           # 独立校验（含 LibreOffice 渲染验证）
```
