# 阶段 4（跨平台 CI/CD）交付记录

任务书 §4 阶段 4 要求：`main` 推送自动测试并构建三个目标平台、产物上传为 CI Artifact；创建版本标签后执行正式发布，发布前确认测试与构建成功，把安装包、版本号和更新说明发布到 GitHub Releases；测试构建与正式发布必须分离。

## 1. 两个工作流（测试构建与正式发布分离）

| 文件 | 触发 | 作用 |
|---|---|---|
| `.github/workflows/ci.yml` | 推送 `main`、PR、手动 | 格式检查、静态分析、单元/端到端测试、**三平台原生构建**、产物上传为 Artifact（保留 14 天）。**不发布任何正式版本** |
| `.github/workflows/release.yml` | 推送 `v*` 标签 | 校验标签 → 测试门禁 → 三平台构建 → 校验和与 `latest.json` → 以 **draft** 创建 Release → 校验资产齐全 → 转为正式发布 |

### 1.1 `ci.yml` 任务

| Job | Runner | 内容 |
|---|---|---|
| `lint` | ubuntu-latest | `gofmt -l`、`go vet`、`staticcheck`、`tsc --noEmit`、`vite build`，并校验 **`frontend/dist` 与源码一致**（防止改了前端忘了提交产物） |
| `test` | ubuntu-latest | 先构建前端（`go:embed` 需要），再 `go test ./... -count=1`、`-race`（核心包）、性能基准 |
| `build` | windows-latest / macos-14 ×2 | **原生构建**：`wails build`；Windows 出便携版 + NSIS 安装包（`-nsisscope user -webview2 embed`）；macOS 出 `.zip` 与 `.dmg`。**构建后真实运行二进制做冒烟测试**（`--selfcheck`），捕获"能编译但起不来" |
| `verify` | ubuntu-latest | 下载全部 Artifact，逐个校验六个产物存在、体积正常，生成 `SHA256SUMS.txt`，写 Job Summary |

**关键点：不使用 `GOOS/GOARCH` 交叉编译冒充平台构建。** Windows 在 `windows-latest` 上构建，macOS 在 Apple Silicon runner 上原生构建 arm64、并在同一 runner 上交叉编译 Intel 版本（本机已验证该路径可行，见阶段 1 实测）。

### 1.2 `release.yml` 任务

```
validate → test → build(3 平台) → publish
```

| 步骤 | 保障 |
|---|---|
| 标签格式校验 `^v[0-9]+\.[0-9]+\.[0-9]+$` | 形如 `v1.0.0` 才发布（`v1.2`、`1.2.3`、`v1.2.3-rc1` 均被拒绝） |
| 标签提交必须是 `main` 的后代 | 禁止从功能分支直接发布 |
| 测试门禁（`needs: validate`） | 测试不过就不构建、不发布 |
| 版本号一致性 | `scripts/set-version.py` 把标签写入 `wails.json`（→ macOS `Info.plist`、Windows 版本资源与安装器）；构建后用 `--version`（macOS）/ 文件版本资源（Windows）**反向校验**产物版本 == 标签 |
| 冒烟测试 | 真实运行二进制 `--selfcheck` |
| 产物自检 | 数量与体积校验，异常直接失败 |
| 发布顺序 | **先 draft** → 上传资产 → 校验资产数 == 8（6 安装包 + `checksums.txt` + `latest.json`）→ 才 `--draft=false` 转正式 |
| 更新说明 | 自动生成（上一个标签到本标签的提交列表 + 安装包清单 + 校验说明 + 未签名提示） |

**不会出现"构建失败却发布了残缺版本"**：任一平台构建失败 → `build` job 失败 → `publish` 的 `needs` 不满足，连 draft 都不会创建。

### 1.3 发布产物

| 平台 | 产物 |
|---|---|
| Windows x64 | `inspiration_<ver>_windows_amd64_setup.exe`（NSIS，user scope 免 UAC，内嵌 WebView2 引导）、`inspiration_<ver>_windows_amd64_portable.exe` |
| macOS Apple Silicon | `inspiration_<ver>_darwin_arm64.dmg`、`inspiration_<ver>_darwin_arm64.zip` |
| macOS Intel | `inspiration_<ver>_darwin_amd64.dmg`、`inspiration_<ver>_darwin_amd64.zip` |
| 元数据 | `checksums.txt`（SHA256）、`latest.json`（版本、标签、资产清单含 os/arch/kind/sha256/size） |

`latest.json` 与 `checksums.txt` 是为阶段 5 的**应用内更新器**准备的：更新器只需读取 `releases/latest` 与这两个文件即可确定该下载哪个包并校验完整性。

## 2. 本次已验证 / 未验证

### 2.1 本地已验证（不依赖 GitHub）

| 项 | 方式 | 结果 |
|---|---|---|
| 工作流语法与 Actions 语义 | `actionlint .github/workflows/*.yml` | ✅ 零 error（仅 shellcheck 提示，已逐条修掉） |
| YAML 结构 | `python3 -c "yaml.safe_load(...)"` | ✅ 两个工作流的 jobs 解析正确 |
| 标签校验规则 | 本地跑同一段 shell | ✅ `v1.2.3` 通过；`v1.2`/`1.2.3`/`v1.2.3-rc1`/`vabc` 全部拒绝 |
| `checksums.txt` + `latest.json` 生成 | 用 6 个假产物空跑 | ✅ 6 项资产分类正确（os/arch/kind），sha256 与 checksums 完全一致 |
| 更新说明生成（有上一标签 / 首次发布） | 本地建临时 tag 空跑 | ✅ 两个分支都正确，并已清理临时 tag |
| `scripts/set-version.py` | 本地运行 | ✅ 写入 `1.2.3`；非法输入被拒绝；支持 `python3`/`python` 两种环境 |
| 三平台构建命令 | 阶段 1/2 本机实测 | ✅ `wails build` 的 darwin/arm64、darwin/amd64、windows/amd64 + NSIS 均已跑通 |

