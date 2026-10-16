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

### 2.2 CI 已跑通（2026-10-16，run #38790796775 全绿）

六个 job 全部通过：`lint`、`test`、`build windows-amd64`、`build darwin-arm64`、`build darwin-amd64`、`verify`。
其中三个平台 job 都**真实运行了构建产物**（`--selfcheck` + `--version`），Windows 上还确认了中文输出与退出码正常。

**CI 迭代过程中发现并修复的 9 个真实问题**（全部是本地开发环境掩盖不了的跨平台缺陷）：

| # | 现象 | 根因 | 修复 |
|---|---|---|---|
| 1 | Ubuntu `go test` 失败 | 文件名清洗按"120 个字符"截断，中文 3 字节/字 → 360 字节，超出 Linux 单文件名 255 字节上限（macOS/APFS 宽松，本地复现不出来） | 改为按字节截断（160 字节，保留 UTF-8 边界）+ 回归用例 |
| 2 | lint 失败 | 新增代码有 staticcheck S1002；本地推送前漏跑 staticcheck | 修告警 + `make ci` 一条命令复现 CI 全部检查 |
| 3 | `-race` 步骤失败 | 性能用例的 5 秒绝对阈值在竞态插桩 + 共享 runner 下被击穿（本地 189ms → runner 5.36s） | `//go:build race` 区分：race 模式只记录不判定；普通模式阈值 15 秒 |
| 4 | Linux 上测试二进制 build failed | 测试用了只在 macOS 编译的 `validateAppBundle` | 把与平台无关的 .app 处理拆到 `appbundle.go` + 4 个跨平台用例；`make ci` 增加"跨平台测试编译"检查 |
| 5 | Linux 视角 staticcheck 报未使用 | `ensureWritable` 只在 macOS 分支被调用 | 移到 `install_darwin.go`；`make ci` 增加 `GOOS=linux staticcheck` |
| 6 | Windows "安装 NSIS" 步骤被判失败 | `choco` 装完不刷新 PATH，脚本最后一条 `where.exe makensis` 返回 1，GitHub Actions 用最后一条命令的退出码作为步骤结果 | 步骤以 `exit 0` 结束 + 构建步骤显式把 NSIS 目录加入 `$env:PATH` |
| 7 | Windows "写入版本号" 1 秒失败 | Python 往 cp1252 控制台打印中文 → `UnicodeEncodeError` | 脚本输出改 ASCII + 强制 UTF-8；`make ci` 增加 `PYTHONIOENCODING=cp1252` 模拟检查 |
| 8 | Windows 冒烟测试误判失败 | GUI 子系统 exe 用 `&` 调用时 PowerShell 不等待，`$LASTEXITCODE` 为空 | 改用 `Start-Process -Wait -PassThru` 取真实退出码 |
| 9 | 汇总步骤"缺少产物" | 打包用连字符（`darwin-arm64`），汇总/发布按下划线（`darwin_arm64`）查找 | 矩阵新增 `asset` 字段，打包/校验/发布三处统一 |

**本地防线（`make ci` 现在会做 6 件事）**：gofmt、go vet、staticcheck、前端类型检查与构建、
前端产物与源码一致性、`PYTHONIOENCODING=cp1252` 跑版本脚本、`GOOS=linux staticcheck`、
linux/windows/darwin 三平台的**测试二进制编译**。

**结论**：任务书"不能只用 GOOS/GOARCH 交叉编译就认定跨平台构建成功"这条要求，在实践中确实抓到了 9 个真实缺陷——
其中 4 个（#1/#4/#5/#7）是只在目标平台上才会暴露的代码级问题。

### 2.3 仍未验证

| # | 项 | 说明 |
|---|---|---|
| 1 | 正式发布流程（`release.yml`：draft → 校验 8 个资产 → 转正） | 尚未跑过，需要一次真实标签发布验证 |
| 2 | 产物在真机安装与运行（Windows / Intel Mac） | 需要用户验收（阶段 6）；arm64 本机可自测 |
| 3 | `-webview2 embed` 安装包在干净 Windows 上的实际安装行为 | CI 只验证了构建与运行，未做安装测试 |

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

## 3.2 第一次 CI 运行的结果与修复（2026-10-16）

第一次运行（`c85fb74`）结果：**macOS arm64/amd64 两个构建 job 全绿**（Wails 安装、构建、冒烟自检、zip/dmg 打包、产物上传均通过），
但 Ubuntu 测试与 Windows 构建失败。逐项定位与修复：

