//go:build windows

package sysproc

import (
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

// 这些用例只在 Windows 上编译并运行：CI 的 windows 构建 job 会执行本包测试。
// 本地 macOS 无法验证"窗口是否真的没弹出来"，因此这里锁定机制本身——只要
// HideWindow 没被调用或标志被覆盖，控制台窗口就会再次出现。

func TestHideWindowSetsConsoleSuppression(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "exit", "0")
	HideWindow(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr 未设置：子进程会弹出控制台窗口")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Error("HideWindow 未开启：子进程会弹出控制台窗口")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Errorf("缺少 CREATE_NO_WINDOW：CreationFlags=%#x", cmd.SysProcAttr.CreationFlags)
	}
}

func TestHideWindowKeepsExistingFlags(t *testing.T) {
	const createNewProcessGroup = 0x00000200
	cmd := exec.Command("cmd", "/c", "exit", "0")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup}
	HideWindow(cmd)
	if cmd.SysProcAttr.CreationFlags&createNewProcessGroup == 0 {
		t.Error("已有的 CreationFlags 被覆盖了")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Error("CREATE_NO_WINDOW 缺失")
	}
}

// 加了抑制标志后子进程仍必须能正常执行并回传输出（PowerShell 转换通道依赖这一点）。
func TestHideWindowStillRunsAndCapturesOutput(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "echo", "inspiration-silent-ok")
	HideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("子进程执行失败：%v", err)
	}
	if !strings.Contains(string(out), "inspiration-silent-ok") {
		t.Errorf("未取到子进程输出：%q", out)
	}
}
