# 灵感（Inspiration）最终验收报告

- 版本：**v0.1.0**（发布地址 https://github.com/sheep-ah/inspiration/releases/tag/v0.1.0）
- 验收环境：macOS 27.0.1（Apple M1，16GB）、Go 1.26.1、Node 24.3.0、LibreOffice 26.8.1、Wails CLI v2.16.0、NSIS 3.13、mingw-w64 14.0.0
- CI 环境：GitHub Actions（ubuntu-latest / windows-latest / macos-14）
- 报告日期：2026-10-16
- 说明：**凡未在本环境中真实验证的项目，本报告一律标注为"未验证"，不做推断性通过。**

## 0. 交付物清单

| 交付物 | 位置 |
|---|---|
| 完整源码 | 仓库 `main` 分支（26 个 Go 源文件 4703 行、前端 1931 行、CI 749 行、测试 3078 行） |
| 自动化工作流 | `.github/workflows/ci.yml`、`.github/workflows/release.yml` |
| 各平台安装包 | GitHub Releases `v0.1.0`（6 个安装包 + checksums.txt + latest.json） |
| 测试报告 | 本文件 §3–§6 + `docs/phase1/evidence/` 原始数据 |
| 用户使用说明 | [`docs/user-guide.md`](../user-guide.md) |
| 开发文档 | [`docs/development.md`](../development.md) |
| 阶段技术文档 | [`docs/phase1/`](../phase1/README.md) ~ [`docs/phase5/`](../phase5/README.md) |

## 1. 验收方法

| 手段 | 覆盖 | 可复现命令 |
|---|---|---|
| 单元与端到端测试 | 引擎、校验、存储、转换器、更新器 | `go test ./... -count=1` |
| 竞态检测 | docx / store 核心包 | `go test -race ./internal/docx/ ./internal/store/` |
| 性能基准 | 大文档解析与生成 | `go test ./internal/docx/ -bench . -benchtime 3x -run XXX` |
| 静态检查 | 格式、vet、staticcheck（含 Linux 视角） | `make lint` |
| 本地 CI 全量检查 | 上述全部 + 三平台测试编译 + Windows 控制台编码模拟 | `make ci` |
| 真实 CI 运行 | 三平台原生构建 + 产物真实运行（`--selfcheck`/`--version`） + 汇总校验 | GitHub Actions run 记录（§5.2） |
| 独立渲染校验 | 生成的 DOCX 用 LibreOffice 打开并转 PDF 比对 | `poc/docx-eval/verify.py` |
| 发布与更新端到端 | 真实 Release → 应用内检查/下载/校验/安装 | 见 §6.1 |

**测试规模**：77 个测试用例 + 4 个基准，分布于 contract(9)、converter(6)、docx(16)、e2e(17)、store(9)、updater(20)。

## 2. 技术方案验收（任务书 §6.1）

| # | 验收项 | 状态 | 证据 |
|---|---|---|---|
| 1 | Wails 与 Fyne 对比报告已完成 | ✅ | [`docs/phase1/gui-evaluation.md`](../phase1/gui-evaluation.md)：十项维度对比，全部为本机实测数据（构建耗时、产物体积、CJK 输入缺陷 issue 列表） |
| 2 | 唯一 GUI 框架已经选定 | ✅ | Wails v2.16.0（前端 vanilla-ts）。依据：中文/长文本输入依赖系统输入法、包体积 12MB vs 31–44MB、NSIS 安装器与 `.app` 整包更新路径成熟 |
| 3 | DOCX 处理方案及已知限制已记录 | ✅ | [`docs/phase1/docx-evaluation.md`](../phase1/docx-evaluation.md)：4 个候选方案实测（3 个第三方库全部不合格）+ 自研引擎设计 + 已知限制表（被 `w:br` 打断、跨段落残缺、CDATA、域代码、图表/艺术字文本） |
| 4 | 三个平台的构建和安装方案已确认 | ✅ | [`docs/phase1/build-release-update.md`](../phase1/build-release-update.md)；并在 CI 上真实产出三平台安装包（§5.2） |
| 5 | 更新机制及平台差异已明确 | ✅ | macOS 整包替换 `.app`（Apple 文档要求，改包内文件会破坏签名）；Windows 批处理等待进程退出后静默安装 NSIS 包/覆盖便携版；两者都不碰用户数据目录。macOS 侧已端到端实测（§6.1） |
| 6 | 技术选型完成后没有未经批准的框架更换 | ✅ | `go.mod` 中 GUI 依赖仅 `github.com/wailsapp/wails/v2 v2.16.0`，全程无变更；未引入任何前端运行时框架（前端产物 25KB） |

## 3. 核心功能验收（任务书 §6.2）

