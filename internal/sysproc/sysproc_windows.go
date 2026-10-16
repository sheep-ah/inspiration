//go:build windows

package sysproc

import (
	"os/exec"
	"syscall"
)

// createNoWindow = CREATE_NO_WINDOW：控制台程序不分配控制台，因此根本不会有窗口。
const createNoWindow = 0x08000000

// HideWindow 让 cmd 在 Windows 上不出现控制台窗口。
//
// 同时设置两项，覆盖不同的工具行为：
//   - HideWindow(true) → STARTF_USESHOWWINDOW + SW_HIDE：窗口存在但不显示；
//   - CREATE_NO_WINDOW：控制台子系统程序干脆不创建控制台。
//
// 已有的 SysProcAttr 与 CreationFlags 会被保留（例如更新器的 DETACHED_PROCESS）。
func HideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
