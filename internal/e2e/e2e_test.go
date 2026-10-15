// Package e2e 是阶段 2 的端到端验收测试：真实 DOCX 模板 → 导入 → 生成 → 独立校验 → 重启恢复。
package e2e

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sheep-ah/inspiration/internal/config"
	"github.com/sheep-ah/inspiration/internal/contract"
	"github.com/sheep-ah/inspiration/internal/converter"
	"github.com/sheep-ah/inspiration/internal/docx"
	"github.com/sheep-ah/inspiration/internal/store"
	"github.com/sheep-ah/inspiration/internal/template"
)

// 真实语料：含正文、表格、页眉、页脚、脚注、VML 文本框、DrawingML 文本框、内容控件。
const realTemplate = "../docx/testdata/template_structures.docx"

func testValues() contract.Values {
	return contract.Values{
		"联系人姓名":  "张三 & 李四",
		"法人代表":   "李四<测试>",
		"实际经营地址": "北京市海淀区中关村大街1号院3号楼5层501室（临街商铺，靠近地铁站A口）",
		"联系人邮箱":  "zhangsan@example.com",
		"营业执照名称": "示例科技（北京）有限公司",
		"纳税人识别号": "91110108MA01ABCD2X",
		"电话":     "010-88886666",
		"联系人电话":  "13800000000",
		"开户银行":   "中国工商银行股份有限公司北京中关村支行",
		"账户号":    "0001234567890123456",
		"开票地址":   "上海市浦东新区世纪大道100号A座20层",
	}
}

type stack struct {
	cfg   *config.Config
	store *store.Store
	tpl   *template.Service
	con   *contract.Service
	conv  *converter.Converter
}

func newStack(t *testing.T, root string) *stack {
	t.Helper()
	cfg, err := config.At(root)
	if err != nil {
		t.Fatalf("初始化数据目录失败: %v", err)
	}
	st, err := store.Open(cfg)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	fields := contract.FieldNames()
	conv := converter.New(filepath.Join(root, "convert"))
	return &stack{cfg: cfg, store: st, tpl: template.NewService(st, fields, conv), con: contract.NewService(st, conv), conv: conv}
}

