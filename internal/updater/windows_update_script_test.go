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
	_, script := windowsUpdateScript(`C:\dl\app_setup.exe`, `C:\app\灵感.exe`)

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

// 脚本必须是纯 ASCII：cmd.exe 按控制台代码页解码脚本字节，中文路径（安装目录含「灵感」）
// 写进 .cmd 会被解成乱码，导致 if exist / copy 指向不存在的路径。路径一律走环境变量。
func TestWindowsUpdateScriptIsASCIIOnly(t *testing.T) {
	_, script := windowsUpdateScript(`C:\dl\inspiration_0.1.7_windows_amd64_setup.exe`, `C:\Program Files\灵感\灵感.exe`)
	for i, r := range script {
		if r > 0x7F {
			t.Fatalf("脚本第 %d 字节起出现非 ASCII 字符 %q；中文路径必须通过环境变量传入：\n%s", i, r, script)
		}
	}
	for _, want := range []string{"INSPIRATION_UPDATE_PKG", "INSPIRATION_UPDATE_EXE", "if not defined INSPIRATION_UPDATE_PKG goto done"} {
		if !strings.Contains(script, want) {
			t.Errorf("脚本缺少环境变量引用 %q：\n%s", want, script)
		}
	}
}

// 等待"应用退出"必须用"能不能写 exe"来判断，**不能用 tasklist/find 匹配 PID**：
// PID 会被系统复用，一旦被复用循环就永不退出——现场是一个卡住的 `find "45152"` 窗口
// （v0.1.2~v0.1.6 的真实故障）。同理不能用需要控制台的 timeout。
func TestWindowsUpdateScriptWaitsOnExeLockOnly(t *testing.T) {
	_, script := windowsUpdateScript(`C:\dl\灵感_portable.exe`, `C:\app\灵感.exe`)

	for _, banned := range []string{"tasklist", "find ", "timeout"} {
		if strings.Contains(script, banned) {
			t.Errorf("脚本不该使用 %q（PID 复用会卡死、timeout 需要控制台）：\n%s", banned, script)
		}
	}
	for _, want := range []string{
		`:wait`,
		`2>nul >>"%INSPIRATION_UPDATE_EXE%" echo.`,
		"if not errorlevel 1 goto ready",
		fmt.Sprintf("if %%tries%% GEQ %d goto done", maxWaitRounds),
		"ping -n 2 127.0.0.1 >nul",
		"goto wait",
		":ready",
		":done",
		`(goto) 2>nul & del "%~f0"`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("脚本缺少必要内容 %q：\n%s", want, script)
		}
	}
	// 等待必须有上限：exe 长期不可写（例如装在只读目录）时也不能无限等
	if maxWaitRounds <= 0 {
		t.Error("maxWaitRounds 必须为正数")
	}
}

func TestWindowsUpdateScriptBranches(t *testing.T) {
	setupPath := `C:\dl\inspiration_0.1.7_windows_amd64_setup.exe`
	exePath := `C:\app\灵感.exe`

	method, setupScript := windowsUpdateScript(setupPath, exePath)
	if method != "nsis-silent" {
		t.Errorf("安装包应走 nsis-silent，得到 %q", method)
	}
	if !strings.Contains(setupScript, `start "" /wait "%INSPIRATION_UPDATE_PKG%" /S`) {
		t.Errorf("安装包分支应静默安装并等待：\n%s", setupScript)
	}
	if strings.Contains(setupScript, "copy /y") {
		t.Error("安装包分支不应覆盖 exe")
	}
	if !strings.Contains(setupScript, `if exist "%INSPIRATION_UPDATE_EXE%" start "" "%INSPIRATION_UPDATE_EXE%"`) {
		t.Errorf("安装后应重启应用：\n%s", setupScript)
	}

	method, portableScript := windowsUpdateScript(`C:\dl\灵感.exe`, exePath)
	if method != "portable-swap" {
		t.Errorf("便携版应走 portable-swap，得到 %q", method)
	}
	if !strings.Contains(portableScript, `copy /y "%INSPIRATION_UPDATE_PKG%" "%INSPIRATION_UPDATE_EXE%" >nul`) {
		t.Errorf("便携版分支应覆盖 exe：\n%s", portableScript)
	}
	if strings.Contains(portableScript, "/wait") {
		t.Error("便携版分支不应调用安装器")
	}
	// 自删必须是最后一行，否则 cmd 可能在脚本消失后继续读取而报错
	lines := strings.Split(strings.TrimRight(portableScript, "\r\n"), "\r\n")
	if last := lines[len(lines)-1]; last != `(goto) 2>nul & del "%~f0"` {
		t.Errorf("最后一行应为自删，实际是 %q", last)
	}
}

func TestWindowsUpdateScriptRespectsRoundLimit(t *testing.T) {
	if _, s := windowsUpdateScriptLimited(`C:\dl\a.exe`, `C:\app\b.exe`, 7); !strings.Contains(s, "if %tries% GEQ 7 goto done") {
		t.Errorf("自定义等待轮数未生效：\n%s", s)
	}
	// 非法值回落到默认上限：0 轮等于完全不等待，会把"应用还没退出"的情况变成安装失败
	for _, bad := range []int{0, -1} {
		if _, s := windowsUpdateScriptLimited(`C:\dl\a.exe`, `C:\app\b.exe`, bad); !strings.Contains(s, fmt.Sprintf("if %%tries%% GEQ %d goto done", maxWaitRounds)) {
			t.Errorf("轮数 %d 未回落到默认上限 %d：\n%s", bad, maxWaitRounds, s)
		}
	}
}

func TestIsSetupPackage(t *testing.T) {
	for _, name := range []string{
		"inspiration_0.1.7_windows_amd64_setup.exe",
		`C:\Users\me\Downloads\灵感_SETUP.EXE`,
	} {
		if !isSetupPackage(name) {
			t.Errorf("%q 应判定为安装包", name)
		}
	}
	for _, name := range []string{
		"inspiration_0.1.7_windows_amd64_portable.exe",
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