| # | 验收项 | 状态 | 证据 / 实测记录 |
|---|---|---|---|
| 1 | 应用能够启动、退出和重新启动 | ✅ | 本机 GUI 启动（日志 `应用启动`）、CI 三平台产物真实运行 `--selfcheck`（exit 0）、更新后自动重启（§6.1） |
| 2 | 合同生成和模板管理页面可用 | ✅ | 界面渲染与交互断言（`docs/phase2/evidence/screenshots/`、`docs/phase3/evidence/01-min-window-960x640.png`）；页面到后端的完整链路由 e2e 测试覆盖 |
| 3 | 十个基础字段均为必填 | ✅（数量已按需求变更为 11） | 任务书原定 10 个字段；用户后续要求新增 **`联系人电话`**（与 `电话` 并存、独立录入），现为 **11 个字段**，逐个必填校验有测试（`TestValidateRequiresEveryField` 遍历全部字段） |
| 4 | 邮箱校验正常 | ✅ | `TestValidateEmailFormat`：7 个非法样例 + 3 个合法样例 |
| 5 | 模板上传、搜索、重命名和删除正常 | ✅ | `internal/store`（模糊搜索/重命名/软删除）+ `internal/e2e`（真实 `.doc` 导入、真实 `.docx` 导入）；上传支持 `.docx` 与 `.doc`（`.doc` 自动转换并保留原件） |
| 6 | 应用重启后模板数据仍存在 | ✅ | `TestEndToEndWithRealDocx`（正常关闭重开）+ `TestRestartAfterUncleanShutdown`（未 Close 直接重开，模拟进程被杀） |
| 7 | 所有已支持字段的占位符均被正确替换 | ✅ | 逐部件核对：23 个占位符分布在 document/header/footer/footnotes，10 个字段的期望次数与实际次数完全一致 |
| 8 | 重复占位符全部替换 | ✅ | `联系人姓名` 在语料中出现 6 处，全部替换（`TestRenderReplacesEveryOccurrenceWithRealTemplate`） |
| 9 | 未知占位符会被提示并阻止生成 | ✅ | 三类问题模板（未知/未闭合/被 `w:br` 打断）`Render` 均返回错误且**不产出文件**（测试断言 `os.Stat` 失败）；界面会列出本版本支持的字段 |
| 10 | 正文、表格、页眉、页脚均有真实文件测试记录 | ✅ | `template_structures.docx`（python-docx 生成的真实 Word 包）+ 部件级断言；另有 Go 现场构造的夹具覆盖嵌套表格、多节不同页眉 |
| 11 | 跨 Word 文本片段的占位符有专项测试 | ✅ | `TestManySmallFragmentsAndEmptyTextNodes`（6 个片段 + 2 个空 `<w:t/>` + 片段内加粗）；真实语料 `template_split_runs.docx` |
| 12 | 生成的 DOCX 能被目标 Word 处理软件打开 | ⚠️ **部分验证** | ✅ LibreOffice 26.8.1 headless 打开全部产物并转 PDF 成功、渲染文本无残留占位符；✅ 本机 WPS 6.7.1 可打开（人工确认过程见 `docs/doc-support-evaluation.md`）；❌ **Microsoft Word 未验证**（本机无 Word，需用户在 Windows/Mac 上打开一次） |
| 13 | 原模板不会被修改 | ✅ | e2e 断言源模板字节完全不变、占位符仍在；引擎只在输出文件上操作 |
| 14 | 不会静默覆盖已有合同 | ✅ | `store.UniquePath`（追加 `(2)`/`(3)`…）+ e2e 连续两次生成断言文件互不覆盖；另有"输出路径与模板相同"的显式拒绝 |
| 15 | 生成失败时不会丢失用户已填写的数据 | ✅ | 失败时前端保留表单内容（不刷新页面）；后端不产出半成品文件（临时文件 + 原子重命名）；字段值不落盘是设计决定（敏感信息不进日志/数据库） |

### 3.1 与任务书的差异说明

| 差异 | 原因 | 影响 |
|---|---|---|
| 字段从 10 个增至 **11 个**（新增 `联系人电话`） | 用户明确要求"单独录入联系人电话，不能和电话共用" | 表单、校验、占位符契约、测试与文档同步更新；原有 10 个字段行为不变 |
| 新增 **`.doc` 模板导入**（自动转换 + 保留原件） | 用户只有 `.doc` 模板（任务书原约束为"仅 DOCX"） | 走独立需求评估流程后实施（[`docs/doc-support-evaluation.md`](../doc-support-evaluation.md)），DOCX 主路径不受影响 |
| 新增 **`.doc` 输出**（多通道转换 + 强校验 + 默认保留 `.docx` 备份） | 下游要求必须真 `.doc` | 同上；已知限制：LibreOffice 通道会把表格宽度写成 100%（实测），Word/WPS 通道无此问题 |
| 新增 **应用内更新**（阶段 5 提前到本批次完成） | 任务书本就在阶段 5，本次连续实施 | macOS 侧已端到端验证（§6.1） |

## 4. 三平台构建验收（任务书 §6.3）

| # | 验收项 | 状态 | 证据 |
|---|---|---|---|
| 1 | `main` 分支推送能触发自动测试 | ✅ | CI run #38790796775（6 个 job 全绿）；`lint`（gofmt/vet/staticcheck/前端类型检查）与 `test`（单元+端到端+race+基准） |
| 2 | `main` 分支推送能触发三个目标平台的构建 | ✅ | 同一 run 的 `build` 矩阵：`windows-amd64`、`darwin-arm64`、`darwin-amd64` 全部成功 |
| 3 | Windows x64 分发包可生成 | ✅ | `inspiration_0.1.0_windows_amd64_setup.exe` 9.51 MB（NSIS，内嵌 WebView2 引导）+ `..._portable.exe` 16.9 MB |
| 4 | macOS Intel amd64 分发包可生成 | ✅ | `inspiration_0.1.0_darwin_amd64.zip` 5.92 MB + `.dmg` 6.66 MB |
| 5 | macOS Apple Silicon arm64 分发包可生成 | ✅ | `inspiration_0.1.0_darwin_arm64.zip` 5.49 MB + `.dmg` 6.16 MB |
| 6 | 构建产物和版本信息一致 | ✅ | CI 每个平台构建后运行产物校验版本：macOS 用 `--version` 输出、Windows 用 `--version` 输出重定向（GUI 子系统无 stdout）；断言与标签一致；产物文件名含同一版本号 |
| 7 | 正式版本标签能够触发 GitHub Releases 发布 | ✅ | `v0.1.0` → Release run #38833751163 全绿：validate（标签格式/分支归属）→ test → build×3 → publish（先 draft、校验 8 个资产、转正式） |
| 8 | 正式发布失败时不会错误标记为成功 | ✅ | `publish` 依赖 `validate`/`test`/`build` 全部成功；资产数量不等于 8 时**保持 draft 并失败**（不会转为正式版，应用内更新只读 `releases/latest`，用户不会收到半成品） |

