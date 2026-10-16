# 灵感（Inspiration）开发文档

## 1. 技术栈

| 层 | 选型 | 版本 | 选型依据 |
|---|---|---|---|
| 语言 | Go | 1.26 | 任务书要求；单二进制、跨平台交叉编译、标准库自带 `archive/zip` 与 `encoding/xml` |
| GUI | [Wails](https://wails.io) | v2.16.0 | 见 [`docs/phase1/gui-evaluation.md`](phase1/gui-evaluation.md)：中文输入依赖系统输入法、产物体积小、安装器与 `.app` 更新路径成熟（Fyne 因 CJK 输入法缺陷被否决） |
| 前端 | 原生 TypeScript + Vite | TypeScript 5.x / Vite 7 | 页面只有三页，不引入框架；产物 25 KB |
| 数据库 | [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) | v1.60.1 | 纯 Go（`CGO_ENABLED=0`），三平台交叉编译无需 C 工具链；开启 WAL |
| DOCX 引擎 | 自研（`internal/docx`） | — | 三个第三方候选库实测全部不合格（无 API / panic / 跨 run 全部漏替换），见 [`docs/phase1/docx-evaluation.md`](phase1/docx-evaluation.md) |
| `.doc` 转换 | 多通道调用本机 Office/WPS/LibreOffice | — | 见 [`docs/doc-support-evaluation.md`](doc-support-evaluation.md) |

## 2. 架构

```
main.go                  CLI 入口（--version/--fields/--inspect/--selfcheck/--check-converters…）+ Wails 启动
internal/
  app/                   Wails 绑定层（前端唯一入口）+ 启动失败弹窗
  config/                数据目录与子目录（templates/contracts/logs/update）
  logging/               slog 日志（文件 + 轮转；字段值脱敏）
  id/                    ID 生成
  docx/                  OOXML 引擎：解析、占位符扫描、字节级替换
  store/                 SQLite（模板/合同/设置）+ 原子文件写入 + 文件名清洗 + 软删除
  template/              模板服务：导入（含 .doc→.docx）、校验、搜索、重命名、删除
  contract/              字段定义与校验、生成编排、输出路径决策
  converter/             .doc/.docx 转换：Word COM/AppleScript → WPS COM → LibreOffice + 产物强校验
  updater/               应用内更新：检查、下载、校验、按平台安装
  sysproc/               子进程平台差异：Windows 上抑制控制台窗口（HideWindow + CREATE_NO_WINDOW）
  version/               版本信息（ldflags 注入）
  e2e/                   端到端测试（真实语料 + 真实数据库 + 重启/异常场景）
frontend/
  src/views/             三个页面：templates / generate / about
  src/api.ts             前端到 Wails 绑定的类型化封装
  src/ui.ts              DOM 辅助（元素构造、提示、格式化）
  dist/                  构建产物（提交入库：//go:embed 需要，新克隆无需 Node 即可编译）
build/                   Wails 打包资源（图标、Info.plist、NSIS 模板）
scripts/                 版本注入、图标生成、批量模板转换
docs/                    阶段 1–6 文档与证据
poc/docx-eval/           阶段 1 的引擎评估实验（保留原始数据与校验脚本）
```

**依赖方向**：`app → template/contract → docx/store/converter/updater → config/logging/id/version`。
引擎层（`docx`）不依赖任何其他业务包，便于独立测试与复用。

## 3. 核心模块

### 3.1 OOXML 引擎（`internal/docx`）

- **零依赖**：按 OOXML 规范扫描包内部件，不做全量反序列化
- **部件覆盖**：`document.xml`、`header*.xml`、`footer*.xml`、`footnotes.xml`、`endnotes.xml`，以及文本框、内容控件内的 `w:t`
- **跨 run 替换**：Word 常把一个 `{{占位符}}` 拆到多个 `w:r/w:t`。引擎按段落聚合文本、定位占位符的**起止 run 与文本偏移**，再把新值写回起始位置并清空后续片段，保留首个 run 的格式
- **字节级写入**：只重写被替换的 `w:t` 内容，其余字节（含 `[Content_Types].xml`、`_rels`、样式、图片）原样复制，最大限度保证版式与兼容性
- **拒绝而非猜测**：占位符未闭合 / 被 `w:br`、`w:tab`、域代码打断 / CDATA 内容时返回错误，不做静默处理
- **性能**：大文档（2000 段 + 200 行表格）解析 10 ms、生成 0.19 s（M1）

### 3.2 存储（`internal/store`）

- SQLite（WAL）保存模板元信息、占位符清单、生成记录、设置项
- **原子写入**：先写 `*.tmp` → `fsync` → `os.Rename`，避免半成品文件
- **文件名清洗**：按**字节**截断（`MaxNameBytes = 160`，UTF-8 边界安全）——中文一个字符 3 字节，早期按字符截断会在 Linux 上触发 255 字节文件名上限
- **不覆盖**：`UniquePath` 追加 `(2)`/`(3)`…；软删除模板（保留已生成合同的可追溯性）
- **字段值不落盘**：合同内容属于敏感信息，数据库只记录"用了哪些字段"，不存字段值

### 3.3 转换器（`internal/converter`）

按**保真度**排序尝试，并在每次转换后做**产物强校验**：

| 通道 | 调用方式 | 保真度 |
|---|---|---|
| Microsoft Word | Windows COM / macOS AppleScript | 最高 |
| WPS | Windows COM | 高 |
| LibreOffice | `--headless --convert-to doc` | 中（表格宽度会写成 100%，实测差异） |

强校验：产物必须符合 **OLE2 复合文档头**（`d0cf11e0a1b11ae1`），并回读文本比对关键字段值；不满足则视为失败，绝不产出"改名假 `.doc`"。默认同时保留 `.docx` 备份。

可用性探测可通过 `灵感 --check-converters`（列通道）与 `灵感 --selfcheck --run-converter-check`（实跑一次）确认。

### 3.4 更新器（`internal/updater`）

安全约束（详见 [`docs/phase5/README.md`](phase5/README.md)）：

- 只读 `https://api.github.com/repos/sheep-ah/inspiration/releases/latest`；拒绝 draft/prerelease
- 下载来源白名单：**仅 HTTPS + `github.com` / `objects.githubusercontent.com` / `release-assets.githubusercontent.com`**
- 强制 SHA256 校验（来自 Release 资产 digest 或 `checksums.txt`），失败即删文件并拒绝安装
- 安装前做结构校验（macOS：`Info.plist` + `Contents/MacOS` 内可执行文件）；替换前备份，失败回滚
- **不触碰数据目录**；更新只替换应用本体

## 4. 本地开发

### 4.1 环境要求

- Go 1.26+、Node 20+、Wails CLI（`go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0`）
- 打包 Windows 安装包需要 `mingw-w64` 与 NSIS；生成 `.doc` 需要本机有 Word / WPS / LibreOffice
- 可选：`staticcheck`、`actionlint`（用于本地复现 CI 检查）

### 4.2 常用命令

| 命令 | 用途 |
|---|---|
| `make dev` | 开发模式（前端热重载） |
| `make build` | 构建当前平台，自动注入版本号（`git describe`） |
| `make test` / `make test-race` | 单元 + 端到端 / 竞态检测 |
| `make lint` | gofmt + go vet + staticcheck + 前端类型检查 |
| **`make ci`** | **推送前跑一次：本地完整复现 CI 的全部检查（推荐）** |
| `make selfcheck` | 构建后无界面自检（初始化数据目录与数据库） |
| `make dmg` | 打当前平台的 dmg（内含「应用程序」拖拽别名，挂载自检通过才产出） |
| `make package-macos-arm64` / `-amd64` | macOS 打包：`.app` + dmg（同样走 `scripts/make-dmg.sh` 自检） |
| `make package-windows` | 打 Windows NSIS 安装包 |

### 4.3 命令行参数（排障用）

```
--version                 版本信息（含提交号与构建时间）
--fields                  打印当前版本支持的 11 个占位符字段
--inspect <file>          扫描模板并输出占位符、来源部件与校验结果（JSON）
--check-converters        列出本机可用的 .doc 转换通道
--run-converter-check     实际执行一次 .docx → .doc 转换验证通道
--selfcheck               初始化数据目录与数据库后退出（CI 冒烟测试用）
--data-dir <dir>          覆盖数据目录
```

环境变量：`INSPIRATION_DATA_DIR`（数据目录）、`INSPIRATION_SOFFICE`（LibreOffice 路径）、`INSPIRATION_CONVERTER`（强制通道：`word`/`wps`/`libreoffice`）。

## 5. 测试策略

| 层次 | 位置 | 特点 |
|---|---|---|
| 引擎单测 | `internal/docx` | 覆盖跨 run、空文本节点、多部件、嵌套表格、异常输入；夹具在 `testdata/`（python-docx 生成的真实 Word 包） |
| 存储单测 | `internal/store` | 含"中文长文件名在 Linux 上可创建"等平台边界用例 |
| 端到端 | `internal/e2e` | 真实语料 + 真实数据库 + 重启、异常退出、只读目录、损坏数据库、超长文件名 |
| 更新器 | `internal/updater` | 全部离线（`httptest` 假 GitHub）；含安装失败不破坏当前版本、备份/回滚 |
| 平台差异 | `internal/sysproc`（仅 Windows 编译） | 断言 `HideWindow` 置上 `SW_HIDE` + `CREATE_NO_WINDOW`、不覆盖已有 CreationFlags，且子进程仍能执行并回传输出；由 CI 的 windows job 真实执行 |
| 性能 | `internal/docx/perf_test.go` | 2000 段 + 200 行表格；`//go:build race` 下只记录不判定（竞态插桩会放大耗时） |

**跨平台陷阱（CI 抓到的真实问题，务必注意）**：

1. **文件名按字节而非字符限制**（Linux 255 字节；中文 3 字节/字）
2. **平台条件编译**：只在 `_darwin.go` 里用到的函数，在 Linux 视角下会被 staticcheck 判为未使用；测试若引用平台特有符号会在其他平台**编译失败**（`make ci` 会检查三平台测试编译）
3. **绝对时间断言**：不要在 `-race` 或共享 runner 上使用紧阈值
4. **Windows 控制台编码**：Python 打印非 ASCII 会 `UnicodeEncodeError`（cp1252）；PowerShell 脚本不要写非 ASCII 路径
5. **GUI 子系统 exe**：PowerShell 用 `&` 调用不会等待、`$LASTEXITCODE` 可能为空，必须 `Start-Process -Wait -PassThru`
6. **GitHub Actions 步骤退出码**取最后一条命令的结果：不要以可能失败的命令结尾（如 `where.exe` 找不到就返回 1）
7. **macOS dmg 必须带安装别名**：`hdiutil create -srcfolder App.app` 打出的 dmg 根目录只有一个 `.app`，没有指向 `/Applications` 的符号链接，用户双击只是"从只读卷运行"，看起来像"打开了但没安装"。统一用 `scripts/make-dmg.sh`（暂存目录放 `.app` + `Applications` 别名，打完挂载自检，缺别名就失败且不产出 dmg）
8. **shell 里 `$VAR` 紧邻中文字符**：`"$OUT_DMG（"` 会被解析成变量名 `OUT_DMG（`，`set -u` 下报 unbound variable。中文相邻的变量一律写 `${VAR}`
9. **Windows 上子进程会弹控制台窗口**：应用是 GUI 子系统（自身无控制台），启动控制台程序（`cmd`/`powershell`/`reg`）时 Windows 会**为它新建一个控制台窗口**。用户看到的就是"上传模板/生成合同时弹出命令行执行脚本"。所有 `exec.Command*` 之后必须调用 `sysproc.HideWindow(cmd)`（`internal/sysproc`，非 Windows 平台为空操作），它会置 `HideWindow`（SW_HIDE）并加 `CREATE_NO_WINDOW`。更新流程的批处理同样走这条路径（脚本内用 `ping` 延时，不依赖控制台，因此不需要"最小化窗口"这种折中）
10. **Windows 批处理脚本必须用 CRLF，且不要内嵌中文路径**：`cmd.exe` 执行 `goto` 时按**字节偏移**重新定位脚本文件，LF-only 会让偏移算错、`goto` 之后从行中间继续执行——现场表现为 `'el'`/`'tart'`/`'/nobreak'` 这类"碎片命令"、更新静默失败。另外 `cmd.exe` 按**控制台代码页**解码脚本字节，脚本里写 UTF-8 的中文路径（安装目录/数据目录都含「灵感」）会被解成乱码，`if exist`/`copy`/`start` 指向不存在的路径（表现为"装完不重启/根本没覆盖"）。因此：生成 `.cmd` 一律走 `updater.windowsUpdateScript()`（CRLF + **纯 ASCII**，路径经 `%INSPIRATION_UPDATE_PKG%`/`%INSPIRATION_UPDATE_EXE%` 环境变量传入，环境变量是 UTF-16、不受编码影响），单测锁定"无裸 LF"与"无非 ASCII 字节"。同理：无控制台场景不要用 `timeout.exe`（需要控制台输入句柄），改用 `ping -n N 127.0.0.1 >nul` 延时；自删用 `(goto) 2>nul & del "%~f0"`。Windows 侧还有真实执行脚本的用例（CI 的 windows job 跑），详见 `docs/phase6/acceptance-report.md` §9.4

## 6. CI/CD

| 工作流 | 触发 | 内容 |
|---|---|---|
| `.github/workflows/ci.yml` | `main` 推送 / PR | `lint`（gofmt+vet+staticcheck+前端类型检查+前端产物一致性）、`test`（单元+端到端+race+基准）、`build` 三平台矩阵（含产物真实运行校验）、`verify`（六个产物 + 体积 + SHA256SUMS + Job Summary） |
| `.github/workflows/release.yml` | `v*` 标签 | `validate`（标签格式 + 提交在 main + 版本号一致性）→ `test` 门禁 → `build` 三平台 → `publish`（生成 `checksums.txt` 与 `latest.json`、先建 draft、校验 8 个资产、转正式发布） |

**发布流程**：

```bash
python3 scripts/set-version.py 0.2.0     # 写入 wails.json
git commit -am "chore(release): 0.2.0"
git tag -a v0.2.0 -m "灵感 v0.2.0"
git push origin main && git push origin v0.2.0
```

发布失败时 `publish` 不会执行，draft 也不会转正；应用内更新只读 `releases/latest`，因此用户不会收到半成品。

## 7. 如何扩展

### 7.1 新增一个字段

1. `internal/contract/contract.go`：在 `Fields` 中加定义（键名、占位符、标签、是否必填、校验规则）
2. `internal/docx`：如新字段需要特殊处理才需改动（一般不需要）；若加入新部件类型，在 `engine.go` 的部件清单里补充
3. 测试：`internal/contract/contract_test.go`（校验用例）、`internal/e2e`（真实语料替换次数断言）
4. 文档：`docs/user-guide.md` 的字段表 + `docs/phase6/acceptance-report.md` 的字段数量说明

> 字段值的处理**必须按文本**（`w:t`），避免 Excel 式数值转换导致前导零丢失。

### 7.2 新增一个平台

1. `wails.json` 与 `.github/workflows/*.yml` 的构建矩阵加平台
2. `internal/updater`：加 `install_<goos>.go`（`applyPlatform`）与 `pickAsset` 规则；若无法自我更新，返回"引导用户手动下载"而不是静默失败
3. `internal/config`：确认数据目录约定（`os.UserConfigDir` 一般已覆盖）
4. 文档与验收报告如实标注未验证项

### 7.3 升级 Wails 或换框架

**不要在没有重新做输入法验证的情况下更换 GUI 框架**：中文输入体验是本项目的框架否决级指标（见 `docs/phase1/gui-evaluation.md`）。升级 Wails 后必须重跑：三平台构建、安装包生成、`.app` 整包替换更新、中文输入与长文本粘贴。

## 8. 编码约定

- **错误文案面向用户**：中文、说明原因与下一步动作（例："模板中未找到任何占位符 `{{字段名}}`，请在模板中添加后重新上传"）
- **日志脱敏**：绝不记录字段值（银行账号、税号、电话）、合同正文；只记文件名、模板名、字段名清单、耗时、结果
- **原子写**：所有落盘（模板副本、合同、数据库）先写临时文件再重命名
- **不静默覆盖**：任何写入已存在路径的场景都要显式决策（追加序号或明确报错）
- **拒绝优于猜测**：能确认的才处理，不能确认的必须报错并说明原因（占位符、`.doc` 产物、更新包都遵循此原则）
- **提交规范**：`feat|fix|docs|chore|ci(scope): 摘要`，正文写"为什么"与验证方式；提交前跑 `make ci`
