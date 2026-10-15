package app

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// ShowStartupError 在图形界面里弹出启动失败提示。
// GUI 应用没有控制台，启动期错误如果只写 stderr，用户会看到"点了没反应"。
func ShowStartupError(title, message string) {
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf(`display alert %q message %q as critical`, title, message)
		_ = exec.Command("osascript", "-e", script).Run()
	case "windows":
		ps := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.MessageBox]::Show(%q, %q, 'OK', 'Error') | Out-Null`,
			message, title)
		_ = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps).Run()
	}
}

// StartupErrorHint 针对常见启动失败给出可操作的建议。
func StartupErrorHint(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "database") || strings.Contains(msg, "SQLite") || strings.Contains(msg, "数据库"):
		return "\n\n数据文件可能已损坏或不可写。\n" +
			"1) 确认磁盘空间充足、数据目录可写；\n" +
			"2) 如果确认数据文件损坏：把它重命名（如 inspiration.db.bak）后重新启动应用，会重建一个空数据库；\n" +
			"3) 损坏的数据库不要删除，其中可能仍能抢救出模板信息。"
	case strings.Contains(msg, "无法定位用户配置目录") || strings.Contains(msg, "创建目录失败"):
		return "\n\n请确认用户主目录可写、磁盘空间充足。"
	default:
		return ""
	}
}
