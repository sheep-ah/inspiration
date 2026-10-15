# 独立需求评估：支持 .doc（Word 97-2003 二进制）模板与输出

> 本文是任务书「后续确实需要的功能，作为独立需求评估」的落地评估。
> 触发原因：用户明确 **模板为 .doc（十几个以上）**、**生成的合同必须 .doc**，而任务书原约束是「模板第一版仅支持 DOCX」「输出仅支持 DOCX」。

## 0. 结论摘要

| 问题 | 结论 |
|---|---|
| 能否像 `.docx` 一样"顺手支持" `.doc`？ | **不能**。`.doc` 是 OLE2 复合二进制格式（实测文件头 `d0cf11e0a1b11ae1`），不是 zip+XML；占位符存在 piece table/FKP 结构里，Go 生态无可用的读写库 |
| 原生实现（Go 自研 .doc 引擎）可行吗？ | 技术可行但**不建议第一版做**：月级工作量（估 6–10 周）、必须重建所有含字符位置的 PLCF、写坏老模板的风险极高 |
| 现在最省事的路径是什么？ | **模板侧一次性批量转换**（已交付 `scripts/convert-doc-templates.sh`，实测 13 个模板 13 秒，全部通过体检）+ **导出侧调用本机转换器**（实测保真） |
| 有一个必须先回答的问题 | 下游"必须 .doc"**如何校验**？只按扩展名 / 用 Word 打开 / 还是按文件头解析——三种情况代价差别巨大（见 §4） |

## 1. 需求与约束

| 项 | 内容 |
|---|---|
| 模板格式 | 历史积累的 `.doc`，十几个以上 |
| 输出格式 | 必须 `.doc`（不是"最好 .doc"） |
| 原任务书约束 | 模板仅 DOCX、输出仅 DOCX；扩展需独立评估（本文） |
| 数据原则 | 默认本地处理、不上传合同内容与模板文件 → 转换必须在本机完成，不能用在线转换服务 |

## 2. 技术事实：为什么 .doc 不能"顺手支持"

### 2.1 文件结构

| 层 | 内容 | 对替换的影响 |
|---|---|---|
| OLE2/CFB 容器 | 多个"流"（Stream）组成，含扇区表与目录 | 写入时要重算扇区分配；容器本身不难 |
| `WordDocument` 流 | FIB（File Information Block）+ 主文本 + 各类 PLCF 索引 | 文本长度变化必须同步修正 FIB 中的 `fcMin`/`ccpText`/`ccpFtn`/`ccpHdr`/`ccpTxbx` 等计数字段 |
| `0Table`/`1Table` 流 | CLX（**piece table**）、`PlcfBteChpx`/`PlcfBtePapx`（字符/段落格式的 bin table）、样式表、书签、脚注、批注、域… | 每个 bin table / PLCF 都保存**字符位置**；插入更短或更长的值会让其后所有位置偏移 |
| 文本编码 | 每个 piece 可为 CP1252 压缩（1 字节/字符）或 UTF-16LE（2 字节/字符） | 中文片段必然是 UTF-16LE；中文占位符本身可匹配，但**长度变化必然发生** |
| 页眉/页脚/脚注/文本框 | 它们的文本不在"独立部件"里，而是拼在主文本流的不同区间（`ccpHdr`/`ccpFtn`/`ccpTxbx`），由 PLCF 定位 | 与正文同等对待，没有"只改正文"的取巧空间 |

### 2.2 为什么"替换几个字"会变成"重建整个文件"

占位符 `{{联系人姓名}}`（8 个字符）替换成"张三"（2 个字符）后，其后所有字符偏移都会变化，因此必须同步更新：

1. FIB 中的全部计数字段；
2. piece table（CLX）与每个 piece 的 PCD；
3. **所有**含字符位置的 PLCF：段落/字符格式 bin table、书签、脚注/尾注、批注、域、目录、修订（track changes）；
4. 若长度变化跨越 bin table 页边界，还要重建 FKP 页与 `PlcfBte*` 的页表。

漏改任何一处，Word 的表现是"文档已损坏，无法打开"或静默错版——这正是 Apache POI `HWPF` 长期以**读取**为主、写入能力受限（官方文档明确"write support is fairly limited"）的原因。