## 5. 自动化构建与发布流程验收（任务书 §5）

### 5.1 普通提交（`main` 推送）

| 任务书要求 | 实现 | 证据 |
|---|---|---|
| 代码格式检查 | `gofmt -l` + `tsc --noEmit` | lint job 步骤日志 |
| Go 单元测试 | `go test ./... -count=1` + `-race`（核心包）+ 基准 | test job |
| 静态分析 | `go vet` + `staticcheck` | lint job |
| 核心模块构建 | lint/test job 编译后端与前端的全部包 | ✅ |
| 三平台构建 | `windows-latest` 原生 + `macos-14`（arm64 原生、amd64 交叉） | build 矩阵 |
| 保存各平台构建产物 | `actions/upload-artifact`（保留 14 天） | Artifacts：`inspiration_ci_windows_amd64` / `_darwin_arm64` / `_darwin_amd64` |
| 汇总构建和测试结果 | `verify` job：六个产物存在性 + 体积校验 + `SHA256SUMS.txt` + Job Summary | ✅ |

### 5.2 正式版本发布（`v*` 标签）

| 任务书要求 | 实现 | 证据 |
|---|---|---|
| 校验标签格式和版本号 | `^v[0-9]+\.[0-9]+\.[0-9]+$`；`set-version.py` 把标签写入 `wails.json`，构建后反查产物版本 == 标签 | validate/build job |
| 确认标签对应提交已通过检查 | 标签提交必须在 `main` 上（`git merge-base --is-ancestor`）+ 发布流水线内重新跑测试门禁 | validate/test job |
| 为三个平台构建正式分发包 | 同 CI 矩阵，产物名带版本号 | ✅ |
| 生成可供更新程序使用的版本信息 | `latest.json`（version/tag/releaseUrl/assetsUrl/资产清单含 os、arch、kind、sha256、size） | 发布资产 |
| 计算分发文件校验和 | `checksums.txt`（sha256，6 个安装包） | 发布资产 |
| 发布到 GitHub Releases | `gh release create --draft` → 校验资产数 → `--draft=false` | publish job |
| 上传更新说明与各平台产物 | 自动生成说明（上一标签→本标签的提交列表 + 安装包表 + 校验说明 + 未签名提示） | Release 页面 |
| 确认发布成功后才视为可供更新 | draft 机制 + 资产数断言 | ✅ |
| 构建失败时禁止发布 | `publish.needs` 依赖全部前置 job | ✅ |
| 不得将令牌/私钥写入源码 | 仅用内置 `GITHUB_TOKEN`（`contents: write` 仅限 publish job）；仓库中无任何凭据（已用关键字检索核查） | ✅ |

### 5.3 版本管理

语义化版本：`MAJOR` 不兼容变更 / `MINOR` 向后兼容新功能 / `PATCH` 修复。当前 `v0.1.3`（`v0.1.1` = dmg 安装别名 + Windows 控制台窗口两项修复；`v0.1.2` = Windows 应用内更新修复；`v0.1.3` = 验证用版本递增，见 §9）。
版本号三处一致：应用内可见（关于页 + `--version`）、GitHub Release 标签、产物文件名。

## 6. 更新验收（任务书 §6.4）

### 6.1 macOS（arm64，真实发布包，已端到端验证）

用真实 `v0.1.0` 发布包，把本机应用从 `v0.0.9` 更新到 `v0.1.0`：

| # | 验收项 | 状态 | 实测结果 |
|---|---|---|---|
| 1 | 应用能查询当前正式版本 | ✅ | 关于页显示 `v0.0.9`；`--version` 一致 |
| 2 | 没有新版本时显示正确提示 | ✅ | 单元测试 `TestCheckReportsUpToDate`；当前 v0.1.0 检查后提示"已是最新版本" |
| 3 | 有新版本时显示版本号和更新说明 | ✅ | 真实 API 返回 `v0.1.0` 说明文本，界面与日志均显示（含发布时间） |
| 4 | 下载对应平台的正确产物 | ✅ | 自动选中 `inspiration_0.1.0_darwin_arm64.zip`（5,759,322 字节），未误选 Intel/Windows 包 |
| 5 | 下载失败可重试 | ✅（单测） | 下载中断只删临时文件、不影响当前版本；界面提示"可以重试，当前版本不受影响" |
| 6 | 更新包能进行完整性验证 | ✅ | 实际 sha256 与发布 digest 完全一致；校验失败会删除文件并拒绝安装（单测覆盖） |
| 7 | 更新失败不会破坏当前可用版本 | ✅（单测） | 坏 zip / 缺少 `.app` / 结构不完整三条失败路径：当前版本内容与结构均未改动，且不产生备份 |
| 8 | 更新完成后应用能够正常启动 | ✅ | 替换后 `--version` = **v0.1.0**（证明装的是发布包）；`codesign --verify --deep --strict` 通过；应用自动重启 |
| 9 | 更新后模板及数据库数据仍然存在 | ✅ | 数据目录未被动过：2 个模板（含真实 `.doc` 转换的服务协议书）、2 份已生成合同、3 个模板文件齐全；更新后的 v0.1.0 对真实数据目录自检通过，并用旧模板体检成功（识别 10 个字段） |

