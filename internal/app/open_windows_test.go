//go:build windows

package app

import (
	"strings"
	"syscall"
	"testing"
)

// Windows 侧再钉一层：路径经 syscall.EscapeArg 之后，命令行里只能出现
// "包裹整个参数的引号"，**不能**出现转义出来的字面引号——explorer 解析的是分词后的参数，
// 拿到带字面引号的路径就认为路径非法，只会把资源管理器开在默认位置
// （v0.1.4 的真实回归：窗口能开但不是保存位置）。
func TestRevealEscapedCommandLineWindows(t *testing.T) {
	for _, path := range []string{
		`C:\Users\me\Documents\灵感合同\服务协议.docx`,
		`C:\My Documents\灵感合同\服务协议.docx`,
	} {
		arg := revealCommandFor("windows", path).args[0]
		escaped := syscall.EscapeArg(arg)

		if strings.Contains(escaped, `\"`) {
			t.Errorf("命令行出现转义字面引号，explorer 会解析不出路径：%q", escaped)
		}
		if got := strings.TrimSuffix(strings.TrimPrefix(escaped, `"`), `"`); got != arg {
			t.Errorf("去掉包裹引号后应等于原参数：%q → %q", escaped, got)
		}
		if strings.Contains(arg, " ") && !strings.HasPrefix(escaped, `"`) {
			t.Errorf("含空格的参数应被引号包裹：%q", escaped)
		}
	}
}