### 2.3 生态现状

| 语言/项目 | 能力 | 是否可用于本项目 |
|---|---|---|
| Go | **没有**可用的 .doc 读写库 | — |
| Java `poi-hwpf` | 读为主，写入能力有限 | 需要引入 JVM（比 LibreOffice 更重的运行时） |
| LibreOffice WW8 过滤器（C++/MPL） | 成熟，双向读写 | 可作为外部转换器调用（见 §3.2） |
| antiword / wvWare（GPL） | 只读 | 不适合写入 |
| Python | 无可靠 .doc 写库（python-docx 不支持 .doc） | — |

## 3. 三条路线与实测数据

### 3.1 路线 A：模板侧一次性转换（今天就能用）

已交付工具：`scripts/convert-doc-templates.sh`（调用本机 LibreOffice，本地转换、不联网）

```bash
# 批量把 .doc → .docx，并用灵感自带引擎做模板体检
./scripts/convert-doc-templates.sh --inspector ./build/bin/灵感.app/Contents/MacOS/灵感 ~/合同模板
```

实测结果（13 个 .doc，含 1 个损坏文件与 2 个"有问题"模板）：

| 指标 | 结果 |
|---|---|
| 转换耗时 | **13 秒 / 13 个文件**（约 1 秒/个，冷启动已包含） |
| 转换成功率 | 13/13 |
| 模板体检 | 10 个模板识别到全部 10 个字段；含未知占位符/未闭合占位符的文件被标为"需人工检查"；非模板文件因识别到 0 个占位符也被标出 |
| 渲染保真（.docx → .doc → .docx 往返） | 渲染文本**逐字一致**（唯一差异：标题行多 3 个空格） |
| 关键值 | 前导零 `0001234567890123456`、特殊字符 `张三 & 李四`、`李四<测试>` 均保留 |
| 已知结构差异 | ① 内容控件（w:sdt）外壳丢失，内部文字保留；② 文本框结构被重排（同一占位符可能同时出现在 Choice/Fallback 两个分支，计数 23→25，替换结果不受影响） |

成本：一次性（十几分钟）；风险：低（转换后人工在 Word 抽查一次即可）。

### 3.2 路线 B：应用内自动转换（模板 + 导出）

需要一个**可脚本化**的本机转换器：

| 转换器 | 可脚本化 | 实测/说明 |
|---|---|---|
| LibreOffice headless | ✅ 跨平台 | 本机实测：`.docx → .doc` **1.27 秒/文件**，输出为真 CFB 文档（`file` 识别为 "Composite Document File V2 / MS Word 97"），往返渲染文本一致。缺点：用户机器需安装（macOS cask ≈700MB），且应用调用时必须使用**独立 user profile**（否则与用户正在使用的 LibreOffice 抢锁 —— 本次实测踩到过） |
| 微软 Word COM 自动化 | ✅ 仅 Windows | 办公机器通常已装 Word，零新增安装；缺点：仅 Windows、需处理 COM 权限/弹窗/进程残留 |
| WPS | ❌ macOS / ✅ Windows(COM) | 本机 WPS 6.7.1 实测：`wpsoffice.app` 只有 GUI 二进制与内部 `libwordconvert.dylib`，**无 CLI 入口** → macOS 不可脚本化 |

工程要点（实测得出）：
- 必须传 `-env:UserInstallation=file://<专属目录>`；写成 `--env:` 会被 LibreOffice 判为非法 URL（本次踩坑：`Invalid URL` / `File URL host must be "localhost" or empty on darwin`）
- 转换有冷启动（首次 ~3–4 秒），需要超时与取消
- 转换后应**回读自检**（把 .doc 再转回 .docx 对比文本），失败则降级为"已生成 .docx，请手动另存为 .doc"
- 转换在本地进程内完成，符合"不上传"原则

成本：2–3 天（含降级路径与验收）；风险：中（外部依赖 + 转换器缺失时的体验）。

### 3.3 路线 C：原生 Go 实现 .doc 读写（不推荐第一版）

