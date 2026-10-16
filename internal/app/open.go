package app

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/sheep-ah/inspiration/internal/sysproc"
)

// openPath 用系统默认程序打开文件或目录。
func openPath(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	// Windows 上 cmd.exe 是控制台程序：不抑制就会闪一个黑框
	// （start 打开的目标程序本身不受影响，仍正常显示）。
	sysproc.HideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("无法打开 %s：%w", path, err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// revealPath 在系统文件管理器中定位文件。
func revealPath(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", "-R", path)
	case "windows":
		cmd = exec.Command("explorer", "/select,"+path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	sysproc.HideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("无法定位文件 %s：%w", path, err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
