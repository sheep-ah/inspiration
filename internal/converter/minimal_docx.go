package converter

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"fmt"
	"io"
)

// BuildMinimalDocx 返回一个最小但合法的 .docx（含一句话文本），
// 用于"转换通道自检"这类不需要真实模板的场景。
func BuildMinimalDocx() ([]byte, error) {
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	// 用自带 deflate 压缩（不写 data descriptor，部分 OPC 读取器更严格）
	zw.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, flate.DefaultCompression)
	})
	files := map[string]string{
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
		"word/document.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
<w:p><w:r><w:t>灵感转换通道自检 {{联系人姓名}}</w:t></w:r></w:p>
<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440"/></w:sectPr>
</w:body></w:document>`,
	}
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml"} {
		w, err := zw.Create(name)
		if err != nil {
			return nil, fmt.Errorf("构建自检文档失败: %w", err)
		}
		if _, err := w.Write([]byte(files[name])); err != nil {
			return nil, fmt.Errorf("构建自检文档失败: %w", err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("构建自检文档失败: %w", err)
	}
	return buf.Bytes(), nil
}