- 需要自研：CFB 容器读写 → FIB 解析 → piece table 读写 → CHPX/PAPX 与 bin table（FKP 页）重建 → 全部 PLCF 位置修正 → 文本编码（CP1252/UTF-16LE）处理
- 工作量估算：**6–10 周**（含自建真实 .doc 回归语料与损坏检测），且必须做"打开后 Word 不报错"的大量人工验证
- 致命风险：写坏客户的合同模板 = 业务事故；一旦损坏不可逆
- 结论：如果长期要求"零外部依赖 + 原生 .doc"，应当**作为独立项目立项**，而不是塞进第一阶段

### 3.4 路线对比总表

| 维度 | A 一次性转换（模板侧） | B 应用内自动转换（模板+导出） | C 原生 Go 引擎 |
|---|---|---|---|
| 解决模板 .doc | ✅ | ✅ | ✅ |
| 解决输出 .doc | ❌ | ✅ | ✅（同样要对输出侧重建 PLCF） |
| 新增外部依赖 | LibreOffice（仅转换时、可人工替代） | 必须有一个本机转换器 | 无 |
| 工作量 | 已交付（0.5 天含验收） | 2–3 天 | 6–10 周 |
| 风险 | 低 | 中（依赖缺失/超时/版式差异） | 高（文件损坏） |
| 用户侧要求 | 转换时本机有 LibreOffice 或人工另存 | 每次生成都需要转换器可用 | 无 |
| 版式保真 | 实测一致（文本层面），结构有已知差异 | 同 A 的转换引擎 | 取决于实现质量，最难保证 |

## 4. 关键前置问题：下游"必须 .doc"是如何校验的？

这一条决定方案上限，**必须先确认**：

| 情况 | 含义 | 可行方案 |
|---|---|---|
| ① 只按扩展名/人工用 Word 打开 | Word 会按文件头嗅探格式，扩展名错误也能打开 | 代价最低：甚至可考虑"输出 .docx 后改名为 .doc"（**仅在实测确认下游能正常处理后**才可采用，默认不做） |
| ② 用 POI HWPF 等库按文件头解析 | 必须是真 OLE2 .doc | 必须走路线 B 的导出转换 |
| ③ 老版本 Office（2003/2007）打开 | 需真 .doc 且低版本兼容 | 路线 B + 用旧版 Office 实测一次 |

建议：给下游做一次 5 分钟实测——把一份 `.docx` 改名成 `.doc` 丢过去，看它是否报错。这一步能直接砍掉大量不必要的复杂度。

**已交付验证包（`~/Downloads/灵感-doc格式测试/`，含说明）**，三个文件本质不同、仅扩展名相同：

| 文件 | 文件头 | 本质 |
|---|---|---|
| `A-真DOC(Word97-2003二进制).doc` | `d0cf11e0a1b11ae1` | 真 OLE2 Word 97-2003（LibreOffice 转换） |
| `B-DOCX改名成的doc.doc` | `504b030414000000` | 其实是 .docx（zip/OOXML），仅改扩展名 |
| `C-RTF改名成的doc.doc` | `7b5c727466315c61` | 其实是 RTF 文本，仅改扩展名 |

三者内容一致，LibreOffice 均能打开并读出相同文本（681 字节）。判定标准与风险：
- 只有 A 通过 → 必须真 .doc，走 Word/WPS 自动化（用户环境基本都有 Office/WPS）或 LibreOffice；
- A、B 都通过 → 下游只按扩展名或用 Word 打开，可直接「输出 .docx + 改扩展名副本」，**完全不需要转换器**；但若下游是 Office 2003 则不可用（Office 2003 打不开 .docx）；
- 按文件头解析的自研系统 → RTF 改名（C）会被拒，只能走真 .doc。

## 5. 推荐方案与实施计划

**推荐 D = A + B**（先 A 解决存量模板，再 B 解决输出格式），前提是 §4 已确认确实需要真 `.doc` 二进制。

