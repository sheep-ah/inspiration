package updater

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// 行尾必须是 CRLF。cmd.exe 执行 goto 时按字节偏移重新定位脚本文件，LF-only 会让
// 它从行中间继续执行，出现 'el'/'tart'/'/nobreak' 这类碎片命令，更新静默失败
// （v0.1.0 的 Windows 用户真实遇到：应用退出后没更新、没重启，只剩一屏乱码）。
func TestWindowsUpdateScriptUsesCRLF(t *testing.T) {
	_, script := windowsUpdateScript(1234, `C:\dl\app_setup.exe`, `C:\app\灵感.exe`)

	if !strings.HasPrefix(script, "@echo off\r\n") {
		t.Errorf("脚本开头不是 CRLF 行：%q", firstN(script, 24))
	}
	if !strings.HasSuffix(script, "\r\n") {
		t.Errorf("脚本末尾缺少 CRLF：%q", lastN(script, 24))
	}
	if bad := regexp.MustCompile("[^\r]\n").FindString(script); bad != "" {
		t.Errorf("存在裸 LF 行尾（cmd.exe 的 goto 会错位）：%q", bad)
	}
	if strings.Contains(script, "\r\r\n") {
		t.Error("出现重复回车")
	}
	if strings.Contains(script, "\n\r") {
		t.Error("出现 LF+CR 反序行尾")
	}
}

// 无控制台（DETACHED_PROCESS）下不能用 timeout：它需要控制台输入句柄，
// 会直接报 "ERROR: Input redirection is not supported"，等待循环失效。
func TestWindowsUpdateScriptAvoidsConsoleOnlyTools(t *testing.T) {
	_, script := windowsUpdateScript(4321, `C:\dl\灵感_portable.exe`, `C:\app\灵感.exe`)

	if strings.Contains(script, "timeout") {
		t.Error("脚本使用了 timeout：无控制台时它会失败，应改用 ping 延时")
	}
	for _, want := range []string{
		":wait",
		"goto wait",
		"ping -n 2 127.0.0.1 >nul",
		`tasklist /FI "PID eq 4321" /NH 2>nul | find "4321" >nul`,
		"if errorlevel 1 goto ready",
		":ready",
		fmt.Sprintf("if %%tries%% GEQ %d goto ready", maxWaitRounds),
		`del "%~f0"`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("脚本缺少必要内容 %q：\n%s", want, script)
		}
	}
	// 等待必须有上限：PID 被复用会让无上限循环永不退出，更新永远不发生
	if maxWaitRounds <= 0 {
		t.Error("maxWaitRounds 必须为正数")
	}
}

func TestWindowsUpdateScriptBranches(t *testing.T) {
	setupPath := `C:\dl\inspiration_0.1.2_windows_amd64_setup.exe`
	exePath := `C:\app\灵感.exe`

	method, setupScript := windowsUpdateScript(1, setupPath, exePath)
	if method != "nsis-silent" {
		t.Errorf("安装包应走 nsis-silent，得到 %q", method)
	}
	if !strings.Contains(setupScript, `start "" /wait "C:\dl\inspiration_0.1.2_windows_amd64_setup.exe" /S`) {
		t.Errorf("安装包分支应静默安装并等待：\n%s", setupScript)
	}
	if strings.Contains(setupScript, "copy /y") {
		t.Error("安装包分支不应覆盖 exe")
	}
	if !strings.Contains(setupScript, `if exist "`+exePath+`" start "" "`+exePath+`"`) {
		t.Errorf("安装后应重启应用：\n%s", setupScript)
	}

	method, portableScript := windowsUpdateScript(2, `C:\dl\灵感.exe`, exePath)
	if method != "portable-swap" {
		t.Errorf("便携版应走 portable-swap，得到 %q", method)
	}
	if !strings.Contains(portableScript, `copy /y "C:\dl\灵感.exe" "`+exePath+`" >nul`) {
		t.Errorf("便携版分支应覆盖 exe：\n%s", portableScript)
	}
	if strings.Contains(portableScript, "/wait") {
		t.Error("便携版分支不应调用安装器")
	}
	// 自删必须是最后一行，否则 cmd 可能在脚本消失后继续读取而报错
	lines := strings.Split(strings.TrimRight(portableScript, "\r\n"), "\r\n")
	if last := lines[len(lines)-1]; last != `del "%~f0"` {
		t.Errorf("最后一行应为自删，实际是 %q", last)
	}
}

func TestIsSetupPackage(t *testing.T) {
	for _, name := range []string{
		"inspiration_0.1.2_windows_amd64_setup.exe",
		`C:\Users\me\Downloads\灵感_SETUP.EXE`,
	} {
		if !isSetupPackage(name) {
			t.Errorf("%q 应判定为安装包", name)
		}
	}
	for _, name := range []string{
		"inspiration_0.1.2_windows_amd64_portable.exe",
		`C:\dl\灵感.exe`,
		`C:\dl\setup.exe.bak`,
	} {
		if isSetupPackage(name) {
			t.Errorf("%q 不应判定为安装包", name)
		}
	}
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