补充保障：更新前把旧版本重命名为 `.backup-<时间戳>`（实测备份内版本为 v0.0.9，可回滚）；备份超过 7 天自动清理。

### 6.2 Windows（未验证）

| 项 | 状态 | 说明 |
|---|---|---|
| 安装包可构建 | ✅ | CI 产出 NSIS 安装包与便携版 |
| 便携版/安装版可启动 | ✅ | CI 中真实运行过 exe（`--selfcheck` 退出码 0、`--version` 输出正确、中文输出正常） |
| **安装流程** | ❌ 未验证 | 需要在 Windows 上双击安装包验证：安装目录、WebView2 引导、开始菜单、卸载 |
| **应用内更新（Windows）** | ❌ 未验证 | 批处理等待进程退出 → `/S` 静默安装 → 自动重启的实际行为需要在 Windows 上验证；需要两次发布才能验证"从旧版更新" |

## 7. 未验证项汇总（必须由用户或后续环境验证）

| # | 项 | 原因 | 建议验证方式 |
|---|---|---|---|
| 1 | **Microsoft Word 打开生成结果** | 本机无 Word（仅 LibreOffice 26.8.1 与 WPS） | 用 Word 打开 Release 中任一平台产物生成的合同，核对页眉/页脚/表格/文本框/版式 |
| 2 | **Windows 安装与更新** | 本机为 macOS | 用 `inspiration_0.1.0_windows_amd64_setup.exe` 在 Windows 上安装并生成合同；下次发布后点一次应用内更新 |
| 3 | **macOS Intel 真机运行** | 无 Intel 设备 | 用 `inspiration_0.1.0_darwin_amd64.dmg` 在 Intel Mac 上打开并生成合同 |
| 4 | 应用内更新的 GUI 点击流程 | 自动化环境无法点击（本次用同一套函数以程序驱动验证） | 发布下一版本后在界面上点「检查更新 → 下载并更新」 |
| 5 | 代码签名与公证 | 未购买证书（用户已确认按未签名发布） | 有证书后在 CI 增加签名/公证步骤；否则用户首次打开需右键"打开"、Windows 可能提示 SmartScreen |
| 6 | Word/WPS 转换通道 | 本机未安装 Office/WPS 的可脚本化接口 | 在装有 Microsoft Word 或 WPS 的 Windows 机器上运行 `灵感.exe --check-converters`，确认通道被识别并用于生成 `.doc` |
| 7 | 磁盘写满（非只读）场景 | 需要构造小容量卷 | 用小型磁盘镜像验证生成失败时的错误提示与数据安全 |
| 8 | 中文输入法实际输入体验 | 需要人在桌面用输入法打字 | 在合同生成页用拼音/五笔输入长地址，确认候选框与文本正常 |
| 9 | 大模板（数十 MB、含大量图片）性能 | 未构造该规模语料 | 用真实大合同模板测试解析/生成耗时（当前 2000 段 + 200 行表格实测生成 0.19 秒） |

## 8. 结论

**技术方案、核心功能、三平台构建与发布、应用内更新（macOS）均已按任务书要求实现并通过可复现的验证**：
- 77 个测试用例 + 4 个基准全绿，`make lint`/`make ci` 覆盖格式、静态分析（含 Linux 视角）、三平台测试编译；
- CI 六 job 全绿，三平台产物均在目标系统上真实运行过；
- `v0.1.0` 正式发布成功（8 个资产），发布流程具备"构建失败即不发布"和 draft 双重保护；
- macOS 侧更新流程 9 项验收全部通过，含"更新后数据保留"。

**下列行为无法在当前环境验证，已如实标注且不视为通过**：Microsoft Word 打开结果、Windows 安装与更新、macOS Intel 真机、GUI 点击式更新、代码签名/公证、Word/WPS 转换通道、磁盘写满场景、中文输入法实操、超大模板性能。

按阶段 6 要求，以上未验证项需在具备对应环境时补验；补验完成后本报告第 7 节对应行可更新为"已验证"并附证据。

## 9. 交付后修复记录

### 9.1 macOS dmg 缺少安装别名（2026-10-16，v0.1.0 发布之后发现）

用户反馈双击 `inspiration_0.1.0_darwin_arm64.dmg`「只能打开，没有真正安装到电脑上」，核查结论与修复如下：

| 项 | 内容 |
|---|---|
| 现象 | 双击 dmg 后卷内只有一个 `灵感.app`，双击它只是从只读卷运行，没有安装到 `/Applications` |
| 根因 | 打包用 `hdiutil create -srcfolder build/bin/灵感.app`，卷里没有指向 `/Applications` 的符号链接（macOS 不会自动安装 `.app`） |
| 影响面 | `v0.1.0` 的 macOS dmg（arm64/amd64）；`.zip` 不受影响。从只读 dmg 内运行的应用也无法应用内更新（更新器需就地整包替换） |
| 修复 | 新增 `scripts/make-dmg.sh`：暂存目录放 `.app` + `Applications → /Applications` 别名，打完**挂载自检**（应用目录、可执行文件、Info.plist、签名、别名五项），不通过则删除产物并非 0 退出；`ci.yml` / `release.yml` 改用该脚本（CI 侧因此获得回归保护）；`Makefile` 增 `make dmg`，`make package-macos-*` 一并产 dmg；`docs/user-guide.md` §1 增加明确的安装步骤与"不要在 dmg 里直接双击运行"告警 |
| 验证证据 | `make dmg` → 挂载 `/tmp/mnt-make`：卷内为 `Applications`（`readlink` = `/Applications`）与 `灵感.app`；从卷内复制到本地目录后 `--version` = `v0.1.0`、`--selfcheck` exit 0；负例（不存在的应用包、缺 Info.plist）均 exit 1 且不产出 dmg |
| 未回填 | 已发布的 `v0.1.0` 资产保持原样；修复从下一个版本生效。详见 [`../phase4/README.md`](../phase4/README.md) §3.4 |