| 阶段 | 内容 | 交付物 | 预估 |
|---|---|---|---|
| D1 | 存量 .doc 模板转换与验收 | 已交付脚本 + 转换后模板 + 人工抽查记录 | 0.5 天（脚本已完成，实测 13 秒/13 个） |
| D2 | `internal/converter`：本机转换器探测（LibreOffice/Word COM）、`.docx → .doc`、独立 profile、超时、回读自检、失败降级 | 代码 + 单测 + 报告 | 2 天 |
| D3 | 界面与导出选项：输出格式选择（DOCX / DOC·需转换器）、无转换器时的明确提示与安装指引 | 界面 + 用户说明 | 1 天 |
| D4 | 验收：真实 .doc 模板 + 真实合同在 Word/WPS 中打开验证 | 验收报告 | 0.5 天 |
| D5（可选，独立立项） | 原生 .doc 引擎评估/实现 | 独立评估报告 | 6–10 周 |

**验收标准（D2–D4）**

1. 15 个真实 .doc 模板转换后在 Word 中版式与原件一致（人工确认，逐个签字）；
2. 生成的 `.doc` 能被 Word / WPS 打开，无"文档已损坏"提示，内容与 DOCX 版本一致（逐字段核对）；
3. 无转换器 / 转换超时 / 转换失败时：应用给出明确中文提示，**不产出半成品文件**，且已填写的表单数据不丢失；
4. 转换过程不产生任何网络请求（可用抓包/审计确认）。

## 6. 未验证项（本评估结束时）

| # | 项 | 说明 |
|---|---|---|
| 1 | 用户手上那十几个**真实 .doc 模板**的转换保真度 | 本次只用自建语料实测；需要真实样本（尤其含艺术字/域/修订/嵌入对象的模板风险最高） |
| 2 | 目标 Word / WPS 版本打开生成的 `.doc` | 本机无 Word；LibreOffice + WPS 未做人工核对 |
| 3 | Windows 上 Word COM / WPS COM 自动化 | 本机为 macOS |
| 4 | 下游系统对 .doc 的实际校验方式（§4） | 需要用户确认或做一次改名实测 |
| 5 | .doc 转换在含修订痕迹/批注/嵌入对象模板上的行为 | 未构造语料 |

## 7. 本次实测命令与结果（可复现）

```bash
# 1) 判断文件是不是真 .doc（OLE2/CFB）
file template.doc          # → Composite Document File V2 Document ...
xxd -l 8 template.doc      # → d0cf11e0 a1b11ae1

# 2) .doc → .docx（单文件；注意必须用单横线 -env:）
soffice --headless -env:UserInstallation=file:///tmp/lo-profile \
        --convert-to docx --outdir out template.doc

# 3) .docx → .doc（导出方向，MS Word 97 过滤器）
soffice --headless -env:UserInstallation=file:///tmp/lo-profile \
        --convert-to doc --outdir out contract.docx

# 4) 批量转换 + 模板体检（本仓库脚本）
./scripts/convert-doc-templates.sh --inspector ./build/bin/灵感.app/Contents/MacOS/灵感 <目录>

# 5) 模板体检（应用自带，不需要数据目录）
./build/bin/灵感.app/Contents/MacOS/灵感 --inspect template.docx   # 输出 JSON；有问题时退出码非 0
```

| 实测项 | 结果 |
|---|---|
| `.doc` 文件头 | `d0cf11e0a1b11ae1`（OLE2/CFB，非 zip） |
| 应用当前对 `.doc` 的处理 | 明确报错「只支持 .docx 格式的模板文件」（不是静默失败） |
| `.docx → .doc → .docx` 往返渲染文本 | 逐字一致（仅标题缩进差 3 空格） |
| `.docx → .doc` 单文件耗时 | 1.27 秒，输出 24 KB 真 CFB 文档 |
| 批量 13 个 `.doc → .docx` | 13 秒，13/13 成功，全部通过模板体检 |
| 含未知/未闭合占位符、非模板文件 | 脚本标为"需人工检查"，退出码 1 |
| WPS（macOS 6.7.1）CLI | 无转换入口，不可脚本化 |

## 8. 实施记录：导出侧 .doc 生成（已完成）

用户门禁确认「下游必须真 .doc」后，按路线 B 的导出侧实现完成。

### 8.1 交付内容

