# 阶段 3（完善与稳定性测试）交付记录

任务书 §4 阶段 3 要求：长文本与中文输入测试、表格/页眉/页脚/跨文本片段测试、模板损坏与文件丢失与磁盘写入失败等异常测试、窗口缩放与布局检查、应用重启后数据恢复测试、Logo 与平台图标集成、代码静态检查和自动化测试。

## 1. 完成情况一览

| 任务书要求 | 状态 | 证据 |
|---|---|---|
| 长文本测试 | ✅ | `TestLongTextAndMultilineValues`：约 300 字中文地址、200 字含特殊字符的公司名、多行值（换行 → `<w:br/>`）、11 位以上账户号 |
| 中文输入测试 | 部分验证 | 自动化可覆盖"值写入与还原"；**输入法实际输入需人工在桌面点一次**（见 §4 未验证项） |
| 表格测试 | ✅ | `TestNestedTableAndMultipleSectionHeaders`（嵌套表格）、真实语料含 3×3 表格、大文档 200 行表格 |
| 页眉/页脚测试 | ✅ | 多节各自不同页眉（header1/header2）、页脚、真实语料页眉页脚；均为真实部件级断言 |
| 跨文本片段测试 | ✅ | `TestManySmallFragmentsAndEmptyTextNodes`：占位符拆成 6 个片段、夹 2 个空 `<w:t/>`、其中一段加粗；另有 `template_split_runs.docx` 真实语料 |
| 模板损坏测试 | ✅ | `template_broken_zip/broken_xml` 语料 + `TestGenerateWhenTemplateCorruptedExternally`（外部改写模板文件） |
| 文件丢失测试 | ✅ | `TestGenerateFailsWhenTemplateFileMissing`（模板被删除）+ 列表/详情标记"文件缺失" |
| 磁盘写入失败测试 | ✅ | `TestGenerateToReadOnlyDirFails`（只读目录，校验错误信息且零残留）、`TestGenerateWhenOutputPathIsAFile` |
| 窗口缩放与布局检查 | ✅ | 最小窗口 960×640 下无横向溢出、表单保持两列（见 `evidence/01-min-window-960x640.png`） |
| 应用重启后数据恢复 | ✅ | `TestEndToEndWithRealDocx`（正常关闭重开）+ `TestRestartAfterUncleanShutdown`（**未 Close 直接重开**，模拟进程被杀 / WAL 未 checkpoint） |
| Logo 与平台图标集成 | ✅ | `scripts/gen-icon.py` 可复现生成 `build/appicon.png` 与多尺寸 `build/windows/icon.ico`；macOS 包内 `iconfile.icns` 已更新（见 `evidence/02-app-icon.png`） |
| 代码静态检查 | ✅ | `go vet`、`gofmt -l`、`staticcheck ./...` 全部零告警 |
| 自动化测试 | ✅ | 55 个测试用例 + 4 个基准，5 个包全绿 |

## 2. 本次新增的测试与工具

### 2.1 自包含的 Go 测试夹具（不再依赖 Python）

`internal/docx/fixtures_test.go` 用 Go 现场构造 DOCX（`buildDocxFixture`），可精确控制 XML 结构，覆盖：

| 用例 | 覆盖点 |
|---|---|
| `TestNestedTableAndMultipleSectionHeaders` | 表格内嵌表格；两节各自引用不同页眉；页脚 |
| `TestRevisionMarksAreHandledCorrectly` | `w:ins`（修订插入）中的占位符**替换**；`w:delText`（修订删除，Word 默认不显示）**不改写** |
| `TestPlaceholdersInsideHyperlinkAndContentControl` | 超链接内、内容控件（SDT）内的占位符 |
| `TestPlaceholdersInFootnotesEndnotesComments` | 脚注 / 尾注 / 批注部件（一次扫描 4 个部件） |
| `TestBarrierElementsBlockPlaceholder` | 占位符被 `w:br` / `w:tab` / `w:fldChar` 打断 → 识别为"不安全"、阻止生成、不产出文件 |
| `TestManySmallFragmentsAndEmptyTextNodes` | 跨 6 片段 + 空文本节点 + 片段内加粗 |
| `TestCdataTextIsRejected` | CDATA 包裹的文本 → 明确拒绝而不是破坏结构 |
| `TestUnsupportedAndUnclosedPlaceholdersReported` | 未知占位符与未闭合占位符分别报告并阻止生成 |

