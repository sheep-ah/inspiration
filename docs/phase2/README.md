# 阶段 2（最小可行版本）交付记录

## 1. 本次交付内容

| 任务书要求（§3.1 合同生成 / §3.2 模板管理 / §3.3 占位符替换） | 实现位置 | 状态 |
|---|---|---|
| 模板下拉选择 + 模糊搜索 | `frontend/src/views/generate.ts`（组合框 + 关键字过滤）、`internal/store.fuzzyMatch`（顺序跳字匹配） | 已实现 |
| 十个必填基础字段 | `internal/contract/contract.go`（`Fields` 定义 10 个字段，含标签/提示/长度上限） | 已实现 |
| 必填校验 + 邮箱格式校验 | `contract.Values.Validate()`（空白视为未填写、邮箱正则、税号长度、超长提示） | 已实现 |
| 生成按钮 + 生成过程状态反馈 | 前端 busy 状态 + `generate:start/done/fail` 事件（`internal/app`） | 已实现 |
| 生成成功后打开文件 / 打开目录 | `app.OpenFile` / `app.RevealInFolder`（macOS `open`、Windows `cmd start`/`explorer /select`、Linux `xdg-open`） | 已实现 |
| 可理解的失败提示 | Go 侧统一中文错误；前端 `alertBox` + 字段级红色提示 | 已实现 |
| 电话/账户号/纳税人识别号按文本处理 | 全程 `string`，无数字转换；`contract` 测试断言前导零保留 | 已实现 |
| 不覆盖原始模板 | 模板副本存于数据目录 `templates/<uuid>.docx`，输出写入用户目录；`samePath` 二次防御 | 已实现 |
| 重名不静默覆盖 | `store.UniquePath` 追加 `(2)`、`(3)`… | 已实现 |
| 上传 DOCX 模板（复制到持久化目录） | `template.Service.Import`：扩展名/大小/zip 校验 → 原子复制 → 占位符扫描 → 入库（失败回滚删除文件） | 已实现 |
| 录入/修改模板名称 | `Template.Rename` + 前端重命名对话框 | 已实现 |
| 模糊搜索模板 | 列表搜索框 → `store.ListTemplates(query)` | 已实现 |
| 查看模板详情与占位符 | `template.Detail`（占位符/次数/所在部件、模板问题、未使用字段） | 已实现 |
| 删除模板 + 二次确认 | 前端 `confirmDialog`；后端软删除，文件删除失败不阻断 | 已实现 |
| 检查文件可读取 / 占位符完整可识别 | `internal/docx.Inspect`（结构性校验） | 已实现 |
| 模板文件缺失或损坏时明确报错 | 列表标记「文件缺失」、详情显示问题原因、生成时拒绝并提示 | 已实现 |
| `{{字段名}}` 占位符，同字段多次出现全部替换 | `internal/docx`（段落级拼接 + 全量替换） | 已实现 |
| 未知占位符生成前提示并阻止生成 | `Inspection.Problems()` 阻断 `Render`，**不产出文件** | 已实现 |
| Word 拆分占位符跨文本节点 | 段落内 `w:t` 拼接后再匹配（跨 run 专项语料） | 已实现 |
| 只输出 DOCX，不引入 PDF 依赖 | 无 PDF 相关依赖 | 已实现 |
| 本地数据存储（SQLite + 应用专属数据目录） | `internal/store`（`modernc.org/sqlite` 纯 Go）+ `internal/config` | 已实现 |
| 模板上传失败清理/回滚 | `Import` 失败删除已落盘文件；测试断言无残留记录与文件 | 已实现 |
| 删除模板不影响已生成的合同 | 生成记录用 `template_name` 快照 + 可空 `template_id`；e2e 断言 | 已实现 |
| 日志不输出敏感字段 | 日志只记录模板 id/名称、文件名、目录、错误；生成记录只存字段名 | 已实现 |

### 代码结构（新增）

```
main.go                        Wails 入口 + --version/--selfcheck/--data-dir
internal/version               版本信息（ldflags 注入）
internal/config                数据目录解析与创建
internal/logging               本地日志（5MB × 3 轮转）
internal/id                    UUID v4（crypto/rand，无第三方依赖）
internal/docx                  DOCX 引擎 + 8 个真实语料 testdata + 10 个测试
internal/store                 SQLite（meta/templates/contracts/settings）+ 文件存储
internal/template              模板导入/列表/详情/重命名/删除
internal/contract              字段模型、校验、生成流程
internal/app                   Wails 绑定层（16 个方法）+ 事件
internal/e2e                   端到端验收测试（真实 DOCX）
frontend/                      vanilla-ts + Vite：合同生成 / 模板管理 / 关于
Makefile                        dev/build/test/lint/package 入口
build/darwin/Info.plist        bundle id = com.serviceshare.inspiration
```

