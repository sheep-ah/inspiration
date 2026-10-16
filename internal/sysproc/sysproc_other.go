//go:build !windows

package sysproc

import "os/exec"

// HideWindow 在非 Windows 平台无需处理：macOS/Linux 启动子进程不会额外弹窗口。
func HideWindow(*exec.Cmd) {}
