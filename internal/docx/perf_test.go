package docx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 阶段 3：性能基准。目标——给出可复现的数字，并在超大文档上设阈值防止回归。

// buildLargeFixture 构造一个"大文档"：paragraphs 个含占位符的段落 + 1 个 rows 行表格 + 页眉页脚。
func buildLargeFixture(t *testing.T, paragraphs, rows int) string {
	t.Helper()
	var body strings.Builder
	for i := 0; i < paragraphs; i++ {
		fmt.Fprintf(&body, `<w:p><w:r><w:t xml:space="preserve">第 %d 条：联系人 {{联系人姓名}}，电话 {{电话}}，账户 {{账户号}}，地址 {{实际经营地址}}。</w:t></w:r></w:p>`, i+1)
	}
	body.WriteString(`<w:tbl>`)
	for i := 0; i < rows; i++ {
		body.WriteString(`<w:tr>`)
		for c := 0; c < 4; c++ {
			cell := "单元格内容"
			if c%2 == 1 {
				cell = "{{营业执照名称}}"
			}
			fmt.Fprintf(&body, `<w:tc><w:p><w:r><w:t xml:space="preserve">%s</w:t></w:r></w:p></w:tc>`, cell)
		}
		body.WriteString(`</w:tr>`)
	}
	body.WriteString(`</w:tbl>`)
	return buildDocxFixture(t, docxFixture{
		body:   body.String(),
		header: map[string]string{"word/header1.xml": p("页眉 {{联系人邮箱}}")},
		footer: map[string]string{"word/footer1.xml": p("页脚 {{开票地址}}")},
	})
}

func TestLargeTemplatePerformance(t *testing.T) {
	const paragraphs, rows = 2000, 200
	fixture := buildLargeFixture(t, paragraphs, rows)
	info, err := os.Stat(fixture)
	if err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "large.docx")
	start := time.Now()
	insp, err := Inspect(fixture, supportedForTest)
	inspectDur := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	start = time.Now()
	if err := Render(fixture, out, testValues); err != nil {
		t.Fatal(err)
	}
	renderDur := time.Since(start)
	outInfo, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("大文档：%d 段 + %d 行表格（.docx %.0f KB）｜Inspect %v｜Render %v｜产物 %.0f KB",
		paragraphs, rows, float64(info.Size())/1024, inspectDur, renderDur, float64(outInfo.Size())/1024)

	if insp.FieldUsage["联系人姓名"] != paragraphs {
		t.Errorf("联系人姓名 出现次数 = %d，期望 %d", insp.FieldUsage["联系人姓名"], paragraphs)
	}
	// 阈值放得较宽（CI 机器性能不一），只防止数量级退化
	if inspectDur > 5*time.Second {
		t.Errorf("解析耗时 %v 超过 5s 阈值", inspectDur)
	}
	if renderDur > 5*time.Second {
		t.Errorf("生成耗时 %v 超过 5s 阈值", renderDur)
	}
	// 产物仍需无残留占位符
	if insp, err := Inspect(out, supportedForTest); err != nil {
		t.Fatal(err)
	} else if len(insp.Placeholders()) != 0 {
		t.Errorf("产物仍有 %d 个占位符", len(insp.Placeholders()))
	}
}

func BenchmarkInspectSmallTemplate(b *testing.B) {
	t := &testing.T{}
	fixture := buildLargeFixture(t, 20, 5)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Inspect(fixture, supportedForTest); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRenderSmallTemplate(b *testing.B) {
	t := &testing.T{}
	fixture := buildLargeFixture(t, 20, 5)
	out := filepath.Join(b.TempDir(), "out.docx")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Render(fixture, out, testValues); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkInspectLargeTemplate(b *testing.B) {
	t := &testing.T{}
	fixture := buildLargeFixture(t, 2000, 200)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Inspect(fixture, supportedForTest); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRenderLargeTemplate(b *testing.B) {
	t := &testing.T{}
	fixture := buildLargeFixture(t, 2000, 200)
	out := filepath.Join(b.TempDir(), "out.docx")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Render(fixture, out, testValues); err != nil {
			b.Fatal(err)
		}
	}
}