| 组件 | 说明 |
|---|---|
| `internal/converter` | 外部转换器封装：探测（macOS/Windows/Linux 常见路径 + `INSPIRATION_SOFFICE` 环境变量）、`.docx ↔ .doc`、90 秒超时、专属 LibreOffice 用户配置目录、**转换后自检** |
| `contract.OutputFormat` | 生成请求新增输出格式（`docx` 默认 / `doc`）；`GenerateResult` 增加 `format` 与 `warning` |
| 降级策略 | 无转换器 → 明确拒绝且不产出文件；转换失败/超时/自检不通过 → **回退为生成 .docx 并在结果里带 Warning**，用户不会丢数据 |
| `app.ConverterStatus` | Wails 绑定：界面显示转换器是否就绪 |
| 界面 | 「保存位置」卡片增加输出格式选择（DOCX / DOC·需 LibreOffice）；无转换器时 .doc 选项禁用并给出原因 |
| `docx.ExtractText` | 抽取文档全文（含页眉/页脚/脚注/文本框），用于转换自检比对 |
| `灵感 --inspect <file>` | 模板体检命令（JSON 输出，有问题退出码非 0），供批量转换与 QA 使用 |
| `scripts/convert-doc-templates.sh` | 批量 .doc → .docx 转换 + 模板体检（见 §3.1） |

### 8.2 自检机制（避免把损坏文件交给用户）

1. 产物必须是 OLE2/CFB（文件头 `d0cf11e0a1b11ae1`），否则删除产物并报错；
2. 把产物 `.doc` 回读成 `.docx`，与原始 `.docx` 做**文本行集合**比对：
   - 只报告「原文有、回读后没有」的行（内容丢失 = 失败）；
   - 忽略重复行与空白差异（转换器会把文本框输出成 Choice/Fallback 两个分支，属已知无害差异）。

### 8.3 实测结果

| 项 | 结果 |
|---|---|
| 生成 `.doc` | 成功；24 KB；`file` 识别为 `Composite Document File V2 Document ... MS Word 97`；文件头 `d0cf11e0a1b11ae1` |
| 内容核对 | 10 个字段值全部落在 `.doc` 中（含页眉/页脚/脚注/表格/两个文本框）；无残留 `{{}}`；前导零 `0001234567890123456` 保留 |
| 中间产物 | 生成 `.doc` 后不残留 `.docx` 与临时目录（测试断言） |
| 无转换器 | 明确报错「当前电脑没有可用的 LibreOffice，无法生成 .doc 文件…」，且不产出任何文件（测试断言） |
| 自动化测试 | `go test ./...` 全绿；新增 `internal/converter` 5 个用例 + `internal/e2e` 3 个 `.doc` 相关用例（无 LibreOffice 的机器自动跳过实转用例） |

### 8.4 期间发现并修复的缺陷（真实运行才暴露）

| 缺陷 | 现象 | 根因与修复 |
|---|---|---|
| `fileURL` 未做百分号编码 | 在应用数据目录（`~/Library/Application Support/灵感/`，含空格与中文）下转换时 LibreOffice 崩溃：`libc++abi: terminating ... com::sun::star::uno::RuntimeException`；代码按设计降级为 .docx 并给出警告 | 直接拼接 `file://` + 路径导致空格/非 ASCII 未转义 → 改用 `net/url` 生成 `file:///Users/x/Library/Application%20Support/%E7%81%B5%E6%84%9F/...`；单测覆盖该路径形态 |
| LibreOffice 只认单横线参数 | 写成 `--env:UserInstallation=...` 时报 `Invalid URL` / `File URL host must be "localhost" or empty on darwin` | 改用 `-env:UserInstallation=...`；脚本与 Go 代码均已修正并加注释说明 |

界面效果（输出格式选择 + .doc 生成成功）：

![输出格式选择](phase2/evidence/screenshots/05-doc-output-format.png)

### 8.5 用户侧验证步骤（需要 Word / WPS）

样本已生成在应用数据目录（打开应用「关于」页可直接点击打开）：

