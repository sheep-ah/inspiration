# 项目结构、数据存储与安全设计（阶段 1 交付物）

## 1. 仓库目录设计

```
inspiration/
├── .github/workflows/          # ci.yml（main 推送） / release.yml（v* 标签）
├── cmd/inspiration/main.go     # 应用入口（Wails app.New + 资源嵌入）
├── internal/
│   ├── app/                    # Wails 绑定层（薄）：暴露给前端的方法与事件
│   ├── version/                # 版本号（ldflags 注入）+ 构建时间/commit
│   ├── config/                 # 应用数据目录、可调参数
│   ├── store/                  # SQLite（modernc.org/sqlite）+ 文件存储，事务与迁移
│   ├── docx/                   # 自研 OOXML 引擎（阶段 2 从 poc 迁入，附语料回归测试）
│   ├── template/               # 模板服务：导入、扫描校验、改名、搜索、删除
│   ├── contract/               # 合同生成：字段模型、校验、目录命名、落盘
│   ├── updater/                # 版本检查、下载、校验、平台安装
│   └── logging/                # 日志（脱敏、滚动、不上传）
├── frontend/                   # vanilla-ts + Vite（dist/ 由 Wails 嵌入）
│   ├── index.html  src/  package.json  vite.config.ts
├── build/                      # appicon.png / darwin/Info.plist / windows(manifest, installer)
├── docs/                       # 本目录（阶段 1 评审）与后续用户/开发文档
├── poc/docx-eval/              # 阶段 1 证据：DOCX 引擎原型 + 语料 + 独立校验脚本
├── Makefile                    # dev/build/test/lint/package 统一入口
├── go.mod  go.sum  README.md
```

**依赖清单（冻结）**

| 依赖 | 版本 | 用途 | 许可 |
|---|---|---|---|
| `github.com/wailsapp/wails/v2` | v2.16.0 | GUI 框架 | MIT |
| `modernc.org/sqlite` | v1.60.1 | SQLite 驱动（纯 Go，无 cgo） | BSD-3-Clause |
| 自研 `internal/docx` | — | DOCX 占位符替换 | 项目自有 |
| 前端 | vite + typescript | 构建工具 | MIT |

**已评估但不采用**：`gomutex/godocx`、`lukasjarosch/go-docx`、`nguyenthenguyen/docx`（见 `docx-evaluation.md`）；`fyne.io/fyne/v2`（见 `gui-evaluation.md`）。

## 2. 应用数据目录

使用 Go 标准库 `os.UserConfigDir()` + 固定子目录名，避免把用户数据放进安装目录或临时目录：

| 平台 | 数据根目录 |
|---|---|
| macOS | `~/Library/Application Support/灵感/` |
| Windows | `%APPDATA%\灵感\`（`C:\Users\<用户>\AppData\Roaming\灵感\`） |

```
<数据根>/
├── inspiration.db          # SQLite（模板元数据、设置、合同记录）
├── templates/              # 模板 DOCX 副本，文件名 = <uuid>.docx（不使用用户原始文件名）
├── contracts/YYYY/MM/      # 生成的合同（仅保存用户选择的目标位置之外的兜底/历史索引）
├── logs/                   # 滚动日志（脱敏，见 §4）
└── update/                 # 更新下载与解压临时目录（成功后清理）
```

**行为约定（写入用户文档）**

- 应用升级/重装/卸载**都不删除**数据根目录（卸载由用户在安装器中显式勾选“同时删除我的数据”才清理）。
- 备份 = 复制整个数据根目录（文档中给出路径与一键“打开数据目录”按钮）。
- 模板 DOCX 一律先复制到 `templates/<uuid>.docx` 再入库；数据库只保存元数据与相对路径，不使用用户选择的临时路径。

## 3. 数据库结构（阶段 2 基线）

```sql
PRAGMA journal_mode=WAL;      -- 提升并发与崩溃恢复能力
PRAGMA foreign_keys=ON;

CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL          -- schema_version 等
);

