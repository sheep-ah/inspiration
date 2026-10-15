# 三平台构建、发布与应用内更新方案（阶段 1 交付物）

## 1. 构建与分发方案（选定 Wails v2.16.0 后）

| 平台 | 产物 | 构建方式 | 实测状态 |
|---|---|---|---|
| Windows x64 | `灵感-Setup-<ver>-amd64.exe`（NSIS，建议 user scope，免 UAC）+ 便携版 `灵感-<ver>-windows-amd64.exe` | `wails build -platform windows/amd64 -nsis -webview2 download` | 本机交叉编译 ✅（exe 11.4 MB / 安装包 6.1 MB，共 12.0s）；**未在 Windows 上运行验证** |
| macOS Intel (amd64) | `灵感-<ver>-darwin-amd64.dmg`（内含 `.app`，同时提供 `.zip` 供自动更新使用） | `wails build -platform darwin/amd64` + `hdiutil`/`create-dmg` 打 dmg | 本机交叉编译 ✅（29.3s）；**未在 Intel Mac 上运行验证** |
| macOS Apple Silicon (arm64) | `灵感-<ver>-darwin-arm64.dmg` + `.zip` | `wails build -platform darwin/arm64` | 本机原生编译 ✅；**未在真机安装验证** |
| （可选）macOS 通用包 | `灵感-<ver>-darwin-universal.dmg` | `wails build -platform darwin/universal`（16 MB） | 本机 ✅ |

- Windows 的 WebView2 运行时依赖由 NSIS 安装器处理（`-webview2 download`：安装时下载；`embed`：随包携带离线安装器，体积增大；`browser`：打开浏览器引导）。
- 图标：`build/appicon.png`（macOS 由 Wails 生成 icns；Windows 生成 `.syso`）；NSIS 支持自定义安装脚本（`build/windows/installer/`）。
- 交叉编译只用于本地快速验证；**CI 必须在对应操作系统的 runner 上原生构建**（任务书 §5.1 明确要求）。

## 2. CI/CD 设计（`.github/workflows/`）

### 2.1 `ci.yml`（推送 `main` 与 PR）

```
jobs:
  lint:   gofmt -l / go vet / staticcheck          （ubuntu-latest）
  test:   go test ./... -race -count=1              （ubuntu-latest）+ 前端 npm ci && npm run build
  build:  matrix: windows-latest / macos-14(arm64) / macos-15(arm64 交叉到 amd64)
          → wails build（各平台原生命令）→ 上传 artifact（保留 14 天）
  verify: 汇总 job 状态 + 下载 artifact 做体积/文件名/校验和检查
```

- 需要 runner 上安装：Go（`actions/setup-go`）、Node（`actions/setup-node`）、`wails` CLI（`go install`，固定版本）、macOS 无需额外（CLT 自带）、Windows 需 `choco install nsis` 或用 `makensis` action。
- **不发布正式版本**；普通提交只产出 CI artifact。

### 2.2 `release.yml`（推送 `v*` 标签）

```
1. 校验标签格式 ^v\d+\.\d+\.\d+$，并与 internal/version 中的版本号一致（不一致直接失败）
2. 确认该 tag 的提交已通过 ci.yml（等待/查询对应 commit 的 check 状态）
3. 全平台构建（原生命令，矩阵同上）
4. 生成 checksums.txt（sha256）+ latest.json（版本、发布日期、各平台资产名与 sha256、更新说明）
5. 任一平台缺失或失败 → 整体失败，禁止发布残缺 Release
6. 创建 GitHub Release（draft → 校验资产齐全 → 发布为 latest）
7. 输出 Release URL 与资产清单
```

版本号来源：`internal/version.Version`（构建时 `-ldflags "-X ...=v1.2.3"`），保证“应用内可见版本 = Release 标签 = 产物文件名”。

语义化版本：`MAJOR` 不兼容变更 / `MINOR` 向后兼容新功能 / `PATCH` 修复。

## 3. 应用内更新方案

### 3.1 流程

1. `GET https://api.github.com/repos/<owner>/inspiration/releases/latest`（可配置镜像/代理），只接受非 prerelease、非 draft 的稳定版；
2. 语义化版本比较：无更新 → 显示“已是最新版本”；有更新 → 显示版本号、发布日期、更新说明（Release body）；
3. 按 `runtime.GOOS/GOARCH` 选择资产（命名约定：`灵感-<ver>-darwin-arm64.zip`、`灵感-<ver>-darwin-amd64.zip`、`灵感-<ver>-windows-amd64-Setup.exe`）；
4. 下载到 `update/` 临时目录，**显示进度**（`io.TeeReader` + 事件推送到前端）、支持中断重试；
5. 下载完成后计算 sha256，与 `checksums.txt` 中的值比对，不一致即删除并报错；
6. 平台相关安装（见 §3.2/§3.3）；失败时恢复旧版本或引导用户手动安装；
7. 更新后启动应用，校验模板列表与数据库仍可用（阶段 3/5 的验收项）。