## 2. 验证记录（可复现）

### 2.1 自动化测试

```bash
go test ./...
# ok  github.com/sheep-ah/inspiration/internal/contract
# ok  github.com/sheep-ah/inspiration/internal/docx
# ok  github.com/sheep-ah/inspiration/internal/e2e
# ok  github.com/sheep-ah/inspiration/internal/store
go vet ./... && gofmt -l .    # 均通过
```

| 测试 | 覆盖内容 |
|---|---|
| `internal/docx/engine_test.go` | 23 个占位符跨 4 个部件解析；跨 run 模板与未拆分模板结果一致；所有字段替换次数与人工清点一致；表格/页眉/页脚/脚注/两个文本框/SDT；未修改 zip 条目不变量；未知/未闭合/被打断三类问题阻止生成且不产出文件；损坏 zip/截断 XML/缺失文件；缺少字段值时拒绝生成；特殊字符与前导零 |
| `internal/contract/contract_test.go` | 十个字段名与顺序；逐字段必填；邮箱格式（7 个非法 + 3 个合法样例）；超长值；税号过短；前导零与 trim；默认文件名规则 |
| `internal/store/store_test.go` | 模糊搜索；文件名清洗（路径分隔符/控制字符/保留名/长度）；重名不覆盖（3 次）；模板生命周期与软删除；生成记录只保存字段名；设置读写；数据库版本高于应用时拒绝启动 |
| `internal/e2e/e2e_test.go` | **真实 DOCX 全流程**：导入 → 校验通过 → 生成 → 产物无残留占位符 → 页眉/页脚/脚注已替换 → 样式部件字节不变 → 源模板未被修改 → 重名不覆盖 → 校验失败不产出文件 → 生成记录不含字段值 → **关闭数据库后重开数据仍在** → 删除模板不影响已生成合同；模板文件被删除时生成报错；非 docx/损坏文件被拒绝且无残留 |

### 2.2 应用构建与启动

```bash
wails build                                  # → build/bin/灵感.app (12 MB, arm64)
build/bin/灵感.app/Contents/MacOS/灵感 --version    # dev
build/bin/灵感.app/Contents/MacOS/灵感 --selfcheck --data-dir /tmp/inspiration-selfcheck
# 自检通过：version=dev dataDir=/tmp/inspiration-selfcheck
# 目录结构：contracts/ inspiration.db logs/ templates/ update/
# 数据表：contracts meta settings templates（schema_version=1）
codesign -dv "build/bin/灵感.app"            # Identifier=com.serviceshare.inspiration（ad-hoc 签名）
```

GUI 启动实测：`INSPIRATION_DATA_DIR=/tmp/inspiration-demo 灵感.app/Contents/MacOS/灵感`
→ 进程存活，日志记录 `应用启动 version=dev dataDir=/tmp/inspiration-demo`。

### 2.3 前端界面验证

前端通过 `window.go.app.App` 访问绑定（与 Wails 生成的 `frontend/wailsjs/go/app/App.js` 一致，16 个方法全部对应）。
用 Chrome 加载构建后的 `frontend/dist` 并注入与绑定同形的桩数据，验证了渲染与交互（截图见 `evidence/screenshots/`）：

| 截图 | 验证点 |
|---|---|
| `01-contract-generate.png` | 侧边导航、模板选择框、十个字段的两列自适应网格、字段提示 |
| `02-contract-generate-filled.png` | 表单填写、选中模板信息（字段数）、生成成功后的提示与按钮 |
| `03-template-list.png` | 搜索框、上传按钮、模板列表（占位符数量徽标、大小、更新时间、操作） |
| `04-template-detail.png` | 详情弹窗：校验结论、未使用字段提示、占位符/次数/所在位置表 |

> 说明：截图为「前端 + 桩数据」的渲染验证，用于确认布局与交互逻辑；真实后端链路由 §2.1 的自动化测试覆盖。

**本次界面验证发现并修复的缺陷**：页面切换时已打开的模态框会残留（`ui.ts` 增加 `closeAllModals()`，路由切换时调用），已用脚本回归验证（切页后 `.modal-mask` 不存在）。

## 3. 验收对照（任务书 §6.2 第一阶段相关项）

