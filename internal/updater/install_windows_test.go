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

// 真实执行生成的批处理脚本，覆盖"应用还没退出 → 覆盖/静默安装 → 重启 → 自删"整条链路。
//
// 刻意把应用与更新包放在**含中文的目录**里：生产的安装目录
// （%LOCALAPPDATA%\Programs\灵感）与数据目录（%APPDATA%\灵感）都含中文，而 cmd.exe 按
// **控制台代码页**解码脚本字节——路径若写进脚本内容就会变成乱码，表现为"装完了不重启"
// 或"根本没覆盖"（v0.1.1 现场）。路径改为通过环境变量传递后必须仍然可用，本条用例守的就是它。
//
// 同时它也是 CRLF 的回归防护：LF 行尾会让 cmd 的 goto 错位、脚本碎片化执行，
// 覆盖与重启步骤会被跳过（v0.1.0 现场：'el'/'tart'/'nobreak' 不是内部或外部命令）。
func TestWindowsUpdateScriptExecutesEndToEnd(t *testing.T) {
	t.Run("便携版覆盖", func(t *testing.T) {
		f := newScriptFixture(t, "portable")
		writeTestFile(t, f.download, f.appContent("new"))
		writeTestFile(t, f.appPath, f.appContent("old"))

		runUpdateScriptE2E(t, f, "new")

		if got := readTestFile(t, f.appPath); got != f.appContent("new") {
			t.Errorf("应用没有被覆盖成新版本：%q", got)
		}
	})

	t.Run("安装包静默安装", func(t *testing.T) {
		f := newScriptFixture(t, "setup")
		// 用系统自带 ping.exe 冒充 NSIS 安装器：脚本会执行
		// `start "" /wait "%INSPIRATION_UPDATE_PKG%" /S`，能证明这条命令真的被执行到
		// （ping 收到未知参数会立即退出、不会挂住）。安装包分支不应覆盖 exe。
		copySystemExe(t, "ping.exe", f.download)
		old := f.appContent("old")
		writeTestFile(t, f.appPath, old)

		runUpdateScriptE2E(t, f, "old")

		if got := readTestFile(t, f.appPath); got != old {
			t.Errorf("安装包分支不应覆盖 exe：%q", got)
		}
	})
}

// scriptFixture 是执行用例的目录布局：工作目录保持纯 ASCII，应用/更新包放在含中文的子目录。
type scriptFixture struct {
	dir      string // ASCII 工作目录（脚本、输出、标记文件）
	appDir   string // 含中文的目录，模拟 %LOCALAPPDATA%\Programs\灵感
	appPath  string // 被更新/被重启的应用（.cmd 便于用标记文件观察）
	download string // "下载好的更新包"
	marker   string // 重启后的应用写出的标记文件（ASCII 路径，保证被测脚本内容纯 ASCII）
}

func newScriptFixture(t *testing.T, kind string) *scriptFixture {
	t.Helper()
	base, err := os.MkdirTemp("", "inspiration-e2e-")
	if err != nil {
		t.Fatalf("创建临时目录失败：%v", err)
	}
	// 子进程可能短暂占着目录里的文件，清理失败不影响结论
	t.Cleanup(func() { _ = os.RemoveAll(base) })

	appDir := filepath.Join(base, "灵感")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatalf("创建中文目录失败：%v", err)
	}

	f := &scriptFixture{
		dir:     base,
		appDir:  appDir,
		appPath: filepath.Join(appDir, "灵感.cmd"),
		marker:  filepath.Join(base, "restarted.txt"),
	}
	if kind == "setup" {
		f.download = filepath.Join(appDir, "inspiration_0.1.2_windows_amd64_setup.exe")
	} else {
		f.download = filepath.Join(appDir, "灵感_portable.exe")
	}
	return f
}

// appContent 是"应用"的内容：写出标记文件。内容保持纯 ASCII，
// 这样无论控制台代码页是什么，被重启的应用都能正常写标记（中文只出现在路径上）。
func (f *scriptFixture) appContent(marker string) string {
	return "@echo off\r\necho " + marker + ">\"" + f.marker + "\"\r\n"
}

