package docx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const wmlNSForTest = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

var supportedForTest = []string{
	"联系人姓名", "法人代表", "实际经营地址", "联系人邮箱", "营业执照名称",
	"纳税人识别号", "电话", "联系人电话", "开户银行", "账户号", "开票地址",
}

var testValues = map[string]string{
	"联系人姓名":  "张三 & 李四",
	"法人代表":   "李四<测试>",
	"实际经营地址": "北京市海淀区中关村大街1号院3号楼5层501室（临街商铺，靠近地铁站A口，方位说明很长的中文地址文本）",
	"联系人邮箱":  "zhangsan@example.com",
	"营业执照名称": "示例科技（北京）有限公司",
	"纳税人识别号": "91110108MA01ABCD2X",
	"电话":     "010-88886666",
	"联系人电话":  "13800000000",
	"开户银行":   "中国工商银行股份有限公司北京中关村支行",
	"账户号":    "0001234567890123456",
	"开票地址":   "上海市浦东新区世纪大道100号A座20层",
}

// 语料文件说明见 poc/docx-eval/README.md 与 docs/phase1/docx-evaluation.md。
const (
	tplSimple     = "testdata/template_simple.docx"
	tplStructures = "testdata/template_structures.docx"
	tplSplitRuns  = "testdata/template_split_runs.docx"
	tplUnknown    = "testdata/template_unknown.docx"
	tplUnclosed   = "testdata/template_unclosed.docx"
	tplUnsafe     = "testdata/template_unsafe.docx"
	tplBrokenZip  = "testdata/template_broken_zip.docx"
	tplBrokenXML  = "testdata/template_broken_xml.docx"
)

func TestInspectCountsPlaceholdersAcrossAllParts(t *testing.T) {
	insp, err := Inspect(tplStructures, supportedForTest)
	if err != nil {
		t.Fatalf("Inspect 失败: %v", err)
	}
	if len(insp.Placeholders()) != 23 {
		t.Fatalf("占位符数量 = %d，期望 23", len(insp.Placeholders()))
	}
	if problems := insp.Problems(); len(problems) != 0 {
		t.Fatalf("合格模板不应有问题: %v", problems)
	}
	// 正文 + 表格 + 页眉 + 页脚 + 脚注 + VML 文本框 + DrawingML 文本框 + SDT
	for _, part := range []string{"word/document.xml", "word/header1.xml", "word/footer1.xml", "word/footnotes.xml"} {
		if !contains(insp.Parts, part) {
			t.Errorf("未扫描到部件 %s，实际部件: %v", part, insp.Parts)
		}
	}
	// 期望值按语料结构人工清点：正文(10) + 重复占位符(2) + 表格(2) + DrawingML 文本框(2) +
	// SDT(1) + VML 文本框(1) = 18（document.xml）；页眉(2) + 页脚(2) + 脚注(1) = 5，共 23。
	want := map[string]int{
		"联系人姓名": 6, "营业执照名称": 2, "法人代表": 2, "联系人邮箱": 2,
		"账户号": 2, "开票地址": 2, "电话": 4,
		"实际经营地址": 1, "纳税人识别号": 1, "开户银行": 1,
	}
	for field, n := range want {
		if insp.FieldUsage[field] != n {
			t.Errorf("字段 %s 出现次数 = %d，期望 %d", field, insp.FieldUsage[field], n)
		}
	}
}

func TestInspectSplitRunTemplateMatchesUnsplit(t *testing.T) {
	a, err := Inspect(tplStructures, supportedForTest)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Inspect(tplSplitRuns, supportedForTest)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Placeholders()) != len(b.Placeholders()) {
		t.Fatalf("跨 run 模板占位符数量 %d != 未拆分模板 %d", len(b.Placeholders()), len(a.Placeholders()))
	}
	for k, v := range a.FieldUsage {
		if b.FieldUsage[k] != v {
			t.Errorf("字段 %s: 跨 run 模板 %d != 未拆分模板 %d", k, b.FieldUsage[k], v)
		}
	}
}

