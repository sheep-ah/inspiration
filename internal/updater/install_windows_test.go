//go:build windows

package updater

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 真实执行生成的批处理脚本（把"应用还没退出 → 安装/覆盖 → 重启 → 自删"整条链路跑一遍）：
//
//   - 用一个活着的进程冒充"还没退出的应用"，断言脚本真的等它退出之后才动手；
//   - 断言覆盖/安装步骤执行了、应用被重新启动、脚本自删；
//   - 断言输出里没有碎片命令（LF 行尾导致的 goto 错位正是 v0.1.0 的现场：
//     'el'/'tart'/'/nobreak' 不是内部或外部命令）。
func TestWindowsUpdateScriptExecutesEndToEnd(t *testing.T) {
	t.Run("便携版覆盖", func(t *testing.T) {
		dir := t.TempDir()
		marker := filepath.Join(dir, "restarted.txt")
		download := filepath.Join(dir, "inspiration_0.1.2_windows_amd64_portable.exe")
		appPath := filepath.Join(dir, "灵感.cmd")

		newApp := "@echo off\r\necho new>\"" + marker + "\"\r\n"
		writeTestFile(t, download, newApp)
		writeTestFile(t, appPath, "@echo off\r\necho old>\""+marker+"\"\r\n")

		runUpdateScriptE2E(t, dir, download, appPath, marker, "new")

		if got := readTestFile(t, appPath); got != newApp {
			t.Errorf("应用没有被覆盖成新版本：%q", got)
		}
	})

	t.Run("安装包静默安装", func(t *testing.T) {
		dir := t.TempDir()
		marker := filepath.Join(dir, "restarted.txt")
		// 用系统自带 ping.exe 冒充 NSIS 安装器：脚本会执行
		// `start "" /wait "<它>" /S`，能证明这条命令真的被执行到（ping 收到未知参数会
		// 立即退出、不会挂住）。安装包分支不应覆盖 exe。
		setup := filepath.Join(dir, "inspiration_0.1.2_windows_amd64_setup.exe")
		copySystemExe(t, "ping.exe", setup)
		appPath := filepath.Join(dir, "灵感.cmd")
		old := "@echo off\r\necho old>\"" + marker + "\"\r\n"
		writeTestFile(t, appPath, old)

		runUpdateScriptE2E(t, dir, setup, appPath, marker, "old")

		if got := readTestFile(t, appPath); got != old {
			t.Errorf("安装包分支不应覆盖 exe：%q", got)
		}
	})
}

func runUpdateScriptE2E(t *testing.T, dir, download, appPath, marker, wantMarker string) {
	t.Helper()

	// 冒充"正在运行、约 2 秒后退出"的应用
	holder := exec.Command("cmd", "/c", "ping -n 3 127.0.0.1")
	if err := holder.Start(); err != nil {
		t.Fatalf("无法启动持有进程：%v", err)
	}
	defer func() { _ = holder.Process.Kill(); _, _ = holder.Process.Wait() }()

	// 等待上限压到 4 轮（≈4 秒）：既能验证"持有进程还活着时会等待"，又保证即使
	// 这个 PID 之后恰好被复用，脚本也会很快继续——不让测试依赖 PID 复用的运气。
	const rounds = 4
	method, script := windowsUpdateScriptLimited(holder.Process.Pid, download, appPath, rounds)
	scriptPath := filepath.Join(dir, "update.cmd")
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatalf("写入脚本失败：%v", err)
	}

	started := time.Now()
	// 关键：不要用 CombinedOutput/pipe 抓输出。脚本里 `start` 拉起的子进程会**继承管道
	// 句柄**，即使外层 cmd.exe 退出、甚至被 ctx kill，管道也不会 EOF，读端永远等下去
	// （这正是 CI 里这个用例卡住 20+ 分钟的原因）。改成重定向到文件：Wait 只看进程退出。
	outFile := filepath.Join(dir, "script-out.txt")
	f, err := os.Create(outFile)
	if err != nil {
		t.Fatalf("创建输出文件失败：%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "cmd", "/c", scriptPath)
	command.Stdout = f
	command.Stderr = f
	command.WaitDelay = 15 * time.Second // 进程不退出时的兜底：先 Kill 再等一小会儿
	runErr := command.Run()
	_ = f.Close()
	elapsed := time.Since(started)
	out, _ := os.ReadFile(outFile)
	if errors.Is(runErr, exec.ErrWaitDelay) {
		runErr = nil // 进程已退出，只是仍有子进程占着 I/O 句柄
	}
	if ctx.Err() != nil {
		t.Fatalf("脚本执行超过 90 秒未结束（疑似等待循环没有退出）\n脚本：\n%s\n输出：%s", script, out)
	}
	if runErr != nil {
		t.Fatalf("脚本执行失败（method=%s）：%v\n输出：%s\n脚本：\n%s", method, runErr, out, script)
	}
	for _, bad := range []string{"不是内部或外部命令", "找不到批处理文件", "语法不正确", "系统找不到指定的路径"} {
		if strings.Contains(string(out), bad) {
			t.Fatalf("脚本出现碎片/错误命令（命中 %q）：\n%s\n脚本：\n%s", bad, out, script)
		}
	}

	// 重启步骤会启动应用，应用写出标记文件
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("应用没有被重新启动（缺少 %s）\n输出：%s", marker, out)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if got := strings.TrimSpace(readTestFile(t, marker)); got != wantMarker {
		t.Errorf("重启的应用不是更新后的版本：标记为 %q，期望 %q", got, wantMarker)
	}

	// 必须等到持有进程退出（即至少循环一次）才继续
	if elapsed < 800*time.Millisecond {
		t.Errorf("脚本没有等待持有进程退出（耗时 %v）", elapsed)
	}
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Errorf("脚本没有自删（err=%v）", err)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入 %s 失败：%v", path, err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败：%v", path, err)
	}
	return string(data)
}

func copySystemExe(t *testing.T, name, dst string) {
	t.Helper()
	src := filepath.Join(os.Getenv("SystemRoot"), "System32", name)
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("读取 %s 失败：%v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatalf("写入 %s 失败：%v", dst, err)
	}
}
