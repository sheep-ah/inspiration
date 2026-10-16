package updater

import (
	"fmt"
	"strings"
)

// maxWaitRounds 是"等待应用退出"的最大轮数（每轮 ping -n 2 ≈ 1 秒，约 5 分钟）。
const maxWaitRounds = 300

// 路径通过环境变量传给脚本，而不是把字面量写进 .cmd。原因：cmd.exe 按**控制台代码页**
// 解码脚本字节，而我们的安装目录与数据目录都含中文（「灵感」）——UTF-8 写进 .cmd 后会被
// 解成乱码，`if exist` / `copy` 就指向一个不存在的路径（表现为"装完了但没重启"或压根没覆盖）。
// 环境变量是以 UTF-16 传递的，不受脚本编码影响，因此脚本本身可以保持纯 ASCII。
const (
	envUpdateExe = "INSPIRATION_UPDATE_EXE" // 当前应用 exe 的完整路径
	envUpdatePkg = "INSPIRATION_UPDATE_PKG" // 已下载的更新包路径
)

// windowsUpdateScript 生成"等当前进程退出 → 安装/覆盖 → 重启 → 自删"的批处理脚本。
//
// 为什么单独放在非平台文件里：脚本内容不依赖 Windows 也能构造，放在这里可以让
// 全平台单测检查它的关键不变量（尤其是行尾必须是 CRLF），Windows 上再跑一次真实执行。
//
// 两个必须守住的细节（都踩过真实的坑）：
//
//  1. **行尾必须 CRLF**。cmd.exe 执行 goto 时是按字节偏移重新定位脚本文件的；
//     LF-only 的文件会让偏移算错，goto 之后从行中间接着执行，于是出现
//     'el'/'tart'/'/nobreak' 这类"碎片命令"，更新静默失败（v0.1.0 的 Windows
//     用户实际遇到：应用退出后没更新、也没重启，只留下一屏乱码报错）。
//
//  2. **不要用 timeout 等待**。timeout.exe 需要控制台输入句柄，而更新脚本是以
//     DETACHED_PROCESS（无控制台）启动的，`timeout /t 1` 会直接报
//     "ERROR: Input redirection is not supported"。改用 ping 做无控制台延时。
//
//  3. **等待必须有上限**（maxWaitRounds）。PID 会被系统复用：万一被杀掉等待的
//     进程号被新进程占用，无上限的循环会让更新永远不发生。
//
// 返回的 method 用于 ApplyResult.Method：nsis-silent / portable-swap。
//
// downloadPath / exePath 只用于判定分支（安装包 vs 便携版），不再写进脚本内容；
// 脚本通过 envUpdatePkg / envUpdateExe 读取它们，因此脚本主体是纯 ASCII。
func windowsUpdateScript(pid int, downloadPath, exePath string) (method, script string) {
	return windowsUpdateScriptLimited(pid, downloadPath, exePath, maxWaitRounds)
}

// windowsUpdateScriptLimited 与 windowsUpdateScript 相同，但可以指定等待轮数上限。
// 存在的意义是可测性：单测把上限压到几轮，既能验证"被杀进程还在时会等待"，
// 又不会因为 PID 恰好被复用而把测试拖住（曾把 CI 的 Windows job 拖到 11 分钟）。
func windowsUpdateScriptLimited(pid int, downloadPath, exePath string, maxRounds int) (method, script string) {
	if maxRounds <= 0 {
		maxRounds = maxWaitRounds
	}
	lines := []string{
		"@echo off",
		"setlocal",
		"set tries=0",
		":wait",
		// 用 PID 精确过滤，避免杀掉别的进程；find 只在有输出行时返回 0。
		fmt.Sprintf(`tasklist /FI "PID eq %d" /NH 2>nul | find "%d" >nul`, pid, pid),
		"if errorlevel 1 goto ready",
		// 上限（ping -n 2 ≈ 1 秒/轮）：PID 会被系统复用，一旦被等待的进程号被新进程
		// 占用，无上限的循环会让更新永远不发生——到点就照常往下走。
		"set /a tries+=1 >nul",
		fmt.Sprintf("if %%tries%% GEQ %d goto ready", maxRounds),
		"ping -n 2 127.0.0.1 >nul",
		"goto wait",
		":ready",
		// 两个路径都由启动方通过环境变量给出；缺失时直接结束，避免弹出无意义的报错
		fmt.Sprintf("if not defined %s goto done", envUpdatePkg),
		fmt.Sprintf("if not defined %s goto done", envUpdateExe),
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
