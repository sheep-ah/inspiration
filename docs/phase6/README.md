# 阶段 6（最终验收与交付）

任务书 §4 阶段 6 要求：交付完整源码、自动化工作流、各平台安装包、测试报告、用户使用说明和开发文档；
无法在当前环境中验证的平台行为必须明确标记，不能伪造验证结果。

## 交付物

| 交付物 | 位置 | 状态 |
|---|---|---|
| 完整源码 | 仓库 `main`（提交 `b136ef3` 起） | ✅ 26 个 Go 源文件 4703 行 + 前端 1931 行 + CI 749 行 + 测试 3078 行 |
| 自动化工作流 | [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml)、[`release.yml`](../../.github/workflows/release.yml) | ✅ 均已在真实运行中验证（CI 六 job 全绿；发布流程产出 8 个资产） |
| 各平台安装包 | [GitHub Releases v0.1.0](https://github.com/sheep-ah/inspiration/releases/tag/v0.1.0) | ✅ Windows x64（安装版 + 便携版）、macOS arm64、macOS amd64（各含 zip/dmg） |
| 测试报告 | [`acceptance-report.md`](acceptance-report.md) §1–§6 | ✅ 77 个用例 + 4 个基准；`make ci` 一条命令复现全部检查 |
| 用户使用说明 | [`../user-guide.md`](../user-guide.md) | ✅ |
| 开发文档 | [`../development.md`](../development.md) | ✅ |
| 最终验收报告 | [`acceptance-report.md`](acceptance-report.md) | ✅ 逐条对照任务书 §6.1–§6.4，未验证项集中在 §7 |
| 未验证项清单 | [`acceptance-report.md`](acceptance-report.md) §7（9 项） | ✅ 每项注明原因与建议验证方式 |

## 阶段 6 结论

- **技术方案 / 核心功能 / 三平台构建与发布 / 应用内更新（macOS）** 均已实现并通过可复现验证；
- **Microsoft Word 打开结果、Windows 安装与更新、macOS Intel 真机、GUI 点击式更新、代码签名与公证、
  Word/WPS 转换通道、磁盘写满场景、中文输入法实操、超大模板性能** 未在当前环境验证，
  已在验收报告 §7 逐条标注，**不视为通过**。

## 后续（可选）

1. 在 Windows 上安装 `inspiration_0.1.0_windows_amd64_setup.exe` 并生成一份合同，回填验收报告 §6.2 与 §7 第 2 项；
2. 用 Microsoft Word 打开一份生成结果，回填 §3 第 12 项与 §7 第 1 项；
3. 发布 `v0.1.1`/`v0.2.0` 后，在应用界面点一次「检查更新 → 下载并更新」，回填 §7 第 4 项；
4. 若要启用签名：购买 Apple Developer 证书与 Windows 代码签名证书后，在 CI 增加签名/公证步骤，并更新用户说明 §1。