func TestEndToEndWithRealDocx(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "合同输出")

	s := newStack(t, root)

	// 1) 导入真实模板
	detail, err := s.tpl.Import(ctx, realTemplate, "标准合同模板")
	if err != nil {
		t.Fatalf("导入模板失败: %v", err)
	}
	if len(detail.Problems) != 0 {
		t.Fatalf("模板不应有问题: %v", detail.Problems)
	}
	if len(detail.Placeholders) != 10 {
		t.Fatalf("模板应识别 10 个字段，实际 %d 个", len(detail.Placeholders))
	}
	// 模板副本必须落在应用数据目录，且与源文件不同路径
	storedPath := filepath.Join(root, "templates", detail.Template.ID+".docx")
	if _, err := os.Stat(storedPath); err != nil {
		// StoredFile 是 uuid.docx，这里按实际记录取路径
		tpl, getErr := s.store.GetTemplate(ctx, detail.Template.ID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		storedPath = s.store.TemplatePath(tpl)
		if _, statErr := os.Stat(storedPath); statErr != nil {
			t.Fatalf("模板副本未落盘: %v", statErr)
		}
	}
	if strings.HasPrefix(storedPath, filepath.Dir(realTemplate)) {
		t.Fatal("模板副本不应写回源目录")
	}

	// 2) 生成合同
	res, err := s.con.Generate(ctx, contract.GenerateRequest{
		TemplateID: detail.Template.ID,
		Values:     testValues(),
		OutputDir:  outDir,
	})
	if err != nil {
		t.Fatalf("生成合同失败: %v", err)
	}
	if _, err := os.Stat(res.FilePath); err != nil {
		t.Fatalf("生成的文件不存在: %v", err)
	}

	// 3) 独立校验：产物中不再有占位符，且各部件都带上了正确的值
	insp, err := docx.Inspect(res.FilePath, contract.FieldNames())
	if err != nil {
		t.Fatalf("产物无法读取: %v", err)
	}
	if len(insp.Placeholders()) != 0 {
		t.Fatalf("产物中仍存在占位符: %+v", insp.Placeholders())
	}
	if len(insp.Unknown) != 0 || len(insp.Unclosed) != 0 || len(insp.UnsafeSpans) != 0 {
		t.Fatalf("产物存在校验问题: unknown=%v unclosed=%v unsafe=%v", insp.Unknown, insp.Unclosed, insp.UnsafeSpans)
	}

	srcEntry := readEntry(t, realTemplate, "word/document.xml")
	outEntry := readEntry(t, res.FilePath, "word/document.xml")
	if bytes.Equal(srcEntry, outEntry) {
		t.Fatal("正文未被改写")
	}
	if !bytes.Equal(readEntry(t, realTemplate, "word/styles.xml"), readEntry(t, res.FilePath, "word/styles.xml")) {
		t.Error("样式部件被改写（应保持字节一致）")
	}
	for _, part := range []string{"word/header1.xml", "word/footer1.xml", "word/footnotes.xml"} {
		if bytes.Equal(readEntry(t, realTemplate, part), readEntry(t, res.FilePath, part)) {
			t.Errorf("%s 未被替换", part)
		}
	}

	// 4) 原模板未被修改
	if _, err := docx.Inspect(realTemplate, contract.FieldNames()); err != nil {
		t.Fatalf("源模板被破坏: %v", err)
	}
	if !strings.Contains(string(srcEntry), "{{联系人姓名}}") {
		t.Error("源模板中的占位符丢失")
	}

	// 5) 重名不覆盖：再次生成得到不同文件
	res2, err := s.con.Generate(ctx, contract.GenerateRequest{
		TemplateID: detail.Template.ID,
		Values:     testValues(),
		OutputDir:  outDir,
	})
	if err != nil {
		t.Fatalf("第二次生成失败: %v", err)
	}
	if res2.FilePath == res.FilePath {
		t.Fatal("重名时必须生成新文件，不能覆盖")
	}
	if _, err := os.Stat(res.FilePath); err != nil {
		t.Fatalf("先前的文件被覆盖/删除: %v", err)
	}

	// 6) 校验失败时不产出文件
	bad := testValues()
	bad["联系人邮箱"] = "not-an-email"
	before, _ := os.ReadDir(outDir)
	if _, err := s.con.Generate(ctx, contract.GenerateRequest{
		TemplateID: detail.Template.ID,
		Values:     bad,
		OutputDir:  outDir,
	}); err == nil {
		t.Fatal("邮箱格式错误时必须拒绝生成")
	}
	after, _ := os.ReadDir(outDir)
	if len(after) != len(before) {
		t.Fatal("校验失败时不应产出文件")
	}

	// 7) 生成记录只保存字段名，不保存字段值
	records, err := s.store.ListContracts(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("生成记录数量 = %d，期望 2", len(records))
	}
	if len(records[0].FieldNames) == 0 {
		t.Error("生成记录应保存字段名列表")
	}

	// 8) 重启恢复：关闭后重新打开数据库，模板与记录仍在
	if err := s.store.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := newStack(t, root)
	defer func() { _ = restarted.store.Close() }()
	list, err := restarted.tpl.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "标准合同模板" {
		t.Fatalf("重启后模板列表异常: %+v", list)
	}
	if !list[0].FileExists {
		t.Error("重启后模板文件应可访问")
	}
	records2, err := restarted.store.ListContracts(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records2) != 2 {
		t.Fatalf("重启后生成记录数量 = %d，期望 2", len(records2))
	}

	// 9) 删除模板不影响已生成的合同
	if err := restarted.tpl.Delete(ctx, detail.Template.ID); err != nil {
		t.Fatalf("删除模板失败: %v", err)
	}
	if _, err := os.Stat(res.FilePath); err != nil {
		t.Fatalf("删除模板后已生成的合同不应消失: %v", err)
	}
	listAfter, err := restarted.tpl.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(listAfter) != 0 {
		t.Fatal("删除后模板列表应为空")
	}
}