### 9.2 Windows 上传模板/生成合同时弹出控制台窗口（2026-10-16，用户反馈）

用户在 Windows 上反馈：**每次上传模板和生成合同时都会弹出一个命令行窗口执行脚本，执行完自动关闭**，"不懂技术的人看着会很吓人"。

| 项 | 内容 |
|---|---|
| 现象 | 上传模板、生成合同时出现"黑框一闪"或整个转换期间挂着命令行窗口 |
| 根因 | 应用是 GUI 子系统（自身无控制台）。Windows 启动控制台程序时若不在 `STARTUPINFO` 里指定隐藏窗口，系统会**为子进程新建控制台窗口**。代码里所有 `exec.Command` 都未做抑制：`converter` 的 `reg query`（通道探测，每次上传/生成都会跑，最多 4 次：Word.Application + KWPS + WPS + 复核）、`powershell -File convert.ps1`（`.doc` 转换与自检回读）、`app` 的 `powershell`（错误提示框）、`cmd /c start`（打开文件/文件夹） |
| 修复 | 新增 `internal/sysproc`（Windows 实现置 `HideWindow=true` 与 `CREATE_NO_WINDOW`，保留已有 `CreationFlags`；非 Windows 为空操作），并在上述全部启动点调用：`converter.regProgIDExists`、`converter.runCommand`、`app.openPath`、`app.revealPath`、`app.ShowStartupError` |
| 保留的例外 | 无。更新流程的批处理已改为无窗口执行（`sysproc.HideWindow` + 不再用 `start /min`，脚本内改用 `ping` 延时而非需要控制台的 `timeout`）；命令行的视觉效果待 Windows 真机复验 |
| 验证 | ✅ 本机：`GOOS=windows` 编译 + `go vet` + `staticcheck`（Linux/Windows 视角）全过；`make ci` 全绿。✅ CI：新增 windows job 步骤真实执行 `go test ./internal/sysproc/ -count=1 -v`（断言两个标志位已设置、不覆盖已有 CreationFlags、子进程仍能执行并回传输出） |
| 未验证 | 在真实 Windows 桌面上"看不到黑框"这一**视觉效果**：需要用户在有 Word/WPS 的 Windows 上上传一次 `.doc` 模板并生成一份 `.doc` 合同确认 |

### 9.3 v0.1.1 发布记录（2026-10-16）

`v0.1.1` 是把 §9.1（dmg 安装别名）与 §9.2（Windows 控制台窗口）两项修复交付给用户的补丁版本。

| 项 | 内容 |
|---|---|
| 版本提交 | `d166259`（`chore(release): 0.1.1`，`wails.json` → `0.1.1`），标签 `v0.1.1` |
| 发布流水线 | Release run #2（<https://github.com/sheep-ah/inspiration/actions/runs/38883614208>）：`validate` / `测试门禁` / `构建 darwin-arm64` / `构建 windows-amd64` / `构建 darwin-amd64` / `发布到 GitHub Releases` **全部成功** |
| 发布结果 | 正式（`draft=false`、`prerelease=false`），8 个资产：6 个安装包 + `checksums.txt` + `latest.json` |
| 校验和一致性 | 6 个资产的 GitHub asset digest 与 `checksums.txt` 逐项**完全一致** |
| 发布包实测（arm64 dmg） | 下载后 sha256 = `57804850…` 与 `checksums.txt` 一致；挂载后卷内为 `灵感.app` + `Applications → /Applications`（**§9.1 修复已进入发布包**）；从只读卷运行 `--version` = `v0.1.1`；`codesign --verify --deep --strict` 通过 |
| 发布包实测（arm64 zip，更新器使用的资产） | sha256 = `c0bf13c9…` 与 `checksums.txt` 一致；解压后应用 `--version` = `v0.1.1`、`--selfcheck` 退出码 0 |
| 更新检查（真实 API） | 用与 app 层相同的代码路径：`Check(v0.1.0)` → `发现新版本 v0.1.1`，选中 `inspiration_0.1.1_darwin_arm64.zip`（5,759,340 字节），资产 sha256 与发布 digest 一致；`latest.json` 的 6 项资产（os/arch/kind/size/sha256）与发布内容一致 |
| 仍未由本机完成 | GUI 界面上点一次「检查更新 → 下载并更新」（§7 第 4 项）：现在 `v0.1.0` → `v0.1.1` 已具备两次发布的条件，可由用户点击完成；Windows 安装/更新与黑框视觉确认（§7 第 2 项与 §9.2）仍需 Windows 真机 |

> 网络注记：本机访问 `github.com` 的下载地址（`/releases/download/...`）出现间歇性 `i/o timeout`，`api.github.com` 始终正常；同一时刻用 curl 重试可成功，说明是网络路径抖动而非发布产物问题。应用内下载使用同一地址，遇到类似抖动时界面会提示重试且不影响当前版本。

### 9.4 Windows 应用内更新静默失败（2026-10-16，用户反馈）

