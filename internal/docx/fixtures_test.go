package docx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 本文件用 Go 现场构造 DOCX 夹具（不依赖 Python 与外部语料），
// 覆盖真实 Word 文档里容易出问题的结构：嵌套表格、多节页眉、修订痕迹、
// 超链接、内容控件、脚注尾注批注、空文本节点、多片段拆分、CDATA、断行打断等。

type docxFixture struct {
	body      string            // <w:body> 内容（不含 sectPr）
	sectPr    string            // 页面设置（为空用默认 A4）
	header    map[string]string // 部件名 -> 内容，如 {"word/header1.xml": ...}
	footer    map[string]string
	footnotes string
	endnotes  string
	comments  string
}

func buildDocxFixture(t *testing.T, f docxFixture) string {
	t.Helper()
	if f.sectPr == "" {
		f.sectPr = `<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1080" w:bottom="1440" w:left="1080"/></w:sectPr>`
	}
	doc := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"
 xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"
 xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math"
 xmlns:v="urn:schemas-microsoft-com:vml"
 xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing">
<w:body>` + f.body + f.sectPr + `</w:body></w:document>`

	parts := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`,
		"word/document.xml": doc,
	}

	rels := []string{}
	idx := 10
	addPart := func(partName, contentType, relType, content string) {
		parts[partName] = content
		ct := parts["[Content_Types].xml"]
		ct = strings.Replace(ct, "</Types>", fmt.Sprintf(
			`<Override PartName="/%s" ContentType="%s"/></Types>`, partName, contentType), 1)
		parts["[Content_Types].xml"] = ct
		idx++
		rels = append(rels, fmt.Sprintf(
			`<Relationship Id="rId%d" Type="%s" Target="%s"/>`,
			idx, relType, strings.TrimPrefix(partName, "word/")))
	}
	docRels := "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
	for name, content := range f.header {
		addPart(name, "application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml", docRels+"header", wrapHdrFtr("hdr", content))
	}
	for name, content := range f.footer {
		addPart(name, "application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml", docRels+"footer", wrapHdrFtr("ftr", content))
	}
	if f.footnotes != "" {
		addPart("word/footnotes.xml", "application/vnd.openxmlformats-officedocument.wordprocessingml.footnotes+xml", docRels+"footnotes", wrapNotes("footnotes", f.footnotes))
	}
	if f.endnotes != "" {
		addPart("word/endnotes.xml", "application/vnd.openxmlformats-officedocument.wordprocessingml.endnotes+xml", docRels+"endnotes", wrapNotes("endnotes", f.endnotes))
	}
	if f.comments != "" {
		addPart("word/comments.xml", "application/vnd.openxmlformats-officedocument.wordprocessingml.comments+xml", docRels+"comments", wrapNotes("comments", f.comments))
	}
	if len(rels) > 0 {
		parts["word/_rels/document.xml.rels"] = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + strings.Join(rels, "") + `</Relationships>`
	}

	path := filepath.Join(t.TempDir(), "fixture.docx")
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	for _, name := range orderedParts(parts) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(parts[name])); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func orderedParts(parts map[string]string) []string {
	preferred := []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml", "word/_rels/document.xml.rels"}
	var out []string
	for _, p := range preferred {
		if _, ok := parts[p]; ok {
			out = append(out, p)
		}
	}
	rest := []string{}
	for name := range parts {
		dup := false
		for _, p := range out {
			if p == name {
				dup = true
			}
		}
		if !dup {
			rest = append(rest, name)
		}
	}
	for i := 0; i < len(rest); i++ {
		for j := i + 1; j < len(rest); j++ {
			if rest[j] < rest[i] {
				rest[i], rest[j] = rest[j], rest[i]
			}
		}
	}
	return append(out, rest...)
}

const wmlPrefix = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"`

func wrapHdrFtr(kind, content string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:` + kind + ` ` + wmlPrefix + `>` + content + `</w:` + kind + `>`
}

func wrapNotes(kind, content string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:` + kind + ` ` + wmlPrefix + `>` + content + `</w:` + kind + `>`
}

func p(text string) string {
	return `<w:p><w:r><w:t xml:space="preserve">` + text + `</w:t></w:r></w:p>`
}

// ---------------------------------------------------------------------------
// 结构覆盖