| 验收项 | 状态 | 证据 |
|---|---|---|
| 应用能够启动、退出和重新启动 | 部分验证 | 启动 ✅（GUI 实测 + 自检）；退出/重启后的数据恢复 ✅（e2e 关闭重开数据库断言）；GUI 退出按钮未人工点击验证 |
| 合同生成和模板管理页面可用 | 部分验证 | 页面渲染与交互 ✅（截图）；页面到后端的真实连通需在本机点一次（Go 侧链路已由 e2e 覆盖） |
| 十个基础字段均为必填 | ✅ | `contract_test.go` 逐字段断言 |
| 邮箱校验正常 | ✅ | `contract_test.go` 10 个样例 |
| 模板上传、搜索、重命名和删除正常 | ✅ | `store_test.go` + `e2e_test.go`（导入/搜索/重命名/软删除） |
| 应用重启后模板数据仍存在 | ✅ | `e2e_test.go` 重启断言 |
| 所有已支持字段的占位符均被正确替换 | ✅ | 逐字段替换次数核对（23/23） |
| 重复占位符全部替换 | ✅ | `联系人姓名` 6 处全部替换 |
| 未知占位符会被提示并阻止生成 | ✅ | 三类问题模板均阻断且不产出文件 |
| 正文、表格、页眉、页脚均有真实文件测试记录 | ✅ | 语料 + 部件级断言 |
| 跨 Word 文本片段的占位符有专项测试 | ✅ | `template_split_runs.docx` 专项用例 |
| 生成的 DOCX 能被目标 Word 处理软件打开 | 部分验证 | LibreOffice 26.8.1 打开并转 PDF 通过；**Microsoft Word 未验证**（U4） |
| 原模板不会被修改 | ✅ | e2e 断言源模板字节与占位符不变 |
| 不会静默覆盖已有合同 | ✅ | `UniquePath` + e2e 两次生成断言 |
| 生成失败时不会丢失用户已填写的数据 | 部分验证 | 前端失败时保留表单内容（代码路径），未人工演练；字段值不落盘（设计决定） |

## 4. 与阶段 1 设计的偏差

| 偏差 | 原因 |
|---|---|
| 入口 `main.go` 放在仓库根目录（原设计为 `cmd/inspiration/`） | `//go:embed all:frontend/dist` 只能嵌入当前包目录及其子目录，Wails 前端资源必须在根包下 |
| 新增 `internal/id` | 手写 UUID v4（约 20 行）以替代 `github.com/google/uuid`，减少一个第三方依赖 |
| 生成记录不落盘字段值 | 落实「不保存敏感字段」原则；`generate` 失败时由前端保留内存中的表单内容 |

## 5. 已知限制与未验证项（阶段 2 结束时）

| # | 项 | 状态 |
|---|---|---|
| U1 | Windows x64 真机运行与打包 | 未验证（本机 macOS；阶段 4 CI 构建 + 阶段 6 用户验收） |
| U2 | macOS Intel 真机运行 | 未验证 |
| U3 | 真机 GUI 完整点击流程（上传 → 填写 → 生成 → 打开文件） | 未验证（需人工在桌面点一次） |
| U4 | Microsoft Word 打开生成结果 | 未验证（LibreOffice 已验证） |
| U5 | 大模板（数十 MB、含大量图片）解析与生成耗时 | 未验证（阶段 3 用真实大文件测） |
| U6 | 只读目录、磁盘写满、超长路径等异常路径 | 未验证（阶段 3） |
| U7 | 窗口缩放/最小尺寸下的布局检查 | 未验证（阶段 3） |
| — | 引擎已知限制（被 `w:br` 打断的占位符、跨段落残缺、CDATA、域代码、图表/艺术字文本） | 见 `docs/phase1/docx-evaluation.md` §4，均在生成前阻止或明确不支持 |

## 6. 阶段 3 计划

1. 长文本与中文输入实操测试（含输入法、粘贴、超长地址）；
2. 更多真实 Word 模板回归（含嵌套表格、修订痕迹、页眉页脚多节、文本框组合）；
3. 异常路径测试：只读目录、磁盘写满、超长路径、模板被外部修改/删除、数据库损坏；
4. 窗口缩放与最小尺寸布局检查（960×640 下限）；
5. 应用重启/崩溃后的数据一致性（含 WAL 未提交场景）；
6. 应用图标与平台图标集成（`build/appicon.png`、Windows `.syso`、macOS icns）；
7. 性能基准（大模板解析/生成耗时，设定可接受阈值）。