func TestGenerateFailsWhenTemplateFileMissing(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s := newStack(t, root)
	defer func() { _ = s.store.Close() }()

	detail, err := s.tpl.Import(ctx, realTemplate, "待删除模板")
	if err != nil {
		t.Fatal(err)
	}
	tpl, err := s.store.GetTemplate(ctx, detail.Template.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(s.store.TemplatePath(tpl)); err != nil {
		t.Fatal(err)
	}

	_, err = s.con.Generate(ctx, contract.GenerateRequest{
		TemplateID: detail.Template.ID,
		Values:     testValues(),
		OutputDir:  t.TempDir(),
	})
	if err == nil {
		t.Fatal("模板文件缺失时必须报错")
	}
	if !strings.Contains(err.Error(), "丢失") && !strings.Contains(err.Error(), "无法读取") {
		t.Errorf("错误信息应说明模板不可用，实际: %v", err)
	}
}

func TestImportRejectsNonDocx(t *testing.T) {
	ctx := context.Background()
	s := newStack(t, t.TempDir())
	defer func() { _ = s.store.Close() }()

	bad := filepath.Join(t.TempDir(), "not-a-template.txt")
	if err := os.WriteFile(bad, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.tpl.Import(ctx, bad, ""); err == nil {
		t.Fatal("非 .docx 文件必须被拒绝")
	}

	// 损坏的 docx（合法扩展名但内容不是 zip）
	broken := filepath.Join(t.TempDir(), "broken.docx")
	if err := os.WriteFile(broken, []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.tpl.Import(ctx, broken, ""); err == nil {
		t.Fatal("损坏的 docx 必须被拒绝")
	}
	// 失败后不应残留模板记录（失败回滚）
	list, err := s.tpl.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("导入失败后不应留下记录: %+v", list)
	}
	// 失败后不应残留文件
	entries, err := os.ReadDir(filepath.Join(s.store.Root(), "templates"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("导入失败后不应留下文件: %+v", entries)
	}
}

func readEntry(t *testing.T, path, entry string) []byte {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("打开 %s 失败: %v", path, err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != entry {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, rc); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	t.Fatalf("%s 中不存在部件 %s", path, entry)
	return nil
}

// TestGenerateDocFormatWithRealConverter 验证"输出必须 .doc"这条需求：
// 生成的必须是真 OLE2 .doc（下游按文件头解析也能通过），内容与 .docx 一致，且不留中间文件。
func TestGenerateDocFormatWithRealConverter(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "合同输出")
	s := newStack(t, root)
	defer func() { _ = s.store.Close() }()

	if !s.conv.Available() {
		t.Skip("本机没有 LibreOffice，跳过 .doc 生成用例（CI 上属预期）")
	}

	detail, err := s.tpl.Import(ctx, realTemplate, "标准合同模板")
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.con.Generate(ctx, contract.GenerateRequest{
		TemplateID:   detail.Template.ID,
		Values:       testValues(),
		OutputDir:    outDir,
		OutputFormat: contract.FormatDoc,
		KeepDocxCopy: true,
	})
	if err != nil {
		t.Fatalf("生成 .doc 失败: %v", err)
	}
	if res.ConverterName == "" {
		t.Error("结果应报告实际使用的转换通道")
	}
	if res.Format != contract.FormatDoc {
		t.Fatalf("实际格式 = %s，期望 doc", res.Format)
	}
	if res.Warning != "" {
		t.Fatalf("不应有降级警告: %s", res.Warning)
	}
	if !strings.HasSuffix(strings.ToLower(res.FilePath), ".doc") {
		t.Fatalf("产物扩展名应为 .doc: %s", res.FilePath)
	}

	// 必须是真 OLE2/CFB 文档，而不是改了扩展名的 zip
	f, err := os.Open(res.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	head := make([]byte, 8)
	if _, err := io.ReadFull(f, head); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	want := []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	if !bytes.Equal(head, want) {
		t.Fatalf(".doc 文件头不正确: % x", head)
	}

	// 内容一致性：把 .doc 回读成 .docx，逐字段核对填写值
	back := filepath.Join(t.TempDir(), "back.docx")
	if err := s.conv.ToDocx(ctx, res.FilePath, back); err != nil {
		t.Fatalf("回读 .doc 失败: %v", err)
	}
	text, err := docx.ExtractText(back)
	if err != nil {
		t.Fatal(err)
	}
	values := testValues()
	// 只核对"模板真正用到的字段"（模板里没有 {{联系人电话}} 时不应要求它出现在产物中）
	for field := range detail.Template.FieldUsage {
		if !strings.Contains(text, values[field]) {
			t.Errorf(".doc 中缺少字段 %s 的值", field)
		}
	}
	if strings.Contains(text, "{{") {
		t.Error(".doc 中仍存在未替换的占位符")
	}

	// 保留的 .docx 备份：内容保真、可独立打开
	if res.BackupPath == "" {
		t.Fatal("开启备份时应返回备份路径")
	}
	backupText, err := docx.ExtractText(res.BackupPath)
	if err != nil {
		t.Fatalf("备份文件无法读取: %v", err)
	}
	for field := range detail.Template.FieldUsage {
		if !strings.Contains(backupText, testValues()[field]) {
			t.Errorf("备份中缺少字段 %s 的值", field)
		}
	}

	// 输出目录只应包含目标 .doc 与其 .docx 备份，且没有临时目录残留
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".convert-") || strings.HasPrefix(name, ".staged") {
			t.Errorf("临时文件未清理: %s", name)
		}
		if strings.HasSuffix(strings.ToLower(name), ".docx") {
			if filepath.Join(outDir, name) != res.BackupPath {
				t.Errorf("出现了非预期的 .docx：%s", name)
			}
		}
	}
	if len(entries) != 2 {
		t.Errorf("输出目录应有 1 个 .doc + 1 个 .docx 备份，实际 %d 个文件", len(entries))
	}
}

// TestGenerateDocWithoutBackup 验证关闭备份时不留下 .docx。
func TestGenerateDocWithoutBackup(t *testing.T) {
	ctx := context.Background()
	outDir := t.TempDir()
	s := newStack(t, t.TempDir())
	defer func() { _ = s.store.Close() }()
	if !s.conv.Available() {
		t.Skip("本机没有可用转换通道，跳过")
	}
	detail, err := s.tpl.Import(ctx, realTemplate, "标准合同模板")
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.con.Generate(ctx, contract.GenerateRequest{
		TemplateID:   detail.Template.ID,
		Values:       testValues(),
		OutputDir:    outDir,
		OutputFormat: contract.FormatDoc,
		KeepDocxCopy: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.BackupPath != "" {
		t.Errorf("关闭备份时不应返回备份路径: %s", res.BackupPath)
	}
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 1 {
		t.Errorf("输出目录应只有 1 个 .doc，实际 %d 个", len(entries))
	}
}

// TestGenerateDocWithoutConverter 验证没有转换器时的行为：明确报错、不产出文件、不影响 .docx 路径。
func TestGenerateDocWithoutConverter(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	outDir := t.TempDir()
	s := newStack(t, root)
	defer func() { _ = s.store.Close() }()

	// 用一个不存在的转换器覆盖
	s.con = contract.NewService(s.store, converter.NewWithBinary(filepath.Join(root, "convert"), filepath.Join(root, "不存在的soffice")))

	detail, err := s.tpl.Import(ctx, realTemplate, "标准合同模板")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.con.Generate(ctx, contract.GenerateRequest{
		TemplateID:   detail.Template.ID,
		Values:       testValues(),
		OutputDir:    outDir,
		OutputFormat: contract.FormatDoc,
	})
	if err == nil {
		t.Fatal("没有转换器时必须拒绝生成 .doc")
	}
	if !strings.Contains(err.Error(), "LibreOffice") {
		t.Errorf("错误信息应提示安装 LibreOffice，实际: %v", err)
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("拒绝生成时不应留下任何文件: %+v", entries)
	}

	// .docx 路径不受影响
	res, err := s.con.Generate(ctx, contract.GenerateRequest{
		TemplateID:   detail.Template.ID,
		Values:       testValues(),
		OutputDir:    outDir,
		OutputFormat: contract.FormatDocx,
	})
	if err != nil {
		t.Fatalf("未指定转换器时 .docx 仍应可生成: %v", err)
	}
	if res.Format != contract.FormatDocx {
		t.Errorf("实际格式 = %s，期望 docx", res.Format)
	}
}

// TestNormalizeFormat 覆盖非法格式。
func TestNormalizeFormatRejectsUnknown(t *testing.T) {
	ctx := context.Background()
	s := newStack(t, t.TempDir())
	defer func() { _ = s.store.Close() }()
	detail, err := s.tpl.Import(ctx, realTemplate, "标准合同模板")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.con.Generate(ctx, contract.GenerateRequest{
		TemplateID:   detail.Template.ID,
		Values:       testValues(),
		OutputDir:    t.TempDir(),
		OutputFormat: "pdf",
	}); err == nil {
		t.Fatal("不支持的输出格式应被拒绝")
	}
}

// TestImportDocTemplate 验证「只有 .doc 模板」的场景：
// .doc 被自动转换为 .docx 导入，原始 .doc 保留，占位符识别正常，且能直接生成合同。
func TestImportDocTemplate(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	outDir := t.TempDir()
	s := newStack(t, root)
	defer func() { _ = s.store.Close() }()

	if !s.conv.Available() {
		t.Skip("本机没有 LibreOffice，跳过 .doc 导入用例（CI 上属预期）")
	}

	// 用真实语料现场造一个 .doc（Word 97-2003 二进制）
	docxSrc := filepath.Join(t.TempDir(), "source.docx")
	if err := copyFileForTest(realTemplate, docxSrc); err != nil {
		t.Fatal(err)
	}
	docSrc := filepath.Join(t.TempDir(), "旧版模板.doc")
	if err := s.conv.ToDoc(ctx, docxSrc, docSrc); err != nil {
		t.Fatalf("生成 .doc 测试文件失败: %v", err)
	}

	detail, err := s.tpl.Import(ctx, docSrc, "旧版合同模板")
	if err != nil {
		t.Fatalf("导入 .doc 模板失败: %v", err)
	}
	if detail.Template.SourceFormat != store.SourceFormatDoc {
		t.Errorf("SourceFormat = %q，期望 doc", detail.Template.SourceFormat)
	}
	if len(detail.Placeholders) != 10 {
		t.Fatalf("占位符字段数 = %d，期望 10", len(detail.Placeholders))
	}
	if len(detail.Problems) != 0 {
		t.Errorf("模板不应有问题: %v", detail.Problems)
	}
	if detail.Warning == "" {
		t.Error("由 .doc 转换而来应当给出提示（版式核对）")
	}

	// 原始 .doc 必须保留在数据目录
	tpl, err := s.store.GetTemplate(ctx, detail.Template.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tpl.OriginalFile == "" {
		t.Fatal("应保留原始 .doc")
	}
	originalPath := s.store.OriginalFilePath(tpl)
	head := make([]byte, 8)
	f, err := os.Open(originalPath)
	if err != nil {
		t.Fatalf("原始 .doc 未保留: %v", err)
	}
	if _, err := io.ReadFull(f, head); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if !bytes.Equal(head, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}) {
		t.Errorf("保留的原始文件不是 .doc: % x", head)
	}

	// 用 .doc 导入的模板也要能正常生成合同
	res, err := s.con.Generate(ctx, contract.GenerateRequest{
		TemplateID: detail.Template.ID,
		Values:     testValues(),
		OutputDir:  outDir,
	})
	if err != nil {
		t.Fatalf("用 .doc 导入的模板生成失败: %v", err)
	}
	text, err := docx.ExtractText(res.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	for field := range detail.Template.FieldUsage {
		if !strings.Contains(text, testValues()[field]) {
			t.Errorf("生成结果缺少字段 %s", field)
		}
	}

	// 删除模板时，保留的原始 .doc 也应一并清理
	if err := s.tpl.Delete(ctx, detail.Template.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(originalPath); err == nil {
		t.Error("删除模板后原始 .doc 应被清理")
	}
}

// TestImportDocWithoutConverter 验证没有 LibreOffice 时的 .doc 导入行为。
func TestImportDocWithoutConverter(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s := newStack(t, root)
	defer func() { _ = s.store.Close() }()

	// 用一个不存在的转换器
	noConv := converter.NewWithBinary(filepath.Join(root, "convert"), filepath.Join(root, "missing-soffice"))
	s.tpl = template.NewService(s.store, contract.FieldNames(), noConv)

	docPath := filepath.Join(t.TempDir(), "模板.doc")
	if err := os.WriteFile(docPath, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := s.tpl.Import(ctx, docPath, "")
	if err == nil {
		t.Fatal("没有转换器时导入 .doc 应报错")
	}
	if !strings.Contains(err.Error(), "LibreOffice") {
		t.Errorf("错误信息应提示需要 LibreOffice，实际: %v", err)
	}
	// 不应留下记录或文件
	list, err := s.tpl.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("失败后不应留下模板记录: %+v", list)
	}
	entries, err := os.ReadDir(filepath.Join(root, "templates"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("失败后不应留下文件: %+v", entries)
	}
}

func copyFileForTest(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
