# GUI 框架选型：Wails 与 Fyne 对比（阶段 1 交付物）

> 所有结论均来自本机实测（2026-10，macOS 27.0.1 / Apple M1 / Go 1.26.1）与官方仓库 issue、官方文档，不做未经验证的推断。

## 0. 选型结论

**唯一 GUI 框架：Wails v2.16.0，前端模板 `vanilla-ts`（TypeScript + Vite，不引入运行时 UI 框架）。**

- 决定项：中文/长文本输入能力、安装包与更新路径、包体积。
- 次决定项：布局与响应式能力、构建复杂度。
- **不采用 Fyne v2.8.2**，关键原因是官方仓库中仍未解决的 CJK 输入法缺陷（详见 §3.4），以及 cgo/OpenGL 依赖带来的分发与兼容成本。

## 1. 验证环境（实测）

| 项目 | 值 |
|---|---|
| 操作系统 | macOS 27.0.1（Darwin 27.0.0，Apple M1） |
| Go | go1.26.1 darwin/arm64 |
| Node / npm | v24.3.0 / 11.4.2 |
| Xcode | Command Line Tools 2418（未安装完整 Xcode） |
| Wails CLI | v2.16.0（`go install github.com/wailsapp/wails/v2/cmd/wails@latest`） |
| Fyne CLI / 库 | tools v1.7.3 / `fyne.io/fyne/v2` v2.8.2 |
| 交叉编译工具链 | mingw-w64 14.0.0_3（Homebrew，1.4 GB）+ NSIS 3.13（makensis） |
| 渲染验证工具 | LibreOffice 26.8.1（headless）+ pypdf 6.20.1 |

## 2. 十项要求对比

| # | 维度 | Wails v2.16.0（实测） | Fyne v2.8.2（实测） |
|---|---|---|---|
| 1 | 三平台支持 | `wails build` → darwin/arm64 ✅；`-platform darwin/amd64` ✅；`-platform darwin/universal` ✅；`-platform windows/amd64` ✅（本机交叉编译）。Linux 未验证（不在目标平台） | 同一份最小程序：darwin/arm64 ✅（cgo）、darwin/amd64 ✅（cgo+clang）、windows/amd64 ✅（cgo+mingw）。三平台均需 cgo |
| 2 | 构建/安装/分发 | 单命令产出 `.app` / `.exe`；`-nsis` 直接产出 NSIS 安装包（6.1 MB）；`-webview2 download\|embed\|browser\|error` 处理 Windows 运行时依赖。实测耗时：arm64 33.8s（含首次 npm install）、windows/amd64 12.0s、universal 4.2s（增量） | `go build` 产出裸可执行文件；无安装包生成能力，需自行接 NSIS 或使用 `fyne-cross`（Docker）。交叉编译必须准备目标平台 C 工具链 |
| 3 | 组件/布局/响应式 | HTML/CSS：flex/grid/媒体查询，长表单、列表、对话框按 Web 常规实现；窗口缩放自适应成本低 | 自带 widget（Entry/Form/List/Table/Dialog…）；布局由 `container` 组合决定，窄窗口需要显式的布局分支（`container.NewGridWrap` 可自动换行，但表单换行/错误提示需逐处处理） |
| 4 | 中文输入/长文本/文件选择 | 文本输入使用系统 WebView 的原生输入控件，IME 由系统实现；文件选择用 Wails 原生对话框 API | **存在多个仍开放的 CJK 输入缺陷**：候选框位置错乱（fx#4546）、Windows 下无法输入中文（fx#4544、fx#5818）、候选框出现在应用外（fx#618）；默认字体一度不支持中文 |
| 5 | Go ↔ UI 交互 | Go 方法绑定为 JS 函数（Promise）+ 事件；跨语言边界清晰，业务逻辑仍在 Go | 纯 Go 调用，无跨语言边界（本项 Fyne 更简单） |
| 6 | 依赖/维护/许可 | Wails MIT；前端仅 Vite + TypeScript（运行时零框架依赖） | Fyne MIT；依赖 glfw/gl/字体解析等 cgo 库（间接依赖较多） |
| 7 | 包体积/启动/资源 | arm64 `.app` 7.8 MB；universal `.app` 16 MB；windows `.exe` 11.4 MB；NSIS 安装包 6.1 MB（WebView 由系统提供） | arm64 31.7 MB；amd64 33.3 MB；windows `.exe` 43.8 MB（OpenGL/字体等静态链接） |
| 8 | 图标/原生体验 | `build/appicon.png` + `build/darwin/Info.plist` + Windows manifest/NSIS 模板；原生菜单、托盘、对话框、主题 | `fyne package` 可嵌入图标、支持托盘与原生对话框；窗口与控件自绘，观感与系统控件有差异 |
| 9 | GitHub Actions 复杂度 | 需要 Node + Wails CLI；Windows 打包需 NSIS；macOS 需 Xcode CLT。官方 runner 可直接覆盖（windows-latest / macos-14 / macos-15） | 需要 cgo 工具链：macOS 用 CLT，Windows 需要 mingw-w64；或引入 fyne-cross(Docker) |
| 10 | 应用自动更新 | 无官方更新器，但产物形态利于更新：macOS 是标准 `.app` 整包，Windows 有官方 NSIS 安装器（支持 `/S` 静默、`-nsisscope user`） | 无官方更新器；默认产物是裸 exe，更新/卸载路径需自建安装器 |