CREATE TABLE IF NOT EXISTS templates (
  id           TEXT PRIMARY KEY,          -- uuid v4
  name         TEXT NOT NULL,             -- 用户可编辑的模板名（唯一性不做强约束）
  stored_file  TEXT NOT NULL,             -- templates/ 下的文件名（uuid.docx）
  size_bytes   INTEGER NOT NULL,
  sha256       TEXT NOT NULL,             -- 导入时计算的原始文件摘要
  placeholders TEXT NOT NULL DEFAULT '[]',-- JSON: [{name,count,parts:[]}]
  field_usage  TEXT NOT NULL DEFAULT '{}',-- JSON: 字段 -> 出现次数
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL,
  deleted_at   INTEGER                    -- 软删除
);
CREATE INDEX IF NOT EXISTS idx_templates_name ON templates(name);

CREATE TABLE IF NOT EXISTS contracts (
  id            TEXT PRIMARY KEY,
  template_id   TEXT,                     -- 关联模板（模板删除后置 NULL，不影响历史）
  template_name TEXT NOT NULL,            -- 生成时的模板名快照
  file_name     TEXT NOT NULL,
  file_path     TEXT NOT NULL,
  sha256        TEXT NOT NULL,
  field_names   TEXT NOT NULL DEFAULT '[]',-- 只用到的字段名，不含字段值
  created_at    INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
```

**数据安全决策**

- **不落盘任何字段值**（银行账号、税号、电话等敏感信息只存在于内存与前端的当前会话）：`contracts` 表只记录“用到了哪些字段”，不记录值；因此“生成失败不丢数据”通过内存保留 + 界面提示实现，而不是写磁盘。
- 日志只记录：模板 id/名称、字段**名**（不含值）、目标路径、耗时、错误码与堆栈；禁止记录字段值、合同正文。
- 模板删除采用软删除 + 物理文件在事务提交后删除；已生成合同不依赖模板文件存在。

**删除模板不影响已有合同**：`contracts` 通过 `template_id`（可空）与 `template_name` 快照关联，模板删除时不触碰已生成文件。

**上传失败回滚**：先写临时文件 → 校验（zip/XML/占位符扫描）→ `fsync` + `rename` 到 `templates/` → 再写数据库；数据库写失败则删除已落盘文件（补偿事务）。

## 4. 安全与合规

| 主题 | 措施 |
|---|---|
| 数据处理原则 | 默认本地处理；除“检查更新”外不发起任何网络请求；不上传模板/合同/字段值 |
| 模板导入校验 | 扩展名 `.docx`；zip 结构校验；`[Content_Types].xml` 主类型校验；拒绝 `.docm`/`.doc`/加密文档；限制单文件大小（默认 50 MB） |
| 路径安全 | 落盘文件名由应用生成（uuid / 用户输入经清洗，禁止路径分隔符与保留名）；目标目录写入前做 `filepath.Clean` + 前缀校验，防目录穿越 |
| 覆盖保护 | 目标文件已存在时自动追加 `-1`、`-2` 后缀，**不静默覆盖**；模板文件永不作为输出目标 |
| 更新完整性 | 仅 HTTPS；下载后校验 sha256（与 Release 的 `checksums.txt` 比对）；macOS 额外 `codesign --verify`；Windows 安装器签名（需证书） |
| 更新来源 | 只接受配置仓库的 `releases/latest`，拒绝 prerelease/draft；不执行 Release 之外的下载 |
| 恶意模板 | 不执行宏、不解析外部关系、不下载远程内容；对超大/畸形 XML 加解析上限与超时 |
| 敏感字段 | 电话、账户号、纳税人识别号全程按字符串处理（无数字转换，避免前导零丢失）；界面可一键清空表单；不做剪贴板后台读取 |
| 依赖安全 | 锁定版本、`go.sum` 校验、CI 中 `govulncheck`（阶段 4 起） |
| 日志 | 脱敏规则见 §3；日志仅存本地 |

## 5. 前端与绑定约定（阶段 2 基线）

- 绑定层只暴露窄接口：`ListTemplates(query)`、`ImportTemplate(path)`、`RenameTemplate(id,name)`、`DeleteTemplate(id)`、`InspectTemplate(id)`、`GenerateContract(req)`、`OpenFile(path)`、`RevealInFolder(path)`、`CheckUpdate()`、`DownloadUpdate(id)`、`ApplyUpdate(id)`、`CurrentVersion()`。
- 长任务（导入扫描、生成、下载）通过 Wails 事件推送进度，前端渲染进度与可取消状态。
- 错误统一为 `{code, message, detail}`，界面显示可理解的中文提示；`detail` 只写日志。
