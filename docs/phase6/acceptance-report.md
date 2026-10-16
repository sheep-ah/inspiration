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

语义化版本：`MAJOR` 不兼容变更 / `MINOR` 向后兼容新功能 / `PATCH` 修复。当前 `v0.1.0`。
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

本节与 §7 的关系：§7 是"当前环境无法验证"的清单，本节是"已发布产物上发现的缺陷及其修复"，两者独立。
