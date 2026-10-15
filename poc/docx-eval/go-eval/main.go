package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"docxeval/engine"

	godocx "github.com/gomutex/godocx"
	golukas "github.com/lukasjarosch/go-docx"
	nguyen "github.com/nguyenthenguyen/docx"
)

// 十个必填字段（含前导零、特殊字符、超长文本）
var fieldOrder = []string{
	"联系人姓名", "法人代表", "实际经营地址", "联系人邮箱", "营业执照名称",
	"纳税人识别号", "电话", "开户银行", "账户号", "开票地址",
}

var fieldValues = map[string]string{
	"联系人姓名":  "张三 & 李四",
	"法人代表":   "李四<测试>",
	"实际经营地址": "北京市海淀区中关村大街1号院3号楼5层501室（临街商铺，靠近地铁站A口，方位说明很长的中文地址文本用于验证长文本换行与排版是否正常）",
	"联系人邮箱":  "zhangsan@example.com",
	"营业执照名称": "示例科技（北京）有限公司",
	"纳税人识别号": "91110108MA01ABCD2X",
	"电话":     "010-88886666",
	"开户银行":   "中国工商银行股份有限公司北京中关村支行",
	"账户号":    "0001234567890123456",
	"开票地址":   "上海市浦东新区世纪大道100号A座20层",
}

type AdapterResult struct {
	Adapter    string   `json:"adapter"`
	Template   string   `json:"template"`
	OK         bool     `json:"ok"`
	Error      string   `json:"error"`
	Notes      []string `json:"notes"`
	OutputFile string   `json:"outputFile"`
	Panic      bool     `json:"panic"`
}

type Report struct {
	Adapters  []AdapterResult `json:"adapters"`
	Inspect   map[string]any  `json:"inspect"`
	Templates []string        `json:"templates"`
}

func safeRun(name string, fn func() error) (res AdapterResult) {
	res.Adapter = name
	defer func() {
		if r := recover(); r != nil {
			res.OK = false
			res.Panic = true
			res.Error = fmt.Sprintf("panic: %v", r)
		}
	}()
	err := fn()
	res.OK = err == nil
	if err != nil {
		res.Error = err.Error()
	}
	return res
}