func runUpdateScriptE2E(t *testing.T, f *scriptFixture, wantMarker string) {
	t.Helper()

	// 冒充"正在运行、约 2 秒后才退出"的应用
	holder := exec.Command("cmd", "/c", "ping -n 3 127.0.0.1")
	if err := holder.Start(); err != nil {
		t.Fatalf("无法启动持有进程：%v", err)
	}
	defer func() { _ = holder.Process.Kill(); _, _ = holder.Process.Wait() }()

	// 等待上限压到 4 轮（≈4 秒）：既能验证"持有进程还活着时会等待"，又保证即使
	// 这个 PID 之后恰好被复用，脚本也会很快继续——不让测试依赖 PID 复用的运气。
	const rounds = 4
	method, script := windowsUpdateScriptLimited(holder.Process.Pid, f.download, f.appPath, rounds)
	scriptPath := filepath.Join(f.dir, "update.cmd")
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatalf("写入脚本失败：%v", err)
	}

	// 关键：不要用 CombinedOutput/pipe 抓输出。脚本里 `start` 拉起的子进程会**继承管道
	// 句柄**，即使外层 cmd.exe 退出、甚至被 ctx kill，管道也不会 EOF，读端永远等下去。
	// 重定向到文件后，Wait 只看进程退出。
	outFile := filepath.Join(f.dir, "script-out.txt")
	out, err := os.Create(outFile)
	if err != nil {
		t.Fatalf("创建输出文件失败：%v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "cmd", "/c", scriptPath)
	// 路径走环境变量（脚本内容保持纯 ASCII）：这里应用与更新包都在中文目录下，
	// 验证中文路径经环境变量传递后仍能被 if exist / copy / start / start /wait 正确处理。
	command.Env = append(os.Environ(),
		envUpdatePkg+"="+f.download,
		envUpdateExe+"="+f.appPath,
	)
	command.Stdout = out
	command.Stderr = out
	command.WaitDelay = 15 * time.Second // 进程不退出时的兜底：先 Kill 再等一小会儿

	started := time.Now()
	runErr := command.Run()
	elapsed := time.Since(started)
	_ = out.Close()
	scriptOutput, _ := os.ReadFile(outFile)
	if errors.Is(runErr, exec.ErrWaitDelay) {
		runErr = nil // 进程已退出，只是仍有子进程占着 I/O 句柄
	}
	if ctx.Err() != nil {
		t.Fatalf("脚本执行超过 90 秒未结束（疑似等待循环没有退出）\n脚本：\n%s\n输出：%s", script, scriptOutput)
	}
	if runErr != nil {
		t.Fatalf("脚本执行失败（method=%s）：%v\n输出：%s\n脚本：\n%s", method, runErr, scriptOutput, script)
	}
	for _, bad := range []string{"不是内部或外部命令", "找不到批处理文件", "语法不正确", "系统找不到指定的路径"} {
		if strings.Contains(string(scriptOutput), bad) {
			t.Fatalf("脚本出现碎片/错误命令（命中 %q）：\n%s\n脚本：\n%s", bad, scriptOutput, script)
		}
	}

	// 重启步骤会启动应用，应用写出标记文件。
	// 注意：不能只等"文件存在"——cmd.exe 是先创建重定向目标再写入内容，
	// 存在但还没写完时会读到空串（CI#33 就是这么偶发失败的）。等到内容非空为止。
	deadline := time.Now().Add(20 * time.Second)
	var got string
	for {
		if data, err := os.ReadFile(f.marker); err == nil {
			if got = strings.TrimSpace(string(data)); got != "" {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("应用没有被重新启动或标记为空（%s，读到 %q）\n输出：%s\n脚本：\n%s",
				f.marker, got, scriptOutput, script)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if got != wantMarker {
		t.Errorf("重启的应用不是期望的版本：标记为 %q，期望 %q", got, wantMarker)
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