## 3. 判断标准落地（对应任务书 §2.2）

### 3.1 “现代、灵活的界面布局 + 可接受前端技术栈” → 本项目成立

本项目界面是 3 个页面：合同生成（10 个字段的长表单 + 模板模糊搜索下拉）、模板管理（列表 + 搜索 + 详情 + 删除确认）、设置/更新（进度与提示）。
长表单的换行、字段错误提示、窄窗口适配在 HTML/CSS 中是常规工作；在 Fyne 中需要为每个容器写 Resize 逻辑。

### 3.2 “Go 原生开发体验、减少独立前端工程” → 本项 Fyne 更优，但不足以抵消其他代价

前端工程成本可控：采用 `vanilla-ts`（无 React/Vue），构建产物是一个 `dist/` 目录，由 Wails 打进二进制，不产生运行时部署负担。

### 3.3 交叉编译不能等同于平台验证

两者在本机都只是**交叉编译成功**。任务书要求“不能仅设置 GOOS/GOARCH 就认定跨平台构建成功”——因此阶段 4 的 CI 将在对应操作系统的 runner 上原生构建（windows-latest / macos-14 / macos-15），本机交叉编译只用于本地快速验证。真实运行验证仍需目标机器（见 `risks-and-unverified.md`）。

### 3.4 决定性证据：CJK 输入法

Fyne 官方仓库仍开放的 issue（2026-10 检索）：

- `fyne-io/fyne#4546` Windows 下中文输入法候选框固定在屏幕左上角，无法跟随输入位置。
- `fyne-io/fyne#4544` 表单无法输入中文（默认字体亦不支持中文，需自定义主题/FYNE_FONT）。
- `fyne-io/fyne#5818` web/Entry 组件无法输入中文，根因是底层 glfw 的 IME 支持缺失（引用 glfw PR#2130）。
- `fyne-io/fyne#618` 输入法状态框出现在应用之外（macOS/Windows 均报告）。

任务书 §3.1 要求 10 个中文必填字段，§4 阶段 3 明确要求“长文本和中文输入测试”。该缺陷直接命中必测项，属于选型否决级别风险。

### 3.5 补充证据：安全更新形态

Apple 官方文档明确：macOS 上整个 `.app` 已被签名，替换包内二进制会破坏签名（Apple Developer Forums #781548、TN2206）。Wails 产出标准 `.app` 包，更新策略是“整包替换 + 签名校验”，路径清晰；Fyne 裸 exe 需要自行组装 `.app` 结构才能有同样的更新语义。

## 4. 冻结决定与约束

| 项目 | 冻结值 |
|---|---|
| GUI 框架 | Wails **v2.16.0**（v2 稳定线） |
| 前端 | `vanilla-ts` 模板（TypeScript + Vite），**不引入** React/Vue/Svelte 等运行时框架 |
| Node 版本（构建机/CI） | ≥ 20（本机 24.3.0 实测通过） |
| 运行时依赖 | Windows：WebView2 Runtime（安装包内置 download/embed 策略）；macOS：系统 WKWebView |
| 最低系统版本 | 待阶段 4 在 `build/darwin/Info.plist` / NSIS 脚本中显式声明并验证（**当前未验证**） |
| 变更规则 | 未经明确批准不得更换框架；Wails v3（v3.0.0-beta.30，beta）不在第一版采用，如迁移需独立评审 |

## 5. 复现命令

```bash
# Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest      # v2.16.0
wails doctor                                                   # 依赖自检

# 最小应用与三平台构建
wails init -n wails-poc -t vanilla
cd wails-poc && wails build                                    # darwin/arm64
wails build -platform darwin/amd64
wails build -platform darwin/universal
wails build -platform windows/amd64 -nsis                       # 需 mingw-w64 + nsis

# Fyne 对照程序
go install fyne.io/tools/cmd/fyne@latest                        # v1.7.3
CGO_ENABLED=1 go build -o app-arm64 .                           # darwin/arm64
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -o app-amd64 .  # darwin/amd64
CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc \
  go build -ldflags "-H=windowsgui" -o app.exe .                 # windows/amd64
```

产物大小与构建耗时见 §2 第 7 项（实测值）。