func TestRenderReplacesEveryOccurrenceWithRealTemplate(t *testing.T) {
	// 每个模板只断言它真正用到的字段，且逐部件核对，避免"整篇搜索碰巧命中"。
	partValues := map[string]map[string][]string{
		tplSimple: {
			"word/document.xml": {"联系人姓名", "联系人邮箱", "营业执照名称"},
		},
		tplStructures: {
			"word/document.xml":  {"联系人姓名", "法人代表", "实际经营地址", "营业执照名称", "纳税人识别号", "联系人邮箱", "电话", "开户银行", "账户号", "开票地址"},
			"word/header1.xml":   {"营业执照名称", "联系人姓名"},
			"word/footer1.xml":   {"联系人邮箱", "电话"},
			"word/footnotes.xml": {"电话"},
		},
		tplSplitRuns: {
			"word/document.xml":  {"联系人姓名", "法人代表", "实际经营地址", "营业执照名称", "纳税人识别号", "联系人邮箱", "电话", "开户银行", "账户号", "开票地址"},
			"word/header1.xml":   {"营业执照名称", "联系人姓名"},
			"word/footer1.xml":   {"联系人邮箱", "电话"},
			"word/footnotes.xml": {"电话"},
		},
	}

	for tpl, parts := range partValues {
		t.Run(filepath.Base(tpl), func(t *testing.T) {
			original, err := os.ReadFile(tpl)
			if err != nil {
				t.Fatal(err)
			}
			srcEntries := readZip(t, tpl)

			out := filepath.Join(t.TempDir(), "out.docx")
			if err := Render(tpl, out, testValues); err != nil {
				t.Fatalf("Render 失败: %v", err)
			}

			for part, fields := range parts {
				text := strings.Join(extractPartTextsOf(t, out, part), "\n")
				if strings.Contains(text, "{{") {
					t.Errorf("%s 中仍有未替换的占位符: %s", part, text)
				}
				for _, field := range fields {
					if !strings.Contains(text, testValues[field]) {
						t.Errorf("%s 中缺少字段 %s 的值", part, field)
					}
				}
			}

			// 原模板文件内容不变
			after, err := os.ReadFile(tpl)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(original, after) {
				t.Error("原始模板文件被修改")
			}
			for name, data := range srcEntries {
				if !bytes.Equal(data, readZip(t, tpl)[name]) {
					t.Errorf("原始模板部件 %s 被修改", name)
				}
			}
		})
	}
}

func TestRenderPreservesUnmodifiedZipEntriesByteForByte(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.docx")
	if err := Render(tplStructures, out, testValues); err != nil {
		t.Fatal(err)
	}
	src := readZip(t, tplStructures)
	dst := readZip(t, out)

	changed := map[string]bool{}
	for name, data := range src {
		if !bytes.Equal(data, dst[name]) {
			changed[name] = true
		}
	}
	for _, name := range []string{"word/styles.xml", "word/numbering.xml", "word/settings.xml", "word/theme/theme1.xml", "[Content_Types].xml"} {
		if changed[name] {
			t.Errorf("不应被改写的部件发生了变化: %s", name)
		}
	}
	if len(changed) != 4 {
		names := make([]string, 0, len(changed))
		for n := range changed {
			names = append(names, n)
		}
		t.Errorf("期望只有 4 个 WML 部件变化，实际 %d 个: %v", len(changed), names)
	}
}

func TestRenderBlocksProblemTemplates(t *testing.T) {
	cases := map[string]string{
		tplUnknown:  "不支持",
		tplUnclosed: "未闭合",
		tplUnsafe:   "打断",
	}
	for tpl, wantMsg := range cases {
		t.Run(filepath.Base(tpl), func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "should-not-exist.docx")
			err := Render(tpl, out, testValues)
			if err == nil {
				t.Fatal("问题模板必须阻止生成")
			}
			if !strings.Contains(err.Error(), wantMsg) {
				t.Errorf("错误信息未说明原因，err=%v", err)
			}
			if _, statErr := os.Stat(out); statErr == nil {
				t.Error("阻止生成时不应产出文件")
			}
		})
	}
}

func TestInspectReportsBrokenFiles(t *testing.T) {
	if _, err := Inspect(tplBrokenZip, supportedForTest); err == nil {
		t.Error("非 zip 文件应当报错")
	} else if !strings.Contains(err.Error(), "DOCX") {
		t.Errorf("错误信息应指出不是有效 DOCX，实际: %v", err)
	}
	if _, err := Inspect(tplBrokenXML, supportedForTest); err == nil {
		t.Error("截断的 XML 应当报错")
	}
	if _, err := Inspect(filepath.Join(t.TempDir(), "missing.docx"), supportedForTest); err == nil {
		t.Error("不存在的文件应当报错")
	}
}

func TestRenderWithUnsupportedValueReturnsError(t *testing.T) {
	values := map[string]string{}
	for k, v := range testValues {
		values[k] = v
	}
	delete(values, "电话")
	out := filepath.Join(t.TempDir(), "out.docx")
	err := Render(tplStructures, out, values)
	if err == nil {
		t.Fatal("缺少字段值时应当报错，避免生成缺信息的合同")
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Error("报错时不应产出文件")
	}
}

// ---------------------------------------------------------------------------
// 测试用独立实现：直接用 encoding/xml 抽取 w:t 文本，不依赖被测代码的解析逻辑。

func extractPartTextsOf(t *testing.T, path, part string) []string {
	t.Helper()
	data := readZip(t, path)[part]
	if len(data) == 0 {
		return nil
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	var (
		inText bool
		texts  []string
	)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("产物解析失败(%s): %v", part, err)
		}
		switch tt := tok.(type) {
		case xml.StartElement:
			if tt.Name.Space == wmlNSForTest && tt.Name.Local == "t" {
				inText = true
			}
		case xml.CharData:
			if inText {
				texts = append(texts, string(tt))
			}
		case xml.EndElement:
			if tt.Name.Space == wmlNSForTest && tt.Name.Local == "t" {
				inText = false
			}
		}
	}
	return texts
}

func readZip(t *testing.T, path string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("打开 %s 失败: %v", path, err)
	}
	defer zr.Close()
	out := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, rc); err != nil {
			t.Fatal(err)
		}
		rc.Close()
		out[f.Name] = buf.Bytes()
	}
	return out
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
