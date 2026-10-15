// Package engine 是 DOCX 占位符替换的原型实现，用于阶段 1 技术选型验证。
//
// 设计要点：
//  1. 直接在 zip 内的 XML 部件字节流上做「外科手术式」编辑：只重写发生变化的
//     <w:t> 文本节点，其余字节原样保留（未修改的 zip 条目通过 CreateRaw 原样复制）。
//  2. 以 <w:p> 段落为单位拼接所有 <w:t> 文本，因此天然支持 Word 把占位符
//     拆成多个 run / 多个文本片段的情况。
//  3. 遍历 word/ 下所有 WordprocessingML 部件（document/header/footer/footnotes/
//     endnotes/comments），因此正文、表格、页眉、页脚、脚注、文本框一视同仁。
//  4. 不做全量 XML 重序列化，最大限度保留原始格式与未知元素。
package engine

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"encoding/xml"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
)

const wmlNS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

// placeholderRe 匹配 {{字段名}}，字段名内不允许出现花括号。
var placeholderRe = regexp.MustCompile(`\{\{([^{}]*)\}\}`)

// 会打断占位符连续性的元素：出现这些元素时，跨元素的占位符不可安全替换。
var barrierElements = map[string]bool{
	"br": true, "tab": true, "cr": true, "drawing": true, "pict": true,
	"fldChar": true, "instrText": true, "object": true, "noBreakHyphen": true,
	"softHyphen": true, "sym": true, "footnoteReference": true, "endnoteReference": true,
}

// Placeholder 表示模板中出现的一个占位符。
type Placeholder struct {
	Part   string // zip 部件名，如 word/header1.xml
	Name   string // 去掉定界符并 TrimSpace 之后的字段名
	Raw    string // 原始字面量，如 {{ 联系人姓名 }}
	Text   string // 所在段落拼接后的文本（用于诊断）
	unsafe bool   // 是否跨越 barrier 元素
}

// Inspect 结果。
type InspectResult struct {
	Parts        []string      // 参与扫描的部件
	Found        []Placeholder // 全部可解析的占位符
	Unknown      []Placeholder // 不在支持字段列表中的占位符
	Unclosed     []Placeholder // 只有 {{ 没有 }} 的残缺占位符
	UnsafeSpans  []Placeholder // 跨 w:br/w:tab 等元素的占位符
	UnknownCount map[string]int
}

// Errors 把需要阻止生成的问题汇总成错误。
func (r *InspectResult) Errors() []string {
	var errs []string
	seen := map[string]bool{}
	for _, p := range r.Unknown {
		key := "unknown:" + p.Name
		if !seen[key] {
			seen[key] = true
			errs = append(errs, fmt.Sprintf("模板中存在不支持的占位符 {{%s}}（%s）", p.Name, p.Part))
		}
	}
	for _, p := range r.Unclosed {
		key := "unclosed:" + p.Part + ":" + p.Text
		if !seen[key] {
			seen[key] = true
			errs = append(errs, fmt.Sprintf("模板中存在未闭合的占位符（%s）：%q", p.Part, p.Text))
		}
	}
	for _, p := range r.UnsafeSpans {
		key := "unsafe:" + p.Name + ":" + p.Part
		if !seen[key] {
			seen[key] = true
			errs = append(errs, fmt.Sprintf("占位符 {{%s}} 被换行/制表符等元素打断，无法安全替换（%s）", p.Name, p.Part))
		}
	}
	return errs
}

type textNode struct {
	startTagStart int64
	startTagEnd   int64
	textStart     int64
	textEnd       int64
	text          string
	preserve      bool
	cdata         bool
}

type paragraph struct {
	nodes    []*textNode
	text     string
	barriers []int // text 中被打断元素的字节位置
}

// ---------------------------------------------------------------------------
// 解析