```
~/Library/Application Support/灵感/contracts/
├── 测试合同-doc.doc     真 Word 97-2003 二进制，用 Word/WPS 打开核对
└── 测试合同-docx.docx   同一份内容，用于对照
```

重点核对：版式与 `.docx` 一致、无「文档已损坏」提示、页眉/页脚/脚注/表格/文本框中的值正确、账户号前导零保留。

### 8.6 仍未验证

| # | 项 |
|---|---|
| 1 | Word / WPS 打开生成的 `.doc`（本机无 Word，需人工确认） |
| 2 | 旧版 Office（2003/2007）兼容性 |
| 3 | Windows 上的转换器路径与 Word/WPS COM 备选方案 |
| 4 | 十几个真实 .doc 模板的批量转换保真度（需样本） |
| 5 | 含修订痕迹/批注/嵌入对象的复杂 .doc 模板 |

## 9. 最强兼容方案（已实现）

### 9.1 目标
不依赖用户判断下游怎么校验，也不假设用户机器上装了什么——**尽可能产出「所有校验方式都能通过」的产物**，无法保证时明确告知而不是悄悄降级。

### 9.2 四层保障

| 层 | 做法 | 解决什么 |
|---|---|---|
| ① 多通道转换（按保真度排序） | **Microsoft Word**（macOS AppleScript / Windows COM）→ **WPS Office**（Windows COM：KWPS.Application / WPS.Application / 被占用的 Word.Application）→ **LibreOffice**（三平台 headless） | 有 Office/WPS 的机器用原厂引擎，几乎不走样；没有则退回 LibreOffice |
| ② 产物强校验 | 必须是真 OLE2/CFB（文件头 `d0cf11e0a1b11ae1`）**且**回读文本比对无丢失，否则删除产物并报错 | 下游无论按扩展名、按文件头还是用 Office 打开都能通过；不会交付「假 .doc」或损坏文件 |
| ③ 双份输出（默认开启） | 生成 `.doc` 的同时保留一份同名 `.docx`（界面可关闭） | 即便 `.doc` 版式有差异，用户手里永远有一份保真文件，可用于打印/存档/核对 |
| ④ 无通道时明确拒绝 | 不产出任何文件，提示安装 Word / WPS / LibreOffice，并给出 `.docx` 备选 | 不会出现「以为生成了 .doc 其实没有」 |

安全性：调用 Word/WPS 时设置 `AutomationSecurity=3`（**禁止宏**）、`ConfirmConversions=false`、只读打开、转换后退出；全程本机进程，不联网、不上传。

### 9.3 自查命令与实测输出

```bash
灵感 --check-converters                     # 查看有哪些转换通道可用（不需要数据目录）
灵感 --selfcheck --run-converter-check      # 真的跑一次 .docx → .doc 验证链路与自检
```

本机实测（只装了 LibreOffice）：

```
转换通道（按优先级）：
  ✗ Microsoft Word   未安装 Microsoft Word
  ✗ WPS Office       当前平台不支持 WPS 自动化
  ✔ LibreOffice      LibreOffice 已就绪（文本框/图文框位置可能与原稿不同）

将使用：LibreOffice 已就绪（文本框/图文框位置可能与原稿不同）
```

```
自检通过：version=... dataDir=...
  转换通道：LibreOffice 已就绪（文本框/图文框位置可能与原稿不同）
转换验证通过：converter-check.doc（文件头 d0 cf 11 e0 a1 b1 1a e1）
```

界面验证（DOM 断言，桩数据）：`字段数=11`；格式提示为「将使用 LibreOffice 转换（文本框/图文框位置可能与原稿不同，建议保留 .docx 备份）」；备份复选框默认勾选；生成结果为「生成成功（DOC · LibreOffice）」并展示 `.docx` 备份位置与「打开备份」按钮。

### 9.4 验证情况