> 注：`checksums.txt` 与 Release 资产同源（都来自 GitHub Release 的 HTTPS 响应），能防传输损坏与镜像篡改，但**不能防仓库被攻破**；如需要更强保证，后续可加“校验和数字签名”（独立需求）。

### 3.2 Windows 更新

- 主路径：下载 `...-Setup.exe`（NSIS，user scope）→ 校验 sha256 → 用 `ShellExecute` 启动安装器并带 `/S`（静默）参数，安装完成后由安装器脚本重新拉起应用；安装器写入的是同一个安装目录，用户数据在 `%APPDATA%\灵感\` 不受影响。
- 备选路径（便携版）：把新 exe 下载到临时目录 → 写一个 `update.cmd`（等待当前进程退出 → 覆盖 exe → 重新启动）→ 以 `DETACHED_PROCESS` 启动该脚本 → 应用退出。
- 不做“运行中覆盖自身 exe”：Windows 会锁定正在运行的可执行文件。
- 签名：未签名的安装包会触发 SmartScreen 警告（见 `risks-and-unverified.md`）。

### 3.3 macOS 更新

- 下载 `...-darwin-<arch>.zip` → 校验 sha256 → 解压到 `update/` 临时目录；
- 校验包完整性：`codesign --verify --deep --strict <new.app>`（若已签名/公证则同时 `spctl -a -t exec -vv`）；
- 整包替换：把当前 App 移动到临时/废纸篓位置 → 把新 `.app` 移动到原路径 → 重启；任一步失败则回滚旧包。**不修改包内文件**（Apple 文档明确：改动包内内容会使签名失效，Apple Developer Forums #781548、TN2206）；
- 若当前目录不可写（例如装在 `/Applications` 但无写权限）或校验不通过：改为“打开下载的 dmg / 显示 Finder 位置”并引导用户拖拽安装（满足任务书“按平台要求完成更新或引导用户安装”）；
- 未做公证（notarization）的包在其他机器上会被 Gatekeeper 拦截，**需要 Apple Developer ID 证书**才能做到“下载即安装”。

### 3.4 依赖评估

| 方案 | 版本 / 许可 | 评估 |
|---|---|---|
| 自研更新器 | 项目自有 | 需要的功能（进度事件、平台安装、UI 提示、可测试）都要自己做，代码量小且完全可控；**默认方案** |
| `creativeprojects/go-selfupdate` | v1.6.0 / MIT, Apache-2.0 | 提供 GitHub/Gitea/GitLab 检测、按 OS/arch 选择资产、sha256 校验、回滚；可作为“检测+下载+校验”的替代实现。缺点是资产命名匹配规则固定，且 macOS 的整包替换仍需自己实现 |
| `minio/selfupdate` | v0.6.0 / MPL-2.0 | 只支持二进制替换，Windows 兼容性一般，macOS 整包替换不适用 |

**冻结决定**：阶段 5 默认自研更新器；`go-selfupdate` 作为可替换的下载/校验实现保留评估结论，不引入为强制依赖。

## 4. 发布与数据安全

- 数据目录（用户数据）与安装目录分离，卸载/重装不清空用户数据（详见 `architecture.md`）：macOS `~/Library/Application Support/灵感/`，Windows `%APPDATA%\灵感\`。
- 更新流程不清空 `templates/`、`contracts/`、`inspiration.db`。
- Release 资产不包含任何用户数据；构建过程不上传合同/模板内容。
- 令牌与签名材料：只放在 GitHub Actions Secrets（`APPLE_CERTIFICATE_P12_BASE64`、`APPLE_ID`/`APPLE_APP_PASSWORD`/`APPLE_TEAM_ID`、`WINDOWS_CERT_*`），**不入库**（任务书 §5.2）。

## 5. 尚未验证（必须明确）

| 项 | 状态 |
|---|---|
| Windows 安装/运行/更新 | 未验证（本机只能交叉编译） |
| macOS Intel 真机运行 | 未验证（无 Intel 机器） |
| macOS arm64 真机安装/更新 | 构建已验证，安装与更新未验证 |
| 代码签名与公证（Windows Authenticode / Apple Developer ID） | 未验证，需要证书 |
| GitHub Actions 实际运行 | 未验证（工作流尚未创建，阶段 4 交付） |
| 更新器对真实 Release 的端到端测试 | 未验证（依赖阶段 4 的发布产物） |