func parsePart(data []byte) ([]*paragraph, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var (
		paras   []*paragraph
		stack   []*paragraph
		curNode *textNode
		last    int64
		depth   int
	)
	top := func() *paragraph {
		if len(stack) == 0 {
			return nil
		}
		return stack[len(stack)-1]
	}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("XML 解析失败: %w", err)
		}
		off := dec.InputOffset()
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			isWML := t.Name.Space == wmlNS
			switch {
			case isWML && t.Name.Local == "p":
				stack = append(stack, &paragraph{})
			case isWML && t.Name.Local == "t":
				curNode = &textNode{startTagStart: last, startTagEnd: off, textStart: off}
			case isWML && barrierElements[t.Name.Local] && top() != nil:
				p := top()
				p.barriers = append(p.barriers, len(p.text))
			}
		case xml.CharData:
			if curNode != nil {
				raw := data[curNode.textStart:off]
				if bytes.HasPrefix(raw, []byte("<![CDATA[")) {
					curNode.cdata = true
					curNode.textEnd = off
					curNode.text = string(t)
				} else {
					curNode.textEnd = off
					curNode.text = string(t)
				}
				curNode.preserve = hasPreserve(data[curNode.startTagStart:curNode.startTagEnd])
				if p := top(); p != nil {
					p.text += curNode.text
					p.nodes = append(p.nodes, curNode)
				}
				curNode = nil
			}
		case xml.EndElement:
			depth--
			isWML := t.Name.Space == wmlNS
			if isWML && t.Name.Local == "t" && curNode != nil {
				// 空元素 <w:t/>：start == end
				curNode.textStart = curNode.startTagEnd
				curNode.textEnd = curNode.startTagEnd
				curNode.text = ""
				curNode.preserve = hasPreserve(data[curNode.startTagStart:curNode.startTagEnd])
				if p := top(); p != nil {
					p.nodes = append(p.nodes, curNode)
				}
				curNode = nil
			}
			if isWML && t.Name.Local == "p" && len(stack) > 0 {
				paras = append(paras, stack[len(stack)-1])
				stack = stack[:len(stack)-1]
			}
		}
		last = off
	}
	if len(stack) > 0 { // 容错：未闭合
		paras = append(paras, stack...)
	}
	// Go 的 encoding/xml 不校验标签闭合，这里自己兜底，避免把被截断/损坏的部件当成正常模板
	if depth != 0 {
		return nil, fmt.Errorf("XML 结构不完整：有 %d 个未闭合标签（文件可能已损坏）", depth)
	}
	return paras, nil
}

func hasPreserve(startTag []byte) bool {
	i := bytes.Index(startTag, []byte("xml:space"))
	if i < 0 {
		return false
	}
	return bytes.Contains(startTag[i:], []byte("preserve"))
}

// ---------------------------------------------------------------------------
// 扫描与校验

