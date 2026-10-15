package e2e

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sheep-ah/inspiration/internal/config"
	"github.com/sheep-ah/inspiration/internal/contract"
	"github.com/sheep-ah/inspiration/internal/docx"
	"github.com/sheep-ah/inspiration/internal/store"
)

// 阶段 3：异常路径与边界场景。目标——任何异常都必须给出可理解的中文提示，
// 且不产出半成品文件、不破坏已有数据。

func TestGenerateToReadOnlyDirFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root 下权限位无效")
	}
	ctx := context.Background()
	s := newStack(t, t.TempDir())
	defer func() { _ = s.store.Close() }()

	detail, err := s.tpl.Import(ctx, realTemplate, "只读目录测试")
	if err != nil {
		t.Fatal(err)
	}
	roDir := filepath.Join(t.TempDir(), "只读目录")
	if err := os.MkdirAll(roDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(roDir, 0o755) }()

	_, err = s.con.Generate(ctx, contract.GenerateRequest{
		TemplateID: detail.Template.ID,
		Values:     testValues(),
		OutputDir:  roDir,
	})
	if err == nil {
		t.Fatal("只读目录必须报错")
	}
	if !strings.Contains(err.Error(), "不可写") && !strings.Contains(err.Error(), "无法") {
		t.Errorf("错误信息应说明是权限问题，实际: %v", err)
	}
	entries, _ := os.ReadDir(roDir)
	if len(entries) != 0 {
		t.Fatalf("只读目录不应产生文件: %+v", entries)
	}
}

