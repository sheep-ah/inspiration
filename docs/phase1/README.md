# 灵感（Inspiration）— 阶段 1 技术评审结论

本目录是任务书**阶段 1**的全部交付物。所有“已验证”结论都有可复现的命令与原始输出，未验证项集中在 `risks-and-unverified.md`。

## 1. 交付物索引

| 文件 | 内容 |
|---|---|
| `gui-evaluation.md` | Wails 与 Fyne 十项对比、实测数据、选型结论与冻结约束 |
| `docx-evaluation.md` | DOCX 处理方案评估（4 个候选）、自研引擎设计与实测指标、已知限制 |
| `build-release-update.md` | 三平台构建/分发、CI/CD 设计、应用内更新方案与平台差异 |
| `architecture.md` | 项目目录设计、数据目录规范、数据库结构、安全与隐私约定 |
| `risks-and-unverified.md` | 风险清单、未验证项清单、需要用户决策的事项 |
| `evidence/report.json` | Go 侧原始结果（占位符扫描、各库行为、阻止生成验证） |
| `evidence/verify.json`、`evidence/verify_out.txt` | 独立校验（Python + lxml + LibreOffice）原始结果 |
| `../../poc/docx-eval/` | 可复现的引擎原型、测试语料、独立校验脚本 |

## 2. 选型结论（阶段门禁核心）

| 决策项 | 结论 | 依据 |
|---|---|---|
| GUI 框架 | **Wails v2.16.0**（前端 `vanilla-ts`） | 中文/长文本输入依赖系统原生输入控件；Fyne 存在开放的 CJK 输入缺陷（#4544/#4546/#5818/#618）；包体积 7.8 MB vs 31.7 MB；NSIS 安装器与 `.app` 整包更新路径成熟 |
| DOCX 处理 | **自研 OOXML 引擎**（`archive/zip` + `encoding/xml`，字节级替换） | 三个候选库实测全部不合格：无替换 API/重写模板结构、`{{x}}` 解析错误并 panic、跨 run 片段全漏替；自研引擎在含正文/表格/页眉/页脚/脚注/两个文本框/SDT 的语料上 23/23 替换成功，跨 run 片段一致，未修改部件字节完全不变 |
| 本地存储 | SQLite：`modernc.org/sqlite` v1.60.1（纯 Go，无 cgo）+ 应用数据目录存放 DOCX | 已实测三平台（darwin/arm64、darwin/amd64、windows/amd64）`CGO_ENABLED=0` 构建通过，本机读写往返成功 |
| 应用内更新 | 自研更新器（GitHub Releases 检测 + 进度下载 + sha256 校验 + 平台安装） | 需要进度事件与平台差异处理；`go-selfupdate` v1.6.0 已评估为可选替代 |
| 构建/发布 | 推送 `main` → 三平台原生 runner 构建 + artifact；`v*` 标签 → 校验 + 全平台构建 + 校验和 + GitHub Releases | 任务书 §5 要求；交叉编译仅用于本地验证，不作为交付依据 |

## 3. 关键能力验证矩阵（阶段 1 门禁：核心依赖的关键能力已验证）

| 能力 | 状态 | 证据 |
|---|---|---|
| Wails 构建 darwin/arm64 | ✅ 已验证 | `wails build` → 7.8 MB `.app`，33.8s |
| Wails 构建 darwin/amd64 | ✅ 已验证 | `wails build -platform darwin/amd64`，29.3s |
| Wails 构建 darwin/universal | ✅ 已验证 | 16 MB `.app` |
| Wails 构建 windows/amd64 + NSIS 安装包 | ✅ 已验证 | exe 11.4 MB，安装包 6.1 MB（需 mingw-w64 + nsis） |
| Fyne 构建三平台（对照） | ✅ 已验证 | arm64 31.7 MB / amd64 33.3 MB / windows 43.8 MB |
| SQLite 纯 Go 驱动三平台构建 + 运行 | ✅ 已验证 | `CGO_ENABLED=0` 交叉编译通过；本机 INSERT/SELECT 往返成功 |
| DOCX 占位符替换（正文/表格/页眉/页脚/脚注/文本框/SDT） | ✅ 已验证 | 23/23，见 `evidence/verify.json` |
| 跨 Word 文本片段（run 拆分） | ✅ 已验证 | 拆分模板与未拆分模板结果一致 |
| 重复占位符全部替换 | ✅ 已验证 | `联系人姓名` 6 处全部替换 |
| 未知/未闭合/被打断占位符阻止生成 | ✅ 已验证 | 三类模板 `Render` 报错且不产出文件 |
| 格式保留（未修改部件字节不变） | ✅ 已验证 | 仅 4 个 WML 部件变化，其余条目逐字节相同 |
| 生成文件可被文字处理软件打开 | ✅ 已验证（LibreOffice 26.8.1） | 全部产物转 PDF 成功，渲染文本无残留占位符 |
| 特殊字符 / 前导零 / 长文本 | ✅ 已验证 | `张三 & 李四`、`李四<测试>`、`0001234567890123456`、长中文地址 |
| 损坏/加密/非 DOCX 文件的错误提示 | ✅ 已验证 | 三类输入均返回明确中文提示 |
| 三平台真机安装与运行 | ❌ 未验证 | 见 `risks-and-unverified.md` U1/U2/U3 |
| Microsoft Word 打开产物 | ❌ 未验证 | 本机无 Word（仅 LibreOffice） |
| GitHub Actions 真实运行 | ❌ 未验证 | 阶段 4 交付 |
| 代码签名/公证 | ❌ 未验证 | 依赖证书（见 D2） |

## 4. 阶段 2 进入条件与计划

门禁条件已满足：**选型结论明确（Wails v2.16.0 + 自研 docx 引擎 + modernc.org/sqlite）；核心依赖的关键能力已实测验证。**

阶段 2（MVP）实施顺序（每步都以可运行、可测试为准，不做纯演示界面）：

1. 骨架：`wails init -t vanilla-ts`、目录结构、`internal/version`、Makefile、日志与数据目录初始化；
2. 存储层：SQLite 打开/迁移、模板与合同表、文件存储（uuid 命名、sha256、事务补偿）；
3. DOCX 引擎落地：把 `poc/docx-eval/go-eval/engine` 移入 `internal/docx`，补齐单元测试（语料直接作为 `testdata`）；
4. 模板服务：导入（选文件→复制→校验→入库）、模糊搜索、改名、详情（占位符列表）、删除（二次确认+软删除）；
5. 合同生成：10 字段表单 + 校验（必填、邮箱格式、长文本）、目标路径选择、重名自动加后缀、生成后打开文件/目录；
6. 端到端：真实 DOCX 走通“导入→生成→打开”全流程（阶段门禁要求真实文件，非模拟数据）。

## 5. 复现阶段 1 的全部验证

```bash
# 1) GUI 框架对照（见 gui-evaluation.md §5）
# 2) DOCX 引擎评估
cd poc/docx-eval
python3 -m pip install python-docx lxml pypdf
python3 make_corpus.py
cd go-eval && go run . ../corpus ../out
cd .. && python3 verify.py corpus out     # 需 LibreOffice（brew install --cask libreoffice）
```