// Parts 返回需要处理的部件名。
func listParts(r *zip.Reader) ([]string, error) {
	var names []string
	for _, f := range r.File {
		name := f.Name
		if !strings.HasSuffix(name, ".xml") {
			continue
		}
		if !strings.HasPrefix(name, "word/") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		head := make([]byte, 65536)
		n, _ := io.ReadFull(rc, head)
		rc.Close()
		if !isWordprocessingPart(head[:n]) {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

func isWordprocessingPart(head []byte) bool {
	dec := xml.NewDecoder(bytes.NewReader(head))
	for {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		if se, ok := tok.(xml.StartElement); ok {
			if se.Name.Space != wmlNS {
				return false
			}
			switch se.Name.Local {
			case "document", "hdr", "ftr", "footnotes", "endnotes", "comments":
				return true
			}
			return false
		}
	}
}

// Inspect 扫描模板，返回占位符与校验结果。
func Inspect(templatePath string, supported []string) (*InspectResult, error) {
	zr, err := zip.OpenReader(templatePath)
	if err != nil {
		return nil, describeOpenError(templatePath, err)
	}
	defer zr.Close()
	return inspectZip(&zr.Reader, supported)
}

func inspectZip(r *zip.Reader, supported []string) (*InspectResult, error) {
	supportedSet := map[string]bool{}
	for _, s := range supported {
		supportedSet[s] = true
	}
	res := &InspectResult{}
	parts, err := listParts(r)
	if err == nil && len(parts) == 0 {
		return nil, errors.New("文件中没有找到 Word 正文部件，可能不是有效的 DOCX")
	}
	if err != nil {
		return nil, err
	}
	res.Parts = parts
	for _, name := range parts {
		data, err := readZipFile(r, name)
		if err != nil {
			return nil, err
		}
		paras, err := parsePart(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		for _, p := range paras {
			matches := placeholderRe.FindAllStringSubmatchIndex(p.text, -1)
			covered := make([][2]int, 0, len(matches))
			for _, m := range matches {
				raw := p.text[m[0]:m[1]]
				nameInner := strings.TrimSpace(p.text[m[2]:m[3]])
				covered = append(covered, [2]int{m[0], m[1]})
				ph := Placeholder{Part: name, Name: nameInner, Raw: raw, Text: p.text}
				res.Found = append(res.Found, ph)
				if len(p.barriers) > 0 {
					for _, b := range p.barriers {
						if b > m[0] && b < m[1] {
							ph.unsafe = true
							res.UnsafeSpans = append(res.UnsafeSpans, ph)
							break
						}
					}
				}
				if !supportedSet[nameInner] {
					res.Unknown = append(res.Unknown, ph)
				}
			}
			// 未闭合的 {{
			for i := 0; i+1 < len(p.text); i++ {
				if p.text[i] == '{' && p.text[i+1] == '{' {
					isCovered := false
					for _, c := range covered {
						if i >= c[0] && i < c[1] {
							isCovered = true
							break
						}
					}
					if !isCovered {
						res.Unclosed = append(res.Unclosed, Placeholder{
							Part: name, Name: "", Raw: p.text[i:], Text: p.text,
						})
						break
					}
				}
			}
		}
	}
	return res, nil
}

// ---------------------------------------------------------------------------
// 生成

// Render 用 values 渲染模板并写出到 outPath。
// values 的 key 是不带定界符的字段名。
func Render(templatePath, outPath string, values map[string]string) error {
	zr, err := zip.OpenReader(templatePath)
	if err != nil {
		return describeOpenError(templatePath, err)
	}
	defer zr.Close()

	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	if res, err := inspectZip(&zr.Reader, keys); err != nil {
		return err
	} else if errs := res.Errors(); len(errs) > 0 {
		return errors.New(strings.Join(errs, "；"))
	}

	out := &bytes.Buffer{}
	zw := zip.NewWriter(out)
	for _, f := range zr.File {
		data, err := readZipFile(&zr.Reader, f.Name)
		if err != nil {
			return err
		}
		if strings.HasPrefix(f.Name, "word/") && strings.HasSuffix(f.Name, ".xml") &&
			isWordprocessingPart(firstChunk(data)) {
			newData, err := renderPart(data, values)
			if err != nil {
				return fmt.Errorf("%s: %w", f.Name, err)
			}
			if !bytes.Equal(newData, data) {
				// 自己压缩并写入 raw 数据，避免 Go zip 默认的 data descriptor（部分 OPC 读取器对 bit3 更严格）
				var cbuf bytes.Buffer
				fw, err := flate.NewWriter(&cbuf, flate.DefaultCompression)
				if err != nil {
					return err
				}
				if _, err := fw.Write(newData); err != nil {
					return err
				}
				if err := fw.Close(); err != nil {
					return err
				}
				hdr := f.FileHeader
				hdr.Flags &^= 0x8
				hdr.Method = zip.Deflate
				hdr.CRC32 = crc32.ChecksumIEEE(newData)
				hdr.CompressedSize64 = uint64(cbuf.Len())
				hdr.UncompressedSize64 = uint64(len(newData))
				hdr.CompressedSize = uint32(cbuf.Len())
				hdr.UncompressedSize = uint32(len(newData))
				w, err := zw.CreateRaw(&hdr)
				if err != nil {
					return err
				}
				if _, err := w.Write(cbuf.Bytes()); err != nil {
					return err
				}
				continue
			}
		}
		// 未修改：原样复制压缩数据
		hdr := f.FileHeader
		hdr.Flags &^= 0x8
		w, err := zw.CreateRaw(&hdr)
		if err != nil {
			return err
		}
		rc, err := f.OpenRaw()
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, rc); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(outPath, out.Bytes(), 0o644)
}

type edit struct {
	start, end int64
	repl       []byte
}

func renderPart(data []byte, values map[string]string) ([]byte, error) {
	paras, err := parsePart(data)
	if err != nil {
		return nil, err
	}
	var edits []edit
	for _, p := range paras {
		matches := placeholderRe.FindAllStringSubmatchIndex(p.text, -1)
		if len(matches) == 0 {
			continue
		}
		type m struct {
			start, end int
			valueRaw   []byte
		}
		var ms []m
		for _, idx := range matches {
			nameInner := strings.TrimSpace(p.text[idx[2]:idx[3]])
			val, ok := values[nameInner]
			if !ok {
				// 未提供值：保持原样并报错，避免产生缺失合同信息
				return nil, fmt.Errorf("模板中的占位符 {{%s}} 不在本应用支持的字段列表中", nameInner)
			}
			ms = append(ms, m{start: idx[0], end: idx[1], valueRaw: encodeValueForWML(val)})
		}
		// 逐节点计算新内容
		base := 0
		for _, n := range p.nodes {
			a, b := base, base+len(n.text)
			base = b
			var buf bytes.Buffer
			cursor := a
			changed := false
			for _, mm := range ms {
				if mm.end <= a || mm.start >= b {
					continue
				}
				ls := mm.start
				if ls < a {
					ls = a
				}
				le := mm.end
				if le > b {
					le = b
				}
				escapeInto(&buf, p.text[cursor:ls])
				if mm.start >= a && mm.start < b {
					buf.Write(mm.valueRaw)
				}
				cursor = le
				changed = true
			}
			if !changed {
				continue
			}
			if n.cdata {
				return nil, errors.New("模板中使用了 CDATA 包裹的文本，本版本不支持替换（请用 Word 重新保存该模板）")
			}
			escapeInto(&buf, p.text[cursor:b])
			edits = append(edits, edit{start: n.textStart, end: n.textEnd, repl: buf.Bytes()})
			// 需要保留前后空白时补 xml:space="preserve"
			if !n.preserve && needsPreserve(buf.Bytes()) {
				startTag := data[n.startTagStart:n.startTagEnd]
				closing := []byte(">")
				if bytes.HasSuffix(startTag, []byte("/>")) {
					closing = []byte("/>")
				}
				newTag := make([]byte, 0, len(startTag)+24)
				newTag = append(newTag, startTag[:len(startTag)-len(closing)]...)
				newTag = append(newTag, []byte(` xml:space="preserve"`)...)
				newTag = append(newTag, closing...)
				edits = append(edits, edit{start: n.startTagStart, end: n.startTagEnd, repl: newTag})
			}
		}
	}
	if len(edits) == 0 {
		return data, nil
	}
	return applyEdits(data, edits), nil
}

func needsPreserve(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	return isSpace(raw[0]) || isSpace(raw[len(raw)-1])
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// encodeValueForWML 把用户输入渲染成可插入 <w:t> 内部的 XML 片段。
// 换行转换为 <w:br/>，其余字符做 XML 转义。
func encodeValueForWML(v string) []byte {
	v = strings.ReplaceAll(v, "\r\n", "\n")
	v = strings.ReplaceAll(v, "\r", "\n")
	parts := strings.Split(v, "\n")
	var buf bytes.Buffer
	for i, part := range parts {
		if i > 0 {
			buf.WriteString(`</w:t><w:br/><w:t xml:space="preserve">`)
		}
		escapeInto(&buf, part)
	}
	return buf.Bytes()
}

func escapeInto(buf *bytes.Buffer, s string) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			buf.WriteString("&amp;")
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		default:
			buf.WriteByte(s[i])
		}
	}
}

func applyEdits(data []byte, edits []edit) []byte {
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	out := data
	for _, e := range edits {
		if e.start == e.end && len(e.repl) == 0 {
			continue
		}
		buf := make([]byte, 0, len(out)+len(e.repl))
		buf = append(buf, out[:e.start]...)
		buf = append(buf, e.repl...)
		buf = append(buf, out[e.end:]...)
		out = buf
	}
	return out
}

// ---------------------------------------------------------------------------
// 工具

func readZipFile(r *zip.Reader, name string) ([]byte, error) {
	for _, f := range r.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, rc); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	return nil, fmt.Errorf("部件不存在: %s", name)
}

// describeOpenError 区分「加密文件 / 非 zip 文件 / 一般损坏」，便于界面给出可理解的提示。
func describeOpenError(path string, err error) error {
	f, ferr := os.Open(path)
	if ferr == nil {
		defer f.Close()
		head := make([]byte, 8)
		if n, _ := io.ReadFull(f, head); n >= 4 {
			// OLE/CFB 复合文档头：Word 的「用密码加密」文件，以及旧的 .doc 文件都是这种格式
			if bytes.Equal(head[:4], []byte{0xD0, 0xCF, 0x11, 0xE0}) {
				return errors.New("该文件是加密文档或旧版 .doc 格式，无法作为模板使用（请先在 Word 中另存为未加密的 .docx）")
			}
			if !bytes.HasPrefix(head[:min(n, 4)], []byte("PK")) {
				return errors.New("该文件不是有效的 DOCX（DOCX 必须是 zip 包结构）")
			}
		}
	}
	return fmt.Errorf("无法读取模板文件（可能已损坏）: %w", err)
}

func firstChunk(data []byte) []byte {
	if len(data) > 4096 {
		return data[:4096]
	}
	return data
}