func TestNestedTableAndMultipleSectionHeaders(t *testing.T) {
	inner := `<w:tbl><w:tr><w:tc><w:p><w:r><w:t>内层：{{账户号}}</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`
	outer := `<w:tbl><w:tr><w:tc>` + inner + `<w:p><w:r><w:t>外层：{{联系人姓名}}</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`
	body := outer + `<w:p><w:r><w:t>正文结束</w:t></w:r></w:p>`
	// 两节，各自引用不同页眉
	sect := `<w:sectPr><w:headerReference w:type="default" r:id="rId11"/><w:pgSz w:w="11906" w:h="16838"/></w:sectPr>`
	fixture := buildDocxFixture(t, docxFixture{
		body: body,
		header: map[string]string{
			"word/header1.xml": p("页眉一：{{营业执照名称}}"),
			"word/header2.xml": p("页眉二：{{电话}}"),
		},
		footer: map[string]string{
			"word/footer1.xml": p("页脚：{{联系人邮箱}}"),
		},
		sectPr: sect,
	})

	insp, err := Inspect(fixture, supportedForTest)
	if err != nil {
		t.Fatal(err)
	}
	if len(insp.Problems()) != 0 {
		t.Fatalf("不应有问题: %v", insp.Problems())
	}
	out := filepath.Join(t.TempDir(), "out.docx")
	if err := Render(fixture, out, testValues); err != nil {
		t.Fatal(err)
	}
	check := func(part, want string) {
		text := strings.Join(extractPartTextsOf(t, out, part), "\n")
		if !strings.Contains(text, want) {
			t.Errorf("%s 中缺少 %q，实际: %s", part, want, text)
		}
		if strings.Contains(text, "{{") {
			t.Errorf("%s 中仍有未替换占位符: %s", part, text)
		}
	}
	check("word/document.xml", testValues["账户号"])
	check("word/document.xml", testValues["联系人姓名"])
	check("word/header1.xml", testValues["营业执照名称"])
	check("word/header2.xml", testValues["电话"])
	check("word/footer1.xml", testValues["联系人邮箱"])
}

func TestRevisionMarksAreHandledCorrectly(t *testing.T) {
	// w:ins 是"已插入"的文本（正常显示）→ 应替换；
	// w:del 是被删除的文本（w:delText，Word 默认不显示）→ 不应被替换。
	ins := `<w:ins w:id="1" w:author="a"><w:r><w:t>{{联系人姓名}}</w:t></w:r></w:ins>`
	del := `<w:del w:id="2" w:author="a"><w:r><w:delText>{{法人代表}}</w:delText></w:r></w:del>`
	fixture := buildDocxFixture(t, docxFixture{body: `<w:p>` + ins + del + `</w:p>`})

	insp, err := Inspect(fixture, supportedForTest)
	if err != nil {
		t.Fatal(err)
	}
	if insp.FieldUsage["联系人姓名"] != 1 {
		t.Errorf("w:ins 中的占位符应被识别，实际 %d", insp.FieldUsage["联系人姓名"])
	}
	if insp.FieldUsage["法人代表"] != 0 {
		t.Errorf("w:delText 中的占位符不应被识别（Word 默认不显示），实际 %d", insp.FieldUsage["法人代表"])
	}
	out := filepath.Join(t.TempDir(), "out.docx")
	if err := Render(fixture, out, testValues); err != nil {
		t.Fatal(err)
	}
	text := strings.Join(extractPartTextsOf(t, out, "word/document.xml"), "\n")
	if !strings.Contains(text, testValues["联系人姓名"]) {
		t.Error("w:ins 中的占位符未被替换")
	}
	raw := string(readZip(t, out)["word/document.xml"])
	if !strings.Contains(raw, "{{法人代表}}") {
		t.Error("被删除的文本（w:delText）不应被改写")
	}
}

func TestPlaceholdersInsideHyperlinkAndContentControl(t *testing.T) {
	hyperlink := `<w:p><w:hyperlink r:id="rId99"><w:r><w:t>{{开户银行}}</w:t></w:r></w:hyperlink></w:p>`
	sdt := `<w:p><w:sdt><w:sdtPr><w:tag w:val="t"/></w:sdtPr><w:sdtContent><w:r><w:t>{{开票地址}}</w:t></w:r></w:sdtContent></w:sdt></w:p>`
	fixture := buildDocxFixture(t, docxFixture{body: hyperlink + sdt})

	out := filepath.Join(t.TempDir(), "out.docx")
	if err := Render(fixture, out, testValues); err != nil {
		t.Fatal(err)
	}
	text := strings.Join(extractPartTextsOf(t, out, "word/document.xml"), "\n")
	for _, field := range []string{"开户银行", "开票地址"} {
		if !strings.Contains(text, testValues[field]) {
			t.Errorf("%s 中的占位符未被替换", field)
		}
	}
	if strings.Contains(text, "{{") {
		t.Errorf("仍有未替换占位符: %s", text)
	}
}

func TestPlaceholdersInFootnotesEndnotesComments(t *testing.T) {
	fixture := buildDocxFixture(t, docxFixture{
		body:      p("正文"),
		footnotes: `<w:footnote w:id="1">` + p("脚注：{{电话}}") + `</w:footnote>`,
		endnotes:  `<w:endnote w:id="1">` + p("尾注：{{联系人电话}}") + `</w:endnote>`,
		comments:  `<w:comment w:id="1" w:author="a">` + p("批注：{{账户号}}") + `</w:comment>`,
	})
	insp, err := Inspect(fixture, supportedForTest)
	if err != nil {
		t.Fatal(err)
	}
	if len(insp.Parts) != 4 {
		t.Errorf("应扫描 4 个部件，实际 %v", insp.Parts)
	}
	out := filepath.Join(t.TempDir(), "out.docx")
	if err := Render(fixture, out, testValues); err != nil {
		t.Fatal(err)
	}
	for part, want := range map[string]string{
		"word/footnotes.xml": testValues["电话"],
		"word/endnotes.xml":  testValues["联系人电话"],
		"word/comments.xml":  testValues["账户号"],
	} {
		text := strings.Join(extractPartTextsOf(t, out, part), "\n")
		if !strings.Contains(text, want) {
			t.Errorf("%s 未替换（期望包含 %s）", part, want)
		}
	}
}

