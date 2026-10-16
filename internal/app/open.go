package app

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/sheep-ah/inspiration/internal/sysproc"
)

// openCommand 描述"用哪个命令打开/定位某个路径"。
// 单独抽成纯函数是为了能在任意平台上做单测：Windows 的两条约束（控制台窗口抑制、
// /select 的引号）都只体现在这里，用 macOS 就能回归验证，不必等真机。
type openCommand struct {
	name string
	args []string
	// hideConsole 只对"控制台子程序"（cmd.exe）为 true。
	// 对 GUI 程序（explorer.exe）绝不能用：STARTF_USESHOWWINDOW + SW_HIDE 会把
	// explorer 要打开的那个窗口一起隐藏，表现就是"点了没反应"（v0.1.1 引入过的真实回归）。
	hideConsole bool
}

// openCommandFor 返回"用默认程序打开文件/目录"的命令。
func openCommandFor(goos, path string) openCommand {
	switch goos {
	case "darwin":
		return openCommand{name: "open", args: []string{path}}
	case "windows":
		// start 打开的目标程序会拿到正常的显示状态；被隐藏的只是 cmd.exe 自己
		return openCommand{name: "cmd", args: []string{"/c", "start", "", path}, hideConsole: true}
	default:
		return openCommand{name: "xdg-open", args: []string{path}}
	}
}

// revealCommandFor 返回"在文件管理器中定位文件"的命令。
func revealCommandFor(goos, path string) openCommand {
	switch goos {
	case "darwin":
		return openCommand{name: "open", args: []string{"-R", path}}
	case "windows":
		// /select 要求路径带引号，否则含空格的路径会解析不出来；
		// 且不能设 hideConsole（见 openCommand 注释）。
		return openCommand{name: "explorer", args: []string{`/select,"` + path + `"`}}
	default:
		return openCommand{name: "xdg-open", args: []string{filepath.Dir(path)}}
	}
}

// openPath 用系统默认程序打开文件或目录。
func openPath(path string) error {
	if err := runOpenCommand(openCommandFor(runtime.GOOS, path)); err != nil {
		return fmt.Errorf("无法打开 %s：%w", path, err)
	}
	return nil
}

// revealPath 在系统文件管理器中定位文件。
func revealPath(path string) error {
	if err := runOpenCommand(revealCommandFor(runtime.GOOS, path)); err != nil {
		// 兜底：定位不了就退化为"打开所在目录"，避免用户点了完全没反应
		if fallbackErr := runOpenCommand(openCommandFor(runtime.GOOS, filepath.Dir(path))); fallbackErr != nil {
			return fmt.Errorf("无法定位文件 %s：%w", path, err)
		}
	}
	return nil
}

func runOpenCommand(c openCommand) error {
	cmd := exec.Command(c.name, c.args...)
	if c.hideConsole {
		sysproc.HideWindow(cmd)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
