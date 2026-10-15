//go:build windows

package updater

import "syscall"

// detachedProcess 让更新脚本脱离当前进程组，应用退出后脚本仍能继续执行。
func detachedProcess() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x00000008 | 0x00000200, // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
	}
}