func TestBarrierElementsBlockPlaceholder(t *testing.T) {
	cases := map[string]string{
		"换行打断":  `<w:p><w:r><w:t>{{联系</w:t></w:r><w:r><w:br/></w:r><w:r><w:t>人姓名}}</w:t></w:r></w:p>`,
		"制表符打断": `<w:p><w:r><w:t>{{联系</w:t><w:tab/><w:t>人姓名}}</w:t></w:r></w:p>`,
		"域代码打断": `<w:p><w:r><w:t>{{联系</w:t></w:r><w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:t>人姓名}}</w:t></w:r></w:p>`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := buildDocxFixture(t, docxFixture{body: body})
			insp, err := Inspect(fixture, supportedForTest)
			if err != nil {
				t.Fatal(err)
			}
			if len(insp.UnsafeSpans) == 0 {
				t.Fatalf("应识别为被元素打断的占位符: %+v", insp.Found)
			}
			if len(insp.Problems()) == 0 {
				t.Fatal("应产生阻止生成的错误")
			}
			out := filepath.Join(t.TempDir(), "out.docx")
			if err := Render(fixture, out, testValues); err == nil {
				t.Fatal("被元素打断的占位符必须阻止生成")
			}
			if _, err := os.Stat(out); err == nil {
				t.Error("阻止生成时不应产出文件")
			}
		})
	}
}

func TestManySmallFragmentsAndEmptyTextNodes(t *testing.T) {
	// 占位符被拆成 6 个片段，其中夹杂空 <w:t/>
	body := `<w:p>` +
		`<w:r><w:t>{{</w:t></w:r>` +
		`<w:r><w:t/></w:r>` +
		`<w:r><w:t>联</w:t></w:r>` +
		`<w:r><w:rPr><w:b/></w:rPr><w:t>系人</w:t></w:r>` +
		`<w:r><w:t/></w:r>` +
		`<w:r><w:t>姓</w:t></w:r>` +
		`<w:r><w:t>名</w:t></w:r>` +
		`<w:r><w:t>}}</w:t></w:r>` +
		`</w:p>`
	fixture := buildDocxFixture(t, docxFixture{body: body})
	out := filepath.Join(t.TempDir(), "out.docx")
	if err := Render(fixture, out, testValues); err != nil {
		t.Fatal(err)
	}
	text := strings.Join(extractPartTextsOf(t, out, "word/document.xml"), "\n")
	if !strings.Contains(text, testValues["联系人姓名"]) {
		t.Errorf("跨 6 个片段的占位符未被替换: %s", text)
	}
	if strings.Contains(text, "{{") || strings.Contains(text, "}}") {
		t.Errorf("产物中残留占位符碎片: %s", text)
	}
}

func TestCdataTextIsRejected(t *testing.T) {
	body := `<w:p><w:r><w:t><![CDATA[{{联系人姓名}}]]></w:t></w:r></w:p>`
	fixture := buildDocxFixture(t, docxFixture{body: body})
	out := filepath.Join(t.TempDir(), "out.docx")
	err := Render(fixture, out, testValues)
	if err == nil {
		t.Fatal("CDATA 包裹的文本应被拒绝（避免结构破坏）")
	}
	if !strings.Contains(err.Error(), "CDATA") {
		t.Errorf("错误信息应说明 CDATA 限制，实际: %v", err)
	}
}

func TestUnsupportedAndUnclosedPlaceholdersReported(t *testing.T) {
	fixture := buildDocxFixture(t, docxFixture{
		body: p("{{不存在的字段}}") + p("{{没闭合") + p("正常：{{联系人姓名}}"),
	})
	insp, err := Inspect(fixture, supportedForTest)
	if err != nil {
		t.Fatal(err)
	}
	if len(insp.Unknown) != 1 || insp.Unknown[0].Name != "不存在的字段" {
		t.Errorf("应识别未知占位符，实际 %+v", insp.Unknown)
	}
	if len(insp.Unclosed) != 1 {
		t.Errorf("应识别未闭合占位符，实际 %+v", insp.Unclosed)
	}
	if len(insp.Problems()) < 2 {
		t.Errorf("应产生至少 2 条错误，实际 %v", insp.Problems())
	}
	// 校验通过前不应产出文件
	out := filepath.Join(t.TempDir(), "out.docx")
	if err := Render(fixture, out, testValues); err == nil {
		t.Fatal("未知/未闭合占位符必须阻止生成")
	}
}