| 失败 | 根因 | 修复 |
|---|---|---|
| Ubuntu `go test` 失败 | **跨平台真实 bug**：文件名清洗按"120 个字符"截断，中文 3 字节/字 → 360 字节，超出 Linux 单文件名 255 字节上限（macOS/APFS 宽松，所以本地测不出来）→ 生成时报 `file name too long` | 改为按**字节**截断（上限 160 字节，保留 UTF-8 边界），并新增两个回归用例（含 emoji/全角字符与"重名后缀后仍可创建"） |
| Windows 构建失败 | chocolatey 安装 NSIS 后**不会刷新当前 runner 进程的 PATH**，后续步骤里的 Wails 找不到 `makensis` | 安装步骤显式把 NSIS 目录写入 `$GITHUB_PATH`，并在构建前打印 `makensis` 路径便于下一次诊断 |
| （预防）Windows 打包脚本 | PowerShell 5.1 会把**无 BOM 的脚本按 ANSI 解码**，脚本里的中文路径会变乱码 | Windows 的 `run` 块改为**全部 ASCII**：动态查找 `build/bin/*.exe` 而不是写死中文文件名；已用脚本校验两个工作流的 pwsh 块不含任何非 ASCII 字符 |

## 3.3 第二次 CI 运行的结果与修复（2026-10-16）

第二次运行（`d09d85b`）暴露两个问题，都是**测试/检查设计问题而非功能缺陷**：

| 失败 | 根因 | 修复 |
|---|---|---|
| lint 失败（staticcheck） | 新增更新器代码里有一处 `S1002`（与 bool 常量比较）；本地推送前漏跑 staticcheck | 修掉告警；新增 `make ci` 目标，一条命令在本地复现 CI 的全部检查（lint + test + race + bench），避免再次漏跑 |
| test 失败（`-race` 步骤） | 性能用例的 5 秒阈值在 **竞态检测**下被击穿：本机生成 189ms，race 插桩后 runner 上 5.36s。绝对时间阈值在 race + 共享 runner 下没有意义 | 用 `//go:build race` 标记区分：race 模式下只记录耗时不做阈值判定；普通模式阈值放宽到 15 秒（本机基线 0.19 秒，仍有 60 倍余量，足以拦住数量级退化） |

**第一条 CI 修复的效果已确认**：`go test`（非 race）步骤通过，说明"Linux 文件名 255 字节"问题已解决。

## 3.4 交付后发现的 dmg 安装问题与修复（2026-10-16，v0.1.0 发布之后）

用户反馈：双击 `inspiration_0.1.0_darwin_arm64.dmg`「只能打开，没有真正安装到电脑上」。

- **根因**：打包命令为 `hdiutil create -volname 灵感 -srcfolder build/bin/灵感.app ...`，卷根目录里**只有一个 `.app`**，没有指向 `/Applications` 的符号链接、也没有安装指引。macOS 不会自动把 `.app` 装到 `/Applications`，双击的结果只是"从只读的 dmg 卷里运行"，所以看起来像"打开了但没安装"。挂载核实：`ls /Volumes/灵感` → 仅 `灵感.app`。
- **影响**：`v0.1.0` 的 macOS dmg（arm64/amd64）；`.zip` 不受影响（解压后仍需用户自行拖入「应用程序」）。另外从只读 dmg 里运行的应用**无法应用内更新**：更新器要就地整包替换，只读卷不可写，会走"引导手动安装"分支。
- **修复**：新增 `scripts/make-dmg.sh` 统一打包——暂存目录放 `.app` + `Applications → /Applications` 符号链接，生成 UDZO dmg 后**挂载自检**（应用目录、可执行文件、Info.plist、签名、别名五项），任一不满足就删掉产物并以非 0 退出；`ci.yml` / `release.yml` 的 macOS 打包步骤改为调用该脚本；`Makefile` 新增 `make dmg`，`make package-macos-*` 也一并产出 dmg。用户说明补上"双击 dmg → 拖入「应用程序」→ 推出 dmg → 从「应用程序」启动"以及"不要直接在 dmg 里双击运行"的告警。
- **验证**：本机 `make dmg` → 挂载后卷内为 `灵感.app` + `Applications`（`readlink` = `/Applications`）→ 从卷里复制出来运行 `--version` 报 `v0.1.0`、`--selfcheck` 退出码 0；负例（应用不存在 / 非有效 .app）均以退出码 1 失败且不产出文件。
- **未回填**：已发布的 `v0.1.0` dmg 仍是旧结构（Release 资产不便替换），修复从下一个版本（v0.1.1 起）生效；v0.1.0 用户的临时办法见 `docs/user-guide.md` §1。

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