用户在 Windows 上点「检查更新 → 下载并更新」后：应用退出但**没有重启、也没有更新**，命令行窗口刷出
`'el' 不是内部或外部命令`、`'/nobreak' 不是内部或外部命令`、`'tart' 不是内部或外部命令`、
`文件名、目录名或卷标语法不正确。`、`找不到批处理文件。`，重新打开仍是 v0.1.0（当时最新为 v0.1.1）。

| 项 | 内容 |
|---|---|
| 现象 | 应用退出后更新完全没发生，只有一屏碎片报错；NSIS 安装与重启两步均未执行 |
| 根因 A | 生成的 `update.cmd` 是 **LF 行尾**（Go 字符串直接写文件）。`cmd.exe` 执行 `goto` 时按**字节偏移**重新定位脚本文件，LF-only 会让偏移算错——`goto wait` 之后从行中间继续执行，于是把 `del`、`start`、`timeout /t 1` 切成 `el`、`tart`、`/nobreak` 这类碎片命令；碎片里恰好执行到 `del "%~f0"`，脚本被提前删除，cmd 继续读就报"找不到批处理文件" |
| 根因 B | 脚本**内嵌了含中文的路径**（`%LOCALAPPDATA%\Programs\灵感\灵感.exe`、`%APPDATA%\灵感\update\...`），而 `cmd.exe` 按**控制台代码页**解码脚本字节：UTF-8 的中文被解成乱码，`if exist` / `copy` / `start` 全部指向不存在的路径。**即使 A 修好，这一层仍会让"安装包分支装完不重启、便携版分支根本没覆盖"** |
| 修复 A（行尾） | 抽出 `updater.windowsUpdateScript()`，脚本统一用 CRLF 拼接并写明原因；自删改用 `(goto) 2>nul & del "%~f0"` 惯用法（避免 cmd 在脚本消失后继续读取而报错）。`cat -A` 实测每行都是 `^M$` |
| 修复 B（路径） | 脚本不再内嵌任何路径，改用 `%INSPIRATION_UPDATE_PKG%` / `%INSPIRATION_UPDATE_EXE%`，由启动方通过 `cmd.Env` 传入（环境变量以 UTF-16 传递，不受脚本编码影响）→ **脚本成为纯 ASCII** |
| 修复 C | 等待延时 `timeout /t 1 /nobreak` → `ping -n 2 127.0.0.1 >nul`：`timeout.exe` 需要控制台输入句柄，而更新脚本以 `DETACHED_PROCESS`（无控制台）运行，`timeout` 本身就会失败 |
| 修复 D | 启动方式 `cmd /c start "" /min <script>` → 直接 `cmd /c <script>` + `sysproc.HideWindow`：更新全程不再出现命令行窗口（§9.2 那条"保留最小化窗口"的例外随之取消） |
| 修复 E（健壮性） | 等待循环加上限（`maxWaitRounds`，约 5 分钟）并把轮数参数化：PID 会被系统复用，无上限循环会让更新**永远**不发生；轮数非法时回落到默认上限（0 轮等于完全不等待） |
| 回归防护（全平台单测） | 行尾 CRLF/无裸 LF、**脚本不含任何非 ASCII 字节**、不使用 `timeout`、两个分支的命令行内容、自删在最后一行、轮数上限生效与非法值回落、变量缺失时安全结束 |
| 回归防护（Windows 实测） | `internal/updater` 的 Windows 用例真实执行生成的脚本：应用与更新包放在**含中文的目录**里（贴近生产）、脚本与标记文件放 ASCII 目录，路径全走环境变量；断言等待持有进程退出 → 覆盖/静默安装 → 重启（标记文件）→ 自删，且输出无碎片命令。**CI#25 的 windows job 已通过**（[run #25](https://github.com/sheep-ah/inspiration/actions/runs/38889217480) step 11） |
| 测试基础设施踩坑 | ① 无上限等待循环把 CI 的 windows job 拖到 11 分钟（#20）；② 用 `CombinedOutput` 抓脚本输出会**卡死在管道 EOF**——脚本里 `start` 拉起的子进程继承了管道句柄，外层 `cmd.exe` 退出甚至被 ctx kill 后管道仍不 EOF（#22 卡 20+ 分钟）→ 改为输出重定向到文件 + `WaitDelay` 兜底 + CI 侧 `-timeout` 上限；③ 本仓库 Actions 日志需管理权限（未登录 403），CI 步骤改为把失败输出打成 `::error:: ` 注解，运行页面即可看到详情（本次正是靠它定位到根因 B）；④ 用例"只等标记文件存在就读取"会偶发读到空串（cmd.exe 先创建重定向目标再写内容）——改为等到内容非空，并让该用例在 CI 跑 3 次以抓同类竞态（CI#33 的真实偶发失败） |
| 用户侧影响 | 修复只对**新版应用**生效：`v0.1.0`/`v0.1.1` 自带的更新器仍是坏的，无法自更新，需要手动安装一次修好的版本（v0.1.2 起），此后应用内更新才走新脚本 |
| 仍未验证 | Windows 真机上的完整界面流程（下载 → 脚本 → 静默安装 → 重启 → 数据保留）：需要在 Windows 上从修好的版本更新到再下一个版本 |

### 9.5 v0.1.2 发布记录（2026-10-16）

`v0.1.2` 把 §9.4（Windows 应用内更新失败的两个根因）交付给用户。**Windows 用户必须手动安装一次本版本**——`v0.1.0`/`v0.1.1` 自带的更新器本身就是坏的，无法自更新。