func TestGenerateWhenOutputPathIsAFile(t *testing.T) {
	ctx := context.Background()
	s := newStack(t, t.TempDir())
	defer func() { _ = s.store.Close() }()

	detail, err := s.tpl.Import(ctx, realTemplate, "路径为文件测试")
	if err != nil {
		t.Fatal(err)
	}
	fileAsDir := filepath.Join(t.TempDir(), "其实是个文件")
	if err := os.WriteFile(fileAsDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = s.con.Generate(ctx, contract.GenerateRequest{
		TemplateID: detail.Template.ID,
		Values:     testValues(),
		OutputDir:  fileAsDir,
	})
	if err == nil {
		t.Fatal("保存位置是文件时必须报错")
	}
	if !strings.Contains(err.Error(), "不是") && !strings.Contains(err.Error(), "文件夹") && !strings.Contains(err.Error(), "无法") {
		t.Errorf("错误信息应说明路径类型不对，实际: %v", err)
	}
}

func TestGenerateWhenTemplateCorruptedExternally(t *testing.T) {
	ctx := context.Background()
	s := newStack(t, t.TempDir())
	defer func() { _ = s.store.Close() }()

	detail, err := s.tpl.Import(ctx, realTemplate, "被破坏的模板")
	if err != nil {
		t.Fatal(err)
	}
	tpl, err := s.store.GetTemplate(ctx, detail.Template.ID)
	if err != nil {
		t.Fatal(err)
	}
	// 外部把模板文件改成垃圾内容
	if err := os.WriteFile(s.store.TemplatePath(tpl), []byte("这不是一个 docx"), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	_, err = s.con.Generate(ctx, contract.GenerateRequest{
		TemplateID: detail.Template.ID,
		Values:     testValues(),
		OutputDir:  outDir,
	})
	if err == nil {
		t.Fatal("模板被破坏后必须报错")
	}
	if !strings.Contains(err.Error(), "DOCX") && !strings.Contains(err.Error(), "无法读取") {
		t.Errorf("错误信息应说明模板不可读，实际: %v", err)
	}
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 0 {
		t.Fatalf("模板损坏时不应产出文件: %+v", entries)
	}
	// 模板详情应把问题显示出来
	d, err := s.tpl.Detail(ctx, detail.Template.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Problems) == 0 {
		t.Error("模板详情应报告问题")
	}
}

func TestVeryLongAndUnicodeFileName(t *testing.T) {
	ctx := context.Background()
	s := newStack(t, t.TempDir())
	defer func() { _ = s.store.Close() }()
	detail, err := s.tpl.Import(ctx, realTemplate, "文件名测试")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		file string
	}{
		{"超长文件名", strings.Repeat("很长的合同名称", 40)},
		{"特殊字符", `合同:2026/年度*合同?<测试>|"引号"`},
		{"emoji 与全角", "合同📄（正式版）２０２６"},
		{"只给扩展名", ".docx"},
	}
	outDir := t.TempDir()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := s.con.Generate(ctx, contract.GenerateRequest{
				TemplateID: detail.Template.ID,
				Values:     testValues(),
				OutputDir:  outDir,
				FileName:   c.file,
			})
			if err != nil {
				t.Fatalf("生成失败: %v", err)
			}
			info, err := os.Stat(res.FilePath)
			if err != nil {
				t.Fatalf("产物不存在: %v", err)
			}
			if info.Size() == 0 {
				t.Error("产物为空")
			}
			// 文件名不得包含路径分隔符
			base := filepath.Base(res.FilePath)
			if strings.ContainsAny(base, `/\`) {
				t.Errorf("文件名中不应包含路径分隔符: %s", base)
			}
		})
	}
}

func TestDatabaseCorruptedFailsClearly(t *testing.T) {
	root := t.TempDir()
	s := newStack(t, root)
	// 写入垃圾后关闭
	if err := s.store.Close(); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, "inspiration.db")
	if err := os.WriteFile(dbPath, bytes.Repeat([]byte("NOT-A-SQLITE-FILE"), 100), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.At(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(cfg); err == nil {
		t.Fatal("数据库损坏时必须报错")
	}
}

// TestLongTextAndMultilineValues 覆盖中文长文本、换行值与特殊字符。
func TestLongTextAndMultilineValues(t *testing.T) {
	ctx := context.Background()
	s := newStack(t, t.TempDir())
	defer func() { _ = s.store.Close() }()
	detail, err := s.tpl.Import(ctx, realTemplate, "长文本测试")
	if err != nil {
		t.Fatal(err)
	}

	longAddress := strings.Repeat("北京市海淀区中关村大街1号院3号楼5层501室，", 12) // 约 300 字
	values := testValues()
	values["实际经营地址"] = longAddress
	values["开户银行"] = "中国工商银行股份有限公司北京中关村支行（自贸试验区支行）"
	values["联系人姓名"] = "张三 & 李四 <测试> \"引号\" '单引号'"
	values["账户号"] = "0001234567890123456"
	values["开票地址"] = "上海市浦东新区世纪大道100号A座20层\n备用地址：北京市朝阳区建国路88号"

	res, err := s.con.Generate(ctx, contract.GenerateRequest{
		TemplateID: detail.Template.ID,
		Values:     values,
		OutputDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	text, err := docx.ExtractText(res.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, longAddress) {
		t.Error("长中文地址未完整写入（可能被截断）")
	}
	if !strings.Contains(text, "张三 & 李四 <测试>") {
		t.Error("特殊字符未正确转义/写入")
	}
	if !strings.Contains(text, "0001234567890123456") {
		t.Error("账户号前导零丢失")
	}
	// 多行值：换行应转成 <w:br/>，两段文字都要在
	if !strings.Contains(text, "上海市浦东新区世纪大道100号A座20层") || !strings.Contains(text, "备用地址：北京市朝阳区建国路88号") {
		t.Error("多行值未按预期写入")
	}
	// 产物必须仍是合法可读的 DOCX
	if _, err := docx.Inspect(res.FilePath, contract.FieldNames()); err != nil {
		t.Fatalf("产物不可读: %v", err)
	}
}

// TestRestartAfterUncleanShutdown 模拟异常退出（未 Close 数据库）后重启。
func TestRestartAfterUncleanShutdown(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s := newStack(t, root)
	detail, err := s.tpl.Import(ctx, realTemplate, "异常退出测试")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.con.Generate(ctx, contract.GenerateRequest{
		TemplateID: detail.Template.ID,
		Values:     testValues(),
		OutputDir:  t.TempDir(),
	}); err != nil {
		t.Fatal(err)
	}
	// 不调用 Close，直接模拟进程被杀（连接未正常关闭，WAL 可能未 checkpoint）
	// 再次打开同一个数据目录
	s2 := newStack(t, root)
	defer func() { _ = s2.store.Close() }()
	list, err := s2.tpl.List(ctx, "")
	if err != nil {
		t.Fatalf("异常退出后重新打开失败: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("异常退出后模板丢失: %+v", list)
	}
	records, err := s2.store.ListContracts(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("异常退出后生成记录丢失: %+v", records)
	}
}

// TestImportWrongFormat 覆盖各种错误类型的导入输入。
func TestImportWrongFormat(t *testing.T) {
	ctx := context.Background()
	s := newStack(t, t.TempDir())
	defer func() { _ = s.store.Close() }()

	cases := []struct {
		name string
		file string
		data []byte
	}{
		{"txt 文件", "a.txt", []byte("hello")},
		{"pdf 扩展名", "a.pdf", []byte("%PDF-1.4")},
		{"docm 宏文件", "a.docm", []byte("PK\x03\x04")},
		{"空 docx", "a.docx", []byte{}},
		{"zip 但不是 docx", "b.docx", []byte("PK\x03\x04不是真的zip")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), c.file)
			if err := os.WriteFile(path, c.data, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := s.tpl.Import(ctx, path, ""); err == nil {
				t.Fatalf("%s 必须被拒绝", c.name)
			}
		})
	}
	// 全部失败后不应留下任何记录或文件
	list, _ := s.tpl.List(ctx, "")
	if len(list) != 0 {
		t.Fatalf("失败导入不应留下记录: %+v", list)
	}
	entries, _ := os.ReadDir(filepath.Join(s.store.Root(), "templates"))
	if len(entries) != 0 {
		t.Fatalf("失败导入不应留下文件: %+v", entries)
	}
}