### 2.2 性能

`internal/docx/perf_test.go`：

```
大文档：2000 段 + 200 行表格 ｜ 解析 11 ms ｜ 生成 240 ms
基准（Apple M1，3 次平均）：
  BenchmarkInspectSmallTemplate-8     0.26 ms/op   （20 段 + 5 行表格）
  BenchmarkRenderSmallTemplate-8      0.85 ms/op
  BenchmarkInspectLargeTemplate-8     9.7  ms/op   （2000 段 + 200 行表格）
  BenchmarkRenderLargeTemplate-8      122  ms/op
```

结论：真实合同模板（约 150 段 + 1 表格）解析/生成在**毫秒级**，界面不会卡；测试设了 5 秒阈值防数量级退化。

### 2.3 异常路径（`internal/e2e/robustness_test.go`）

| 用例 | 断言 |
|---|---|
| 只读输出目录 | 报错含"不可写/无法"、目录内零文件 |
| 输出路径是文件 | 明确提示不是文件夹 |
| 模板被外部改写为垃圾 | 生成报错、零残留、模板详情展示问题 |
| 超长/特殊字符/emoji/全角 文件名 | 正常生成；文件名不含路径分隔符 |
| 数据库文件被损坏 | 启动即报错，不静默继续 |
| 异常退出（未 Close）后重启 | 模板与生成记录均不丢 |
| 错误格式导入（txt/pdf/docm/空/假 zip） | 全部拒绝，且不留记录与文件 |

### 2.4 启动失败可见化

GUI 应用没有控制台：启动期错误（如数据库损坏）原本只写 stderr，用户会看到"点了没反应"。现已改为弹原生对话框，并附可操作建议：

> 数据文件可能已损坏或不可写。
> 1) 确认磁盘空间充足、数据目录可写；
> 2) 如果确认数据文件损坏：把它重命名（如 inspiration.db.bak）后重新启动应用，会重建一个空数据库；
> 3) 损坏的数据库不要删除，其中可能仍能抢救出模板信息。

实现：`internal/app/alert.go`（macOS `osascript display alert`、Windows `MessageBox`），AppleScript 语法已实测有效；`--selfcheck` 等命令行模式不弹窗。

## 3. 图标

`python3 scripts/gen-icon.py`（纯几何绘制 + 4× 超采样，无字体依赖，可复现）：

- `build/appicon.png` 1024×1024 → Wails 生成 macOS `iconfile.icns`（已校验：从 .app 包内解包出的图标即新设计）
- `build/windows/icon.ico`：6 个尺寸（256/128/64/48/32/16）
- 设计：蓝色渐变圆角方块 + 白色文档（右上折角）+ 内容行 + 右下角"完成"圆勾

## 4. 仍未验证（需要真机/人工）

| # | 项 | 说明 |
|---|---|---|
| 1 | 中文输入法实际输入体验 | 需要人在桌面用输入法打字（自动化无法替代） |
| 2 | 窗口缩放的实际拖拽手感 | 已验证 960×640 布局无溢出；拖拽过程未人工观察 |
| 3 | Windows 上的图标显示与启动失败弹窗 | 本机无法验证（`icon.ico` 已生成，Windows 资源嵌入待 CI 构建确认） |
| 4 | macOS Intel 真机 | 无设备 |
| 5 | Word/WPS 转换通道 | 本机未安装，见 `docs/doc-support-evaluation.md` §9.4 |
| 6 | 真实业务模板的更多样本回归 | 目前回归了 1 份真实模板 + 自建语料；样本越多越稳 |
| 7 | 磁盘写满（不是只读） | 需要构造小容量卷；当前只验证了只读目录 |

## 5. 阶段 4 准备情况

- `make lint` 现在包含 `gofmt` + `go vet` + （若安装）`staticcheck`
- 三个平台的构建命令已在 `Makefile`（`package-windows` / `package-macos-amd64` / `package-macos-arm64` / `package-macos-universal`）
- CI 需要：Go 1.26 + Node 20+ + wails CLI v2.16.0 + NSIS（Windows 打包）；前端必须先构建（`//go:embed all:frontend/dist` 需要 dist 存在，仓库里保留了 `frontend/dist/.gitkeep` 占位以保证 Go 编译）
