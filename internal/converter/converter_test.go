package converter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeConverter 是一个"假装成功"的转换器：把输入原样写成输出。
// 用来测试错误路径（产物不是 .doc 时必须判定失败）。
func fakeConverter(t *testing.T, dir string) string {
	t.Helper()
	script := filepath.Join(dir, "fake-soffice.sh")
	body := `#!/bin/bash
# 参数形如: --headless --norestore -env:... --convert-to <filter> --outdir <dir> <src>
outdir=""
src=""
filter=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --outdir) outdir="$2"; shift 2;;
    --convert-to) filter="$2"; shift 2;;
    --*) shift;;
    *) src="$1"; shift;;
  esac
done
base="$(basename "$src")"
ext=".docx"
[[ "$filter" == doc* ]] && ext=".doc"
cp "$src" "$outdir/${base%.*}$ext"
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

func TestDetectWithoutConverter(t *testing.T) {
	t.Setenv(EnvSoffice, filepath.Join(t.TempDir(), "不存在的转换器"))
	// 环境变量指向不存在的文件时，LibreOffice 通道不可用（且不会静默回退到系统里的其它 LibreOffice）
	c := New(t.TempDir())
	status := c.Detect()
	var lo ChannelStatus
	for _, ch := range status.Channels {
		if ch.Kind == KindLibreOffice {
			lo = ch
		}
	}
	if lo.Available {
		t.Fatalf("指向不存在的路径时 LibreOffice 通道应不可用: %+v", lo)
	}
	if !strings.Contains(lo.Detail, "不存在") {
		t.Errorf("通道详情应说明原因，实际: %s", lo.Detail)
	}
}

func TestChannelPriorityPrefersWordThenWPS(t *testing.T) {
	// 本机（macOS 无 Word）应退回 LibreOffice，并把它标为实际使用的通道
	fake := fakeConverter(t, t.TempDir())
	t.Setenv(EnvSoffice, fake)
	c := New(t.TempDir())
	status := c.Detect()
	if !status.Available {
		t.Fatalf("应至少有一个可用通道: %+v", status)
	}
	if status.Kind != KindLibreOffice {
		t.Errorf("无 Word/WPS 时应使用 LibreOffice，实际: %s", status.Kind)
	}
	if len(status.Channels) != 3 {
		t.Fatalf("应报告 3 个通道，实际 %d", len(status.Channels))
	}
	if status.Channels[0].Kind != KindWord || status.Channels[2].Kind != KindLibreOffice {
		t.Errorf("通道顺序应为 word → wps → libreoffice，实际: %+v", status.Channels)
	}
	// 强制指定通道
	c.forceKind = KindWord
	forced := c.Detect()
	if forced.Available {
		t.Error("本机没有 Word，强制 word 通道时不应可用")
	}
}

func TestDetectWithEnvOverride(t *testing.T) {
	fake := fakeConverter(t, t.TempDir())
	t.Setenv(EnvSoffice, fake)
	c := New(t.TempDir())
	status := c.Detect()
	if !status.Available {
		t.Fatalf("指定存在的转换器时应可用: %+v", status)
	}
	if status.Path != fake {
		t.Errorf("应使用指定的转换器: %s", status.Path)
	}
}

func TestToDocRejectsNonDocOutput(t *testing.T) {
	dir := t.TempDir()
	fake := fakeConverter(t, dir)
	src := writeDocxStub(t, dir, "原始内容")

	dst := filepath.Join(dir, "out.doc")
	c := NewWithBinary(filepath.Join(dir, "profile"), fake)
	err := c.ToDoc(context.Background(), src, dst)
	if err == nil {
		t.Fatal("产物不是 OLE2 .doc 时必须判定失败")
	}
	if !strings.Contains(err.Error(), ".doc") {
		t.Errorf("错误信息应说明产物不合法，实际: %v", err)
	}
	if _, statErr := os.Stat(dst); statErr == nil {
		t.Error("自检失败时应删除产物，避免用户拿到损坏文件")
	}
}

func TestFileURL(t *testing.T) {
	cases := map[string]string{
		"/tmp/lo-profile":    "file:///tmp/lo-profile",
		"/var/folders/x/T/p": "file:///var/folders/x/T/p",
		// 含空格与非 ASCII：应用数据目录的真实形态
		"/Users/x/Library/Application Support/灵感/convert": "file:///Users/x/Library/Application%20Support/%E7%81%B5%E6%84%9F/convert",
	}
	for in, want := range cases {
		if got := fileURL(in); got != want {
			t.Errorf("fileURL(%q) = %q，期望 %q", in, got, want)
		}
	}
	// Windows 路径（在非 Windows 上只验证格式化规则）
	if got := fileURL(filepath.FromSlash("C:/Users/x/AppData/p")); !strings.HasPrefix(got, "file:///") && !strings.HasPrefix(got, "file://") {
		t.Errorf("Windows 路径应生成带盘符的 file URL，实际 %q", got)
	}
}

func TestMissingLinesIgnoresDuplicatesAndWhitespace(t *testing.T) {
	original := "  甲方：张三  \n\n乙方：李四\n"
	roundTrip := "甲方：张三\n甲方：张三\n乙方：李四\n"
	if missing := missingLines(original, roundTrip); len(missing) != 0 {
		t.Errorf("重复行与空白差异不应判为丢失: %v", missing)
	}
	lost := "甲方：张三\n" // 缺了"乙方：李四"
	if missing := missingLines(original, lost); len(missing) != 1 || missing[0] != "乙方：李四" {
		t.Errorf("应检测出丢失的行，实际: %v", missing)
	}
}

// writeDocxStub 写一个"内容随意"的文件，仅用于测试错误路径。
func writeDocxStub(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "src.docx")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
