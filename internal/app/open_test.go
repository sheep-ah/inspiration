package app

import (
	"strings"
	"testing"
)

// Windows「打开所在文件夹」的两条硬约束（都来自真实回归）：
//
//  1. **不能抑制控制台窗口**。explorer 是 GUI 程序，STARTF_USESHOWWINDOW+SW_HIDE 会把
//     它要打开的那个窗口一起隐藏——用户看到的就是"点了没反应"（v0.1.1 引入、v0.1.4 修掉）。
//  2. **参数里不能带字面引号**。explorer 解析的是 Windows 分词之后的参数：文档里的
//     `explorer.exe /select,"C:\path"` 那对引号是给分词器的，explorer 实际拿到 `/select,C:\path`。
//     若参数里塞了字面引号，它拿到 `/select,"C:\path"`，路径非法 → 只把资源管理器开在默认
//     位置（v0.1.4 引入、v0.1.6 修掉的第二个回归：窗口能开、但不是保存位置）。
func TestRevealCommandWindows(t *testing.T) {
	path := `C:\Users\haoshiyang\Documents\灵感合同\服务协议.docx`
	c := revealCommandFor("windows", path)

	if c.name != "explorer.exe" {
		t.Errorf("应调用 explorer.exe，得到 %q", c.name)
	}
	want := "/select," + path
	if len(c.args) != 1 || c.args[0] != want {
		t.Errorf("参数应为 %q，得到 %q", want, c.args)
	}
	if strings.Contains(c.args[0], `"`) {
		t.Errorf("参数里不能出现字面引号（含空格路径的引号应由命令行转义负责）：%q", c.args[0])
	}
	if c.hideConsole {
		t.Error("explorer 绝不能隐藏窗口：SW_HIDE 会把要打开的窗口一起隐藏，表现为点了没反应")
	}
}

// 含空格的路径要原样放进参数，引号交给 exec 的命令行转义（分词后 explorer 才拿到干净路径）。
func TestRevealCommandWindowsKeepsSpacesForTheEscaper(t *testing.T) {
	c := revealCommandFor("windows", `C:\My Documents\a.docx`)
	if len(c.args) != 1 {
		t.Fatalf("参数个数异常：%q", c.args)
	}
	if c.args[0] != `/select,C:\My Documents\a.docx` {
		t.Errorf("含空格路径应原样放入参数：%q", c.args[0])
	}
}

// Windows「打开文件」仍要抑制 cmd.exe 的黑框（被隐藏的只是 cmd，目标程序正常显示）。
func TestOpenCommandWindows(t *testing.T) {
	path := `C:\Users\haoshiyang\Documents\灵感合同\服务协议.docx`
	c := openCommandFor("windows", path)

	if c.name != "cmd" {
		t.Errorf("应调用 cmd，得到 %q", c.name)
	}
	want := []string{"/c", "start", "", path}
	if len(c.args) != len(want) {
		t.Fatalf("参数个数应为 %d，得到 %q", len(want), c.args)
	}
	for i := range want {
		if c.args[i] != want[i] {
			t.Errorf("参数 %d 应为 %q，得到 %q", i, want[i], c.args[i])
		}
	}
	if !c.hideConsole {
		t.Error("cmd.exe 是控制台程序，必须抑制窗口，否则会闪一个黑框")
	}
}

func TestOpenAndRevealCommandsOnOtherPlatforms(t *testing.T) {
	if c := openCommandFor("darwin", "/tmp/a.docx"); c.name != "open" || len(c.args) != 1 || c.args[0] != "/tmp/a.docx" || c.hideConsole {
		t.Errorf("darwin 打开文件命令异常：%s %q hide=%v", c.name, c.args, c.hideConsole)
	}
	if c := revealCommandFor("darwin", "/tmp/a.docx"); c.name != "open" || len(c.args) != 2 || c.args[0] != "-R" || c.args[1] != "/tmp/a.docx" || c.hideConsole {
		t.Errorf("darwin 定位文件命令异常：%s %q hide=%v", c.name, c.args, c.hideConsole)
	}
	if c := openCommandFor("linux", "/tmp/a.docx"); c.name != "xdg-open" || len(c.args) != 1 || c.args[0] != "/tmp/a.docx" {
		t.Errorf("linux 打开文件命令异常：%s %q", c.name, c.args)
	}
	// linux 没有 /select 语义，退化为打开所在目录
	if c := revealCommandFor("linux", "/tmp/sub/a.docx"); c.name != "xdg-open" || len(c.args) != 1 || c.args[0] != "/tmp/sub" {
		t.Errorf("linux 定位文件命令异常：%s %q", c.name, c.args)
	}
}
