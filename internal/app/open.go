package app

import (
	"fmt"
	"os/exec"
	"runtime"
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
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("无法定位文件 %s：%w", path, err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