func main() {
	corpus := os.Args[1]
	outDir := os.Args[2]
	os.MkdirAll(outDir, 0o755)

	templates := []string{"template_simple.docx", "template_structures.docx", "template_split_runs.docx"}
	report := Report{Inspect: map[string]any{}, Templates: templates}

	// ---------- 1. 自研引擎：模板扫描/校验 ----------
	for _, tpl := range templates {
		p := filepath.Join(corpus, tpl)
		res, err := engine.Inspect(p, fieldOrder)
		if err != nil {
			report.Inspect[tpl] = map[string]any{"error": err.Error()}
			continue
		}
		unknown := []string{}
		for _, u := range res.Unknown {
			unknown = append(unknown, u.Name+"@"+u.Part)
		}
		unclosed := []string{}
		for _, u := range res.Unclosed {
			unclosed = append(unclosed, u.Text)
		}
		unsafe := []string{}
		for _, u := range res.UnsafeSpans {
			unsafe = append(unsafe, u.Name+"@"+u.Part)
		}
		report.Inspect[tpl] = map[string]any{
			"parts":       res.Parts,
			"found":       len(res.Found),
			"unknown":     unknown,
			"unclosed":    unclosed,
			"unsafe":      unsafe,
			"blockErrors": res.Errors(),
		}
	}

	// 损坏文件处理
	for _, tpl := range []string{"template_broken_zip.docx", "template_broken_xml.docx"} {
		p := filepath.Join(corpus, tpl)
		_, err := engine.Inspect(p, fieldOrder)
		msg := "nil"
		if err != nil {
			msg = err.Error()
		}
		report.Inspect[tpl] = map[string]any{"error": msg}
	}

	// ---------- 2. 自研引擎：生成 ----------
	for _, tpl := range templates {
		out := filepath.Join(outDir, "engine__"+tpl)
		r := safeRun("engine(自研)", func() error {
			return engine.Render(filepath.Join(corpus, tpl), out, fieldValues)
		})
		r.Template = tpl
		r.OutputFile = out
		report.Adapters = append(report.Adapters, r)
	}

	// 必须被阻止生成的三类模板
	for _, tpl := range []string{"template_unknown.docx", "template_unclosed.docx", "template_unsafe.docx"} {
		out := filepath.Join(outDir, "engine__blocked_"+tpl)
		err := engine.Render(filepath.Join(corpus, tpl), out, fieldValues)
		_, statErr := os.Stat(out)
		report.Inspect["block_"+tpl] = fmt.Sprintf("engine.Render = %v | 输出文件是否生成=%v", err, statErr == nil)
	}

	// ---------- 3. nguyenthenguyen/docx ----------
	for _, tpl := range templates {
		out := filepath.Join(outDir, "nguyen__"+tpl)
		notes := []string{}
		r := safeRun("nguyenthenguyen/docx", func() error {
			rd, err := nguyen.ReadDocxFile(filepath.Join(corpus, tpl))
			if err != nil {
				return err
			}
			defer rd.Close()
			ed := rd.Editable()
			for _, k := range fieldOrder {
				before := ed.GetContent()
				if err := ed.Replace("{{"+k+"}}", fieldValues[k], -1); err != nil {
					notes = append(notes, k+": "+err.Error())
				} else if before == ed.GetContent() {
					notes = append(notes, k+": 未匹配(片段被拆分)")
				}
			}
			if err := ed.WriteToFile(out); err != nil {
				return err
			}
			// 页眉页脚需显式调用 ReplaceHeader/ReplaceFooter
			notes = append(notes, "页眉/页脚需单独调用 ReplaceHeader/ReplaceFooter（本测试未调用）")
			notes = append(notes, "脚注/文本框部件不在处理范围内")
			return nil
		})
		r.Template = tpl
		r.OutputFile = out
		r.Notes = notes
		report.Adapters = append(report.Adapters, r)
	}

	// ---------- 4. lukasjarosch/go-docx ----------
	for _, tpl := range templates {
		out := filepath.Join(outDir, "lukas__"+tpl)
		notes := []string{}
		r := safeRun("lukasjarosch/go-docx", func() error {
			doc, err := golukas.Open(filepath.Join(corpus, tpl))
			if err != nil {
				return err
			}
			for _, k := range fieldOrder {
				if err := doc.Replace("{{"+k+"}}", fieldValues[k]); err != nil {
					notes = append(notes, k+": "+err.Error())
				}
			}
			if err := doc.WriteToFile(out); err != nil {
				return err
			}
			ph, err := doc.GetPlaceHoldersList()
			if err != nil {
				notes = append(notes, "GetPlaceHoldersList: "+err.Error())
			} else {
				notes = append(notes, fmt.Sprintf("解析到占位符 %d 个: %s", len(ph), strings.Join(ph, ",")))
			}
			return nil
		})
		r.Template = tpl
		r.OutputFile = out
		r.Notes = notes
		report.Adapters = append(report.Adapters, r)
	}

	// ---------- 5. gomutex/godocx（打开-保存保真度） ----------
	for _, tpl := range templates {
		out := filepath.Join(outDir, "godocx__"+tpl)
		notes := []string{"godocx 未提供占位符替换 API，本项只测「打开→原样保存」的保真度"}
		r := safeRun("gomutex/godocx(open+save)", func() error {
			doc, err := godocx.OpenDocument(filepath.Join(corpus, tpl))
			if err != nil {
				return err
			}
			if err := doc.SaveTo(out); err != nil {
				return err
			}
			return nil
		})
		r.Template = tpl
		r.OutputFile = out
		r.Notes = notes
		report.Adapters = append(report.Adapters, r)
	}

	// 部件级对比：原模板 vs godocx 输出
	for _, tpl := range templates {
		a := filepath.Join(corpus, tpl)
		b := filepath.Join(outDir, "godocx__"+tpl)
		notes := compareParts(a, b)
		report.Inspect["godocx_partdiff_"+tpl] = notes
	}

	buf, _ := json.MarshalIndent(report, "", "  ")
	os.WriteFile(filepath.Join(outDir, "report.json"), buf, 0o644)
	fmt.Println(string(buf))
}

// compareParts 返回两个 docx 部件清单与内容差异（用于评估“打开-保存”是否丢结构）
func compareParts(a, b string) []string {
	za, err1 := zip.OpenReader(a)
	zb, err2 := zip.OpenReader(b)
	if err1 != nil || err2 != nil {
		return []string{"打开失败"}
	}
	defer za.Close()
	defer zb.Close()
	read := func(r *zip.Reader, name string) ([]byte, bool) {
		for _, f := range r.File {
			if f.Name == name {
				rc, err := f.Open()
				if err != nil {
					return nil, false
				}
				defer rc.Close()
				var buf bytes.Buffer
				io.Copy(&buf, rc)
				return buf.Bytes(), true
			}
		}
		return nil, false
	}
	var notes []string
	namesA := map[string]bool{}
	for _, f := range za.File {
		namesA[f.Name] = true
	}
	for _, f := range zb.File {
		if !namesA[f.Name] {
			notes = append(notes, "新增部件: "+f.Name)
		}
	}
	namesB := map[string]bool{}
	for _, f := range zb.File {
		namesB[f.Name] = true
	}
	for _, f := range za.File {
		if !namesB[f.Name] {
			notes = append(notes, "丢失部件: "+f.Name)
		}
	}
	for name := range namesA {
		if !namesB[name] {
			continue
		}
		x, _ := read(&za.Reader, name)
		y, _ := read(&zb.Reader, name)
		if !bytes.Equal(x, y) {
			notes = append(notes, fmt.Sprintf("内容变化: %s (%d -> %d 字节)", name, len(x), len(y)))
		}
	}
	if len(notes) == 0 {
		notes = append(notes, "完全一致")
	}
	return notes
}
