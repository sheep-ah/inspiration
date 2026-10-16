# 灵感（Inspiration）

本地运行的合同生成桌面应用：导入 DOCX 模板，填写 11 个基础字段，一键生成合同。合同内容与模板文件默认只在本机处理，不上传。

- 下载安装：[GitHub Releases](https://github.com/sheep-ah/inspiration/releases)（当前版本 **v0.1.4**；Windows 用户若装的是 v0.1.1 及更早版本，请手动装一次 v0.1.2 或更高版本——旧版自带的更新器有缺陷）
- 使用说明：[`docs/user-guide.md`](docs/user-guide.md) ｜ 开发文档：[`docs/development.md`](docs/development.md) ｜ 验收报告：[`docs/phase6/acceptance-report.md`](docs/phase6/acceptance-report.md)

- 应用形态：桌面应用（Windows x64 / macOS Intel / macOS Apple Silicon）
- 技术栈：Go + Wails v2（前端 TypeScript + Vite）+ SQLite（纯 Go 驱动）
- 输出格式：DOCX（默认）与 **DOC（Word 97-2003，需本机安装 LibreOffice 转换）**；不支持 PDF

## 当前进度

| 阶段 | 状态 |
|---|---|
| 阶段 1 技术评审（选型、关键能力验证、实施计划） | ✅ 已完成，见 [`docs/phase1/`](docs/phase1/README.md) |
| 阶段 2 最小可行版本（模板管理 + 合同生成 + 本地存储） | ✅ 已实现并通过自动化验收，见 [`docs/phase2/`](docs/phase2/README.md)；真机点击验收待用户确认 |
| 需求扩展：`.doc` 模板与 `.doc` 输出 | ✅ 评估 + 导出侧实现完成，见 [`docs/doc-support-evaluation.md`](docs/doc-support-evaluation.md)（用户侧 Word/WPS 核对待确认） |
| 阶段 3 完善与稳定性测试（异常路径、性能、图标、静态检查） | ✅ 已完成，见 [`docs/phase3/`](docs/phase3/README.md)；中文输入法实操与真机项待人工确认 |
| 阶段 4 跨平台 CI/CD（main 自动测试+三平台构建、标签发布） | ✅ 已完成并**在真实 CI 上跑通**（六 job 全绿，三平台产物均真实运行验证），见 [`docs/phase4/`](docs/phase4/README.md) |
| 阶段 5 应用内更新 | ✅ 已完成：`v0.1.0` 已正式发布，macOS 侧用真实发布包端到端验证通过（检查 → 下载 → 校验 → 安装 → 重启 → 数据保留），见 [`docs/phase5/`](docs/phase5/README.md)；Windows 更新路径待真机验证 |
| 阶段 6 最终验收与交付 | ✅ 已完成：验收报告 [`docs/phase6/acceptance-report.md`](docs/phase6/acceptance-report.md)、用户说明 [`docs/user-guide.md`](docs/user-guide.md)、开发文档 [`docs/development.md`](docs/development.md)；未验证项已逐条标注 |
| 交付后修复：macOS dmg 缺安装别名 | ✅ 已修复并随 **v0.1.1** 发布（`scripts/make-dmg.sh` + 打包时挂载自检）：已下载发布包实测——卷内含 `灵感.app` 与 `Applications → /Applications` 别名，sha256 与 `checksums.txt` 一致。见 [`docs/phase4/README.md`](docs/phase4/README.md) §3.4 |
| 交付后修复：Windows「打开所在文件夹」无反应 | ✅ 已修复并随 **v0.1.4** 发布（GUI 程序不能设 SW_HIDE——会把要打开的窗口一起隐藏；`/select` 路径加引号；失败兜底为打开所在目录），见 [`docs/phase6/acceptance-report.md`](docs/phase6/acceptance-report.md) §9.7 |
| 交付后修复：Windows 应用内更新静默失败 | ✅ 已修复并随 **v0.1.2** 发布（脚本强制 CRLF + 纯 ASCII、路径走环境变量、`ping` 延时、隐藏窗口、等待上限），见验收报告 §9.5；Windows 侧已由 CI 真实执行脚本，真机界面流程待复验；`v0.1.0`/`v0.1.1` 自带更新器仍是坏的，需手动装一次 v0.1.2 |
| 交付后修复：Windows 子进程弹命令行窗口 | ✅ 已修复并随 **v0.1.1** 发布（`internal/sysproc` 抑制控制台窗口，覆盖转换探测、`.doc` 转换、提示框、打开文件全部启动点）；CI 的 windows runner 真实执行了专项测试，见 [`docs/phase6/acceptance-report.md`](docs/phase6/acceptance-report.md) §9.2；Windows 上的视觉效果待用户复验 |

## 阶段 1 选型结论（已冻结）

| 决策项 | 结论 |
|---|---|
| GUI 框架 | Wails **v2.16.0**（前端 `vanilla-ts`）；Fyne 因 CJK 输入法缺陷与体积不采用 |
| DOCX 处理 | **自研 OOXML 引擎**（`archive/zip` + `encoding/xml`，字节级替换，保留原始格式） |
| 本地存储 | SQLite（`modernc.org/sqlite` v1.60.1，纯 Go）+ 应用数据目录存放 DOCX |
| 应用内更新 | 自研更新器（GitHub Releases 检测 + 进度下载 + sha256 校验 + 平台安装） |
| 构建发布 | 推送 `main` → 三平台构建 + artifact；`v*` 标签 → 校验 + 构建 + 校验和 + GitHub Releases |

详细对比、实测数据、已知限制与未验证项：

- [`docs/phase1/gui-evaluation.md`](docs/phase1/gui-evaluation.md) — Wails 与 Fyne 十项对比与选型依据
- [`docs/phase1/docx-evaluation.md`](docs/phase1/docx-evaluation.md) — DOCX 方案评估、引擎设计与实测指标
- [`docs/phase1/build-release-update.md`](docs/phase1/build-release-update.md) — 构建分发、CI/CD、更新方案
- [`docs/phase1/architecture.md`](docs/phase1/architecture.md) — 目录设计、数据存储、安全约定
- [`docs/phase1/risks-and-unverified.md`](docs/phase1/risks-and-unverified.md) — 风险、未验证项、待决策事项

## 用户与开发文档

- [`docs/user-guide.md`](docs/user-guide.md) — **使用说明**：安装（含未签名放行）、三步上手、占位符写法与 11 个字段表、模板校验提示含义、`.doc` 输出差异、数据位置与备份、应用内更新、常见问题、已知限制
- [`docs/development.md`](docs/development.md) — **开发文档**：技术栈与选型依据、架构与目录、核心模块（引擎/存储/转换器/更新器）、本地开发命令、测试策略与 6 类跨平台陷阱、CI/CD 与发布流程、扩展指南、编码约定
- [`docs/phase6/acceptance-report.md`](docs/phase6/acceptance-report.md) — **最终验收报告**：§6.1–§6.4 逐条对照任务书要求与证据，含"未验证项"清单

## 独立需求评估

- [`docs/doc-support-evaluation.md`](docs/doc-support-evaluation.md) — **.doc（Word 97-2003）模板与输出支持评估**：技术事实、三条路线对比（含实测数据）、推荐方案与实施计划。工具：`scripts/convert-doc-templates.sh`（批量 .doc → .docx + 模板体检）、`灵感 --inspect <file.docx>`（模板体检，不需要数据目录）

## 自动构建与发布

- 推送 `main` / PR → `.github/workflows/ci.yml`：格式检查、静态分析、单元与端到端测试、**三平台原生构建**（Windows 便携版 + NSIS 安装包、macOS arm64/amd64 的 zip 与 dmg），产物作为 CI Artifact（不发布正式版本）
- 推送 `v1.2.3` 形式的标签 → `.github/workflows/release.yml`：校验标签与分支 → 测试门禁 → 三平台构建 → 生成 `checksums.txt` 与 `latest.json` → **先建 draft、校验 8 个资产齐全后再转为正式发布**

发布操作见 [`docs/phase4/README.md`](docs/phase4/README.md) §4。

## 转换通道自检（生成 .doc 前建议先跑一次）

```bash
灵感 --check-converters                  # 查看 Word / WPS / LibreOffice 哪些可用
灵感 --selfcheck --run-converter-check   # 真的转换一次，验证链路与自检
```

## 快速开始

```bash
# 依赖：Go 1.26+、Node 20+、Wails CLI v2.16
go install github.com/wailsapp/wails/v2/cmd/wails@latest

make test      # 单元测试 + 真实 DOCX 端到端测试
make dev       # 开发模式（热重载）
make build     # 构建当前平台到 build/bin/
```

构建产物：`build/bin/灵感.app`（macOS）。数据目录（模板、数据库、日志）：macOS `~/Library/Application Support/灵感/`，Windows `%APPDATA%\灵感\`。

## 仓库结构

```
main.go               Wails 入口（go:embed 前端资源，因此位于根包）
internal/docx         DOCX 占位符引擎 + 真实语料 testdata
internal/store        SQLite 元数据 + 模板文件存储
internal/template     模板导入/搜索/详情/重命名/删除
internal/contract     十个字段模型、校验与生成
internal/app          Wails 绑定层与事件
internal/e2e          真实 DOCX 端到端测试
frontend/             Wails 前端（vanilla-ts + Vite）
build/                图标、平台清单、安装器脚本
docs/phase1/          阶段 1 技术评审交付物与原始证据
docs/phase2/          阶段 2 交付与验收记录（含界面截图）
poc/docx-eval/        阶段 1 DOCX 引擎原型、测试语料与独立校验脚本
```

## 复现阶段 1 的验证

```bash
cd poc/docx-eval
python3 -m pip install python-docx lxml pypdf
python3 make_corpus.py
cd go-eval && go run . ../corpus ../out
cd .. && python3 verify.py corpus out        # 需要 LibreOffice
```

## 开发环境要求（阶段 2 起）

- Go 1.26.x
- Node.js ≥ 20、npm ≥ 10
- Wails CLI v2.16.0：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- macOS 需 Xcode Command Line Tools；Windows 打包需 NSIS（`choco install nsis`）

## 输出格式说明

| 格式 | 可用条件 | 说明 |
|---|---|---|
| DOCX | 始终可用 | 默认；本地生成，格式保真最好 |
| DOC（Word 97-2003） | 本机装有 **Microsoft Word / WPS Office / LibreOffice** 任一 | 多通道按保真度排序（Word → WPS → LibreOffice）转换，**不上传任何内容**；产物必须是真 OLE2 `.doc` 且通过回读自检，否则丢弃并报错；默认**同时保留一份 `.docx` 备份** |

## 模板格式

| 上传格式 | 行为 |
|---|---|
| `.docx` | 直接导入 |
| `.doc`（Word 97-2003） | 本机 Word / WPS / LibreOffice 自动转换为 `.docx` 后导入，**原始 `.doc` 保留备查**；导入后会提示核对版式 |

> 老模板若没有 `{{字段名}}` 占位符，导入时会明确提示；按字段名（如 `{{联系人姓名}}`）插入占位符后重新上传即可。当前字段：联系人姓名、法人代表、实际经营地址、联系人邮箱、营业执照名称、纳税人识别号、电话、联系人电话、开户银行、账户号、开票地址。

## 数据处理原则

- 默认本地处理，不上传合同内容、模板文件或字段值。
- 除“检查更新”外不发起网络请求。
- 日志不记录合同内容、银行账号、税号、电话等敏感信息。