| 项 | 结果 |
|---|---|
| 通道优先级与探测 | ✅ 单测：无 Word/WPS 时退回 LibreOffice；强制指定通道时按指定判断；顺序 word → wps → libreoffice |
| LibreOffice 通道端到端 | ✅ e2e：真 `.doc`（OLE2）+ 备份 `.docx`（逐字段核对内容）+ 临时文件清理 |
| 关闭备份 | ✅ e2e：输出目录只留 `.doc` |
| 无可用通道 | ✅ e2e：明确报错、零残留 |
| 自检失败丢弃产物 | ✅ 单测：伪转换器产出非 OLE2 时删除产物并报错 |
| **Word 通道（macOS AppleScript）** | ⚠️ **未验证**：本机未安装 Microsoft Word，代码已实现但未经真实 Word 执行 |
| **Word/WPS 通道（Windows COM）** | ⚠️ **未验证**：本机为 macOS；PowerShell 脚本与 ProgID 探测已实现，需在 Windows 上执行一次 `--check-converters --run-converter-check` 确认 |

### 9.5 字段变更

按用户要求新增第 11 个字段 **`联系人电话`**（占位符 `{{联系人电话}}`，联系人手机号），与原有 `{{电话}}`（固定电话）并存：
- 模板里用哪个就填哪个；两个都出现时分别替换；
- 应用对全部字段做必填校验（当前共 11 项）；
- 已同步更新表单、校验、测试与文档。

## 10. 实施记录：模板侧 .doc 自动导入（已完成，解除「只有 .doc」的阻塞）

用户只有 `.doc` 模板、无法上传，因此把「路线 B 的模板侧」也做了：**上传 `.doc` 时应用自动转换成 `.docx` 再导入**。

### 9.1 行为

| 场景 | 行为 |
|---|---|
| 选择 `.docx` | 直接导入（原有流程） |
| 选择 `.doc` | 用本机 LibreOffice 自动转成 `.docx` → 扫描占位符 → 入库；**原始 `.doc` 一并保留**在数据目录 `templates/` 下备查 |
| 模板来源标记 | 数据库新增 `source_format`（docx/doc）与 `original_file`；列表显示「来自 .doc」徽标 |
| 导入后提示 | 明确告知：文字与占位符已保留，但**版式（尤其文本框/图文框位置）可能与原文件不同**，请先在 Word/WPS 中核对再使用 |
| 未识别到占位符 | 导入仍成功，但给出醒目提示：模板中没有 `{{字段名}}` 占位符，无法用于生成合同，并说明正确写法 |
| 本机没有 LibreOffice | 明确报错：「导入 .doc 模板需要本机安装 LibreOffice（用于自动转换为 .docx）；也可以先在 Word / WPS 中另存为 .docx 再上传」，且**不留下任何记录或文件** |
| 删除模板 | 同时清理转换后的 `.docx` 与保留的原始 `.doc` |
| 老版本数据库 | 启动时自动 `ALTER TABLE` 补齐新列（`ensureColumns`），已有模板数据不受影响；`schema_version` 升到 2 |

### 9.2 验证

| 项 | 结果 |
|---|---|
| 自动化测试 | `internal/e2e`：`TestImportDocTemplate`（真实 `.doc` → 导入 → 保留原始文件 → 用该模板生成合同 → 删除时清理原始文件）、`TestImportDocWithoutConverter`（无转换器时报错且无残留）；`internal/store`：迁移用例覆盖新旧库 |
| 真实冒烟 | 用应用代码路径导入一份真 `.doc`：`来源格式=doc`、`占位符字段=10 个`、`问题=[]`，数据目录同时出现 `<uuid>.docx` 与原始 `<uuid>.doc` |
| 回归 | `go test ./...` 5 个包全绿；`go vet`、`gofmt` 干净 |

### 9.3 使用要点（给用户）

1. 模板管理 → 上传模板 → 文件类型可选 `Word 模板 (*.docx;*.doc)`，直接选 `.doc` 即可；
2. 导入后会弹出详情：若显示「未识别到任何 `{{字段名}}` 占位符」，说明老模板里还没有占位符——请打开数据目录里的转换结果，在需要填值的位置插入 `{{联系人姓名}}` 这类占位符（字段名必须与应用的十个字段一致），保存后重新上传；
3. 若详情提示"由 .doc 自动转换而来"，请先用 Word/WPS 打开转换结果核对版式；不满意的话就用 Word 另存为 `.docx` 后上传（同源引擎保真度最高）。