| 项 | 内容 |
|---|---|
| 版本提交 | `2383867`（`chore(release): 0.1.2`，`wails.json` → `0.1.2`），标签 `v0.1.2` |
| 发布流水线 | Release run #3（<https://github.com/sheep-ah/inspiration/actions/runs/38889617710>）：`validate` / `测试门禁` / 三平台构建 / `publish` **全部成功** |
| 发布结果 | 正式（`draft=false`、`prerelease=false`），8 个资产 |
| 校验和一致性 | `inspiration_0.1.2_darwin_arm64.dmg` 与 `.zip` 的本地 sha256 与 `checksums.txt` **逐字节一致**（`8fa60ea8…` / `3cf91bef…`） |
| 发布包实测（arm64 dmg） | 挂载后卷内为 `灵感.app` + `Applications → /Applications`（§9.1 修复保持有效）；从只读卷运行 `--version` = `v0.1.2`；`codesign --verify --deep --strict` 通过 |
| 发布包实测（arm64 zip，更新器使用的资产） | 解压后应用 `--version` = `v0.1.2`、`--selfcheck` 退出码 0（数据目录正常迁移） |
| 元数据 | `latest.json` 的 6 项资产（os/arch/kind/size/sha256）与发布内容一致 |
| Windows 侧证据 | CI#25 的 windows job 真实执行了 v0.1.2 里的更新脚本（等待 → 覆盖/静默安装 → 重启 → 自删，含中文目录），见 §9.4 |
| 仍未验证 | Windows 真机完整界面流程（下载 → 脚本 → 静默安装 → 重启 → 数据保留）：目标版本 `v0.1.3` 已发布（见 §9.6），更新预检已确认 Windows 客户端会选到正确的安装包；待用户在 Windows 上从 v0.1.2 实际点一次更新 |

### 9.6 v0.1.3 发布记录（2026-10-16，验证用版本）

`v0.1.3` 是**版本递增版**：代码与 `v0.1.2` 完全一致，用途是给"从旧版更新到新版"提供一个可更新的目标版本，
让 Windows 真机上的更新链路可以闭环验证（`v0.1.0`/`v0.1.1` 的更新器是坏的，必须先手动装一次 `v0.1.2` 起的新版本）。

| 项 | 内容 |
|---|---|
| 版本提交 | `47bee12`（`chore(release): 0.1.3`，`wails.json` → `0.1.3`），标签 `v0.1.3` |
| 发布流水线 | Release run #4（<https://github.com/sheep-ah/inspiration/actions/runs/38895465044>）：`validate` / `测试门禁` / 三平台构建 / `publish` **全部成功**，正式发布 8 个资产 |
| 校验和一致性 | arm64 的 dmg/zip 本地 sha256 与 `checksums.txt` 一致（`def25929…` / `3d72c900…`） |
| 发布包实测 | dmg 挂载含 `Applications → /Applications`，应用 `--version` = `v0.1.3`、`codesign --verify` 通过；zip 解压后 `--version` = `v0.1.3`、`--selfcheck` 退出码 0；`latest.json` 六项资产正确 |
| **更新预检（Windows/amd64）** | 用 app 层同一套 updater 代码模拟：`Check(v0.1.2)` → `发现新版本 v0.1.3`，选中 `inspiration_0.1.3_windows_amd64_setup.exe`（9,971,527 字节），sha256 与 `checksums.txt` 一致 → Windows 用户点检查更新会拿到正确的安装包 |
| 说明 | 自动生成的发布说明按"上一标签以来的提交"生成，本版只有文档提交，故更新说明很薄；本版用途写在标签注释里 |
| 待验证 | Windows 真机点击流程（v0.1.2 → v0.1.3）：下载 → 脚本 → 静默安装 → 重启 → 数据保留 |


### 9.7 Windows「打开所在文件夹」点击无反应（2026-10-16，用户反馈）

用户在 Windows 上生成合同后点「打开所在文件夹」没有任何反应。

| 项 | 内容 |
|---|---|
| 现象 | 点击后既不报错也不弹窗，Finder/资源管理器无动静（错误提示也不会出现，因为命令本身启动成功了） |
| 根因（**§9.2 修复引入的回归**） | §9.2 为消除控制台黑框，给所有子进程都调用了 `sysproc.HideWindow`（`STARTF_USESHOWWINDOW` + `SW_HIDE`）。但 `explorer.exe` 是 **GUI 程序**：给启动器设 SW_HIDE 会把**它要打开的那个窗口**一起隐藏——于是"启动成功但窗口不可见"，表现为点了没反应 |
| 附带问题 | 当时认为"`/select,` 后的路径要加引号"，于是把参数写成 `/select,"<path>"`（字面引号）——**这引入了第二个问题**：explorer 解析的是 Windows 分词**之后**的参数，文档里那对引号是给分词器的（explorer 实际拿到 `/select,C:\path`），字面引号让它拿到 `/select,"C:\path"`，路径非法 → 只把资源管理器开在默认位置。用户复验后反馈"窗口能开，但不是保存位置" |
| 第二轮修复（v0.1.6） | 参数改为 `/select,<path>`（**不带字面引号**）：含空格的路径由 exec 的命令行转义自动加引号，分词后同样是干净路径。单测锁定"参数里不得出现字面引号"，另有 Windows 侧用例用 `syscall.EscapeArg` 钉住转义结果（只有包裹引号、没有 `\"`） |
| 修复 | 1) `openCommand` 增加 `hideConsole` 语义：只有**控制台程序**（`cmd`/`powershell`/`reg`）才隐藏窗口，`explorer` 明确不隐藏（并在注释里写明原因）；2) `/select,"<path>"` 改为带引号的标准写法；3) 定位失败时兜底为"打开所在目录"，避免再次出现"点了完全没反应" |
| 结构调整 | 命令构造抽成纯函数 `openCommandFor`/`revealCommandFor`（`internal/app/open.go`），Windows 的引号与"是否隐藏"两条约束因此可以在**任意平台单测**，不必等真机 |
| 回归防护 | `internal/app/open_test.go`：Windows 定位文件必须 `explorer /select,"<path>"` 且 `hideConsole=false`；含空格路径必须带引号；Windows 打开文件必须 `cmd /c start "" <path>` 且 `hideConsole=true`；darwin/linux 的命令逐项断言 |
| 仍未验证 | Windows 真机上的视觉效果（点「打开所在文件夹」应弹出资源管理器并选中该文件）——`explorer` 无法在本机或 CI 无交互会话中验证 |
| 版本 | 第一轮修复随 **v0.1.4** 发布；第二轮（去掉字面引号）随 **v0.1.6** 发布（见 §9.10） |

