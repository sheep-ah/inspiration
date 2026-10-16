package updater

import (
	"fmt"
	"strings"
)

// maxWaitRounds 是"等待应用退出"的最大轮数（每轮 ping -n 2 ≈ 1 秒，约 30 秒）。
const maxWaitRounds = 30

// 路径通过环境变量传给脚本，而不是把字面量写进 .cmd。原因：cmd.exe 按**控制台代码页**
// 解码脚本字节，而我们的安装目录与数据目录都含中文（「灵感」）——UTF-8 写进 .cmd 后会被
// 解成乱码，`if exist` / `copy` 就指向一个不存在的路径（表现为"装完了但没重启"或压根没覆盖）。
// 环境变量是以 UTF-16 传递的，不受脚本编码影响，因此脚本本身可以保持纯 ASCII。
const (
	envUpdateExe = "INSPIRATION_UPDATE_EXE" // 当前应用 exe 的完整路径
	envUpdatePkg = "INSPIRATION_UPDATE_PKG" // 已下载的更新包路径
)

// windowsUpdateScript 生成"等应用退出 → 安装/覆盖 → 重启 → 自删"的批处理脚本。
//
// 为什么单独放在非平台文件里：脚本内容不依赖 Windows 也能构造，放在这里可以让
// 全平台单测检查它的关键不变量（行尾 CRLF、纯 ASCII、不使用 tasklist/find），
// Windows 上再跑一次真实执行（用与生产完全相同的启动方式）。
//
// 四条必须守住的细节（每条都对应一次真实故障）：
//
//  1. **行尾必须 CRLF**。cmd.exe 执行 goto 时按字节偏移重新定位脚本文件；LF-only 会让
//     偏移算错，goto 之后从行中间接着执行，出现 'el'/'tart'/'/nobreak' 这类碎片命令。
//
//  2. **不要内嵌中文路径**（见上面 envUpdate* 的说明）。
//
//  3. **不要用 tasklist/find 判断"应用是否已退出"**。曾经用 `tasklist /FI "PID eq N" | find "N"`
//     匹配 PID：PID 会被系统复用，一旦被复用循环就永不退出——用户看到的是一个卡住的
//     `find "45152"` 窗口和"更新一直没反应"。改成**直接探测能不能写 exe**：运行中的 exe
//     被加载器非共享写持有，追加写必然失败；应用退出后立刻成功。既不需要 PID，
//     也精确对应"我能不能替换它了"。追加的换行对 PE 文件无害，且紧接着就会被覆盖/重装。
//
//  4. **不要用 timeout 等待**。timeout.exe 需要控制台输入句柄（脚本以隐藏控制台运行，
//     见 install_windows.go 的启动参数），改用 ping 做延时。
//
// 返回的 method 用于 ApplyResult.Method：nsis-silent / portable-swap。
//
// downloadPath / exePath 只用于判定分支（安装包 vs 便携版），不再写进脚本内容；
// 脚本通过 envUpdatePkg / envUpdateExe 读取它们，因此脚本主体是纯 ASCII。
func windowsUpdateScript(downloadPath, exePath string) (method, script string) {
	return windowsUpdateScriptLimited(downloadPath, exePath, maxWaitRounds)
}

// windowsUpdateScriptLimited 与 windowsUpdateScript 相同，但可以指定等待轮数上限。
// 存在的意义是可测性：单测把上限压到几轮，既能验证"应用还在运行时必须等待"，
// 又不会把测试拖住。
func windowsUpdateScriptLimited(downloadPath, exePath string, maxRounds int) (method, script string) {
	if maxRounds <= 0 {
		maxRounds = maxWaitRounds
	}
	lines := []string{
		"@echo off",
		"setlocal",
		// 两个路径都由启动方通过环境变量给出；缺失时直接结束，避免弹出无意义的报错
		fmt.Sprintf("if not defined %s goto done", envUpdatePkg),
		fmt.Sprintf("if not defined %s goto done", envUpdateExe),
		"set tries=0",
		":wait",
		// 探测 exe 是否已可写（= 应用已退出）。失败说明还在运行，继续等。
		fmt.Sprintf(`2>nul >>"%%%s%%" echo.`, envUpdateExe),
		"if not errorlevel 1 goto ready",
		"set /a tries+=1 >nul",
		fmt.Sprintf("if %%tries%% GEQ %d goto done", maxRounds),
		"ping -n 2 127.0.0.1 >nul",
		"goto wait",
		":ready",
	}

	if isSetupPackage(downloadPath) {
		method = "nsis-silent"
		// /wait：等安装器结束再决定是否重启
		lines = append(lines, fmt.Sprintf(`start "" /wait "%%%s%%" /S`, envUpdatePkg))
	} else {
		method = "portable-swap"
		lines = append(lines,
			fmt.Sprintf(`copy /y "%%%s%%" "%%%s%%" >nul`, envUpdatePkg, envUpdateExe))
	}

	lines = append(lines,
		":done",
		fmt.Sprintf(`if exist "%%%s%%" start "" "%%%s%%"`, envUpdateExe, envUpdateExe),
		// 自删：`(goto)` 让 cmd 立刻停止读取当前脚本文件，之后删它就不会报
		// "找不到批处理文件"（直接写 `del "%~f0"` 作为最后一行有这个风险）。
		// 这一行同样不经过 fmt：`%~f0` 里的 % 交给 Sprintf 会变成动词转义。
		`(goto) 2>nul & del "%~f0"`,
	)

	return method, strings.Join(lines, "\r\n") + "\r\n"
}

// isSetupPackage 判断下载到的是 NSIS 安装包还是便携版可执行文件。
func isSetupPackage(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, "setup.exe")
}