### 2.2 未验证（需要真实 CI 运行）

| # | 项 | 说明 |
|---|---|---|
| 1 | **GitHub Actions 真实执行** | 需要推送后由 CI 实跑；本地无法模拟 runner 环境（尤其 Windows/macOS 的 runner 差异） |
| 2 | Windows 首次构建 | NSIS 安装、`-webview2 embed` 下载引导包、PowerShell 打包与版本资源校验，均需在 runner 上首次验证 |
| 3 | macOS 上 `hdiutil`/`ditto` 打包 | 命令本机可用，但 CI 上未跑过 |
| 4 | 正式发布流程（draft → 校验 → 转正） | 需要一次真实标签发布验证 |
| 5 | 产物在真机安装与运行 | 需要用户验收（阶段 6） |

## 3. 关于 Actions 计费（仓库已设为 public）

- 仓库当前为 **public**（`private: false`）→ **Actions 用量不计费**，包括 macOS runner（私有仓库才会按 10 倍计费）。
- 代价与注意事项：
  1. public 仓库的 macOS runner **排队优先级较低**，高峰期可能等待较久（不是失败）；
  2. Artifacts、Releases、Actions 日志对所有人可见——发布产物与更新日志公开展示，但不含任何用户数据（应用本身是本地处理，仓库里也没有真实合同/客户信息，已核查）；
  3. 若日后改回私有仓库，需要重新评估计费，并考虑给 macOS job 加路径过滤。
- 已在提交前核查仓库内容：无客户名称、无真实合同数据、无令牌或密钥（`grep` 业务关键字与凭据模式均为零命中；`wails.json` 中的「身边云」是本公司署名）。

## 3.1 一个容易踩的坑：别提交 Wails 就地改写的文件

`wails build` 会**就地改写** `build/windows/installer/wails_tools.nsh`（把 `{{.Info.ProductVersion}}` 等占位符替换成实际值）并修改 `wails.json` 的版本号。这两个文件在仓库里都受版本控制：

- **不要把改写后的 `wails_tools.nsh` 提交**：里面 `INFO_PRODUCTVERSION` 会被写死成构建时的版本（例如 `0.0.0`），此后安装器版本号将永远停留在那个值，与 Release 标签不一致；
- 提交前用 `git checkout -- build/windows/installer/wails_tools.nsh wails.json` 还原成模板形式（CI 每次都是全新检出，因此不受影响）；
- 已实测验证版本管道：`scripts/set-version.py 9.9.9` → `wails build -platform windows/amd64 -nsis` → **exe 与 NSIS 安装包内均为 UTF-16 的 "9.9.9"**，说明版本号确实一路传到产物；
- `.gitignore` 已忽略 `build/windows/installer/tmp/`（`-webview2 embed` 会下载约 1.7MB 的 WebView2 引导包）与 `dist/`。

## 4. 发布操作手册（给使用者）

```bash
# 1) 确认 main 上一切正常（CI 绿）
# 2) 更新版本号（可选，CI 会用标签覆盖 wails.json）
python3 scripts/set-version.py 1.0.0
git commit -am "chore(release): 1.0.0"
git push origin main

# 3) 打标签并推送（触发正式发布）
git tag -a v1.0.0 -m "灵感 v1.0.0"
git push origin v1.0.0

# 4) 观察 Actions：Release 工作流 → validate → test → build → publish
#    publish 会先建 draft、上传 8 个资产、校验后转正式
# 5) 发布完成后检查 https://github.com/sheep-ah/inspiration/releases
```

回滚：若发布后发现问题，`gh release edit v1.0.0 --draft` 可把已发布版本退回 draft（应用内更新器只读 `releases/latest`，因此用户不会再收到该版本）。

## 5. 与任务书条款的对照

| 任务书 §5 条款 | 实现 |
|---|---|
| 推送 main 执行格式检查、单元测试、静态分析、核心模块构建、三平台构建、保存产物、汇总结果 | `lint` / `test` / `build` / `verify` 四个 job 一一对应 |
| 平台构建需用合适 Runner，不能只设 GOOS/GOARCH 就认定成功 | Windows 用 `windows-latest`，macOS 用 `macos-14`；并对每个产物做**真实运行冒烟测试** |
| `v1.0.0` 形式的标签触发正式发布 | `release.yml` 仅响应 `v*` 标签 |
| 校验标签格式、确认提交已通过检查、三平台分发、生成版本信息、计算校验和、发布 Release、上传说明、确认成功 | `validate` → `test` → `build` → `publish`（含 `checksums.txt`、`latest.json`、draft 校验后转正） |
| 禁止构建失败时继续发布 | `publish` 依赖全部前置 job；资产数不符时保持 draft 并失败 |
| 不得将令牌/私钥写入源码 | 工作流只用内置 `GITHUB_TOKEN`（`contents: write` 仅限 `publish`），无任何硬编码凭据 |

## 6. 阶段 5 的前置准备（已就绪）

- 发布流程产出 `checksums.txt` + `latest.json`，更新器需要的"版本、平台资产名、sha256、大小"全部具备；
- 资产命名稳定（`inspiration_<ver>_<os>_<arch>.<kind>.<ext>`），更新器可按 `runtime.GOOS/GOARCH` 精确匹配；
- macOS 提供 `.zip`（整包替换更新用）与 `.dmg`（人工安装）；
- Windows 提供 NSIS 安装包（静默安装 `/S`）与便携版（替换 exe）。