### 9.8 v0.1.4 发布记录（2026-10-16）

`v0.1.4` 交付 §9.7（Windows「打开所在文件夹」无反应），也是当前应用内更新的目标版本（取代 `v0.1.3`，后者只是版本递增的验证版）。

| 项 | 内容 |
|---|---|
| 版本提交 | `427bb34`（`chore(release): 0.1.4`，`wails.json` → `0.1.4`），标签 `v0.1.4` |
| 发布流水线 | Release run #5（<https://github.com/sheep-ah/inspiration/actions/runs/38897336977>）：`validate` / `测试门禁` / 三平台构建 / `publish` **全部成功**，正式发布 8 个资产 |
| 校验和一致性 | arm64 的 dmg/zip 本地 sha256 与 `checksums.txt` 一致（`f5679133…` / `4444b421…`） |
| 发布包实测 | dmg 挂载含 `Applications → /Applications`，应用 `--version` = `v0.1.4`、`codesign --verify --deep --strict` 通过；zip 解压后 `--version` = `v0.1.4`、`--selfcheck` 退出码 0；`latest.json` 六项资产正确（Windows 安装包 sha256 `97da381a…`） |
| 回归防护 | `internal/app/open_test.go` 覆盖 windows/darwin/linux 三条路径的命令构造（含"explorer 不得隐藏窗口"与"`/select` 路径必须带引号"） |
| 待用户复验 | Windows 上点「打开所在文件夹」应弹出资源管理器并选中该文件；「打开文件」按钮也应正常（若两者之一仍无反应，按 §9.7 的表抓证据） |

### 9.9 v0.1.5 发布记录：更新下载超时（2026-10-16）

用户提问"下载并更新的超时配的是多久"，核查时发现配置本身不合理——这是本版修复的内容。

| 项 | 内容 |
|---|---|
| 问题 | `updater.New()` 用了 `http.Client{Timeout: 60 * time.Second}`。Go 的这个 Timeout 覆盖"建连 + 重定向 + **读完整个响应体**"，因此**整包必须在 60 秒内下完**：Windows 安装包 9.97 MB → 要求 ≥166 KB/s，便携版 17.7 MB → ≥296 KB/s，macOS arm64 zip 5.76 MB → ≥96 KB/s。低于该带宽即报"下载失败（请检查网络后重试）：context deadline exceeded"。应用层虽给了 30 分钟 context（`DownloadUpdate`），但被这个 60 秒上限先掐死；且当时没有停滞检测，一旦挪掉上限，卡死的连接会挂满 30 分钟 |
| 修复 | ① 不再设 `http.Client.Timeout`，传输时长交给调用方 context（应用层 30 分钟）；② Transport 只约束建连与首字节：Dial 30s / TLS 15s / `ResponseHeaderTimeout` 60s；③ 新增停滞检测 `downloadStallTimeout` = **90 秒无任何数据**才中止并提示可重试（"慢但一直在走"不杀） |
| 实现踩坑 | 可取消的 ctx 必须在构造 `http.Request` **之前**派生，否则 `cancel()` 影响不到正在读的响应体（第一版就挂在 `persistConn.Read`，测试超时暴露） |
| 回归防护 | `internal/updater/timeout_test.go`：断言 Client 无整体 Timeout 但设置了建连/首字节/停滞超时；停滞下载在 stall 超时内中止、给出"停滞"原因、且删除半成品文件；慢但持续有数据的下载必须成功并通过 sha256 校验 |
| 文档 | `docs/development.md` §3.4 新增"下载超时策略"表；跨平台陷阱新增第 11 条（不要给大文件下载设 Client.Timeout + ctx 派生顺序） |
| 版本提交 / 标签 | `c45e658`（`chore(release): 0.1.5`），标签 `v0.1.5` |
| 发布流水线 | Release run #6（<https://github.com/sheep-ah/inspiration/actions/runs/38899337856>）：`validate` / `测试门禁` / 三平台构建 / `publish` **全部成功**，正式发布 8 个资产 |
| 发布包实测 | dmg 与 zip 的本地 sha256 与 `checksums.txt` 一致（`aafecf84…` / `b4146e39…`）；dmg 挂载含 `Applications → /Applications` 且 `--version` = `v0.1.5`、`codesign --verify` 通过；zip 解压后 `--version` = `v0.1.5`、`--selfcheck` 退出码 0；`latest.json` 六项资产正确 |
| 待用户复验 | Windows 上"下载并更新"在较慢网络下应能完整下载（不再 60 秒掐断）；停滞时提示"下载停滞超过 90 秒…可重试" |

本章（§9）与 §7 的关系：§7 是"当前环境无法验证"的清单，§9 是"已发布产物上发现的缺陷及其修复"与"发布记录"，两者独立。
