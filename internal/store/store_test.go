package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/sheep-ah/inspiration/internal/config"
)

func TestFuzzyMatch(t *testing.T) {
	cases := []struct {
		name, query string
		want        bool
	}{
		{"标准合同模板", "", true},
		{"标准合同模板", "合同", true},
		{"标准合同模板", "标准模板", true}, // 允许跳字
		{"标准合同模板", "模板合同", false},
		{"Standard Contract", "standard", true},
		{"标准合同模板", "劳动合同", false},
	}
	for _, c := range cases {
		if got := fuzzyMatch(c.name, normalizeQuery(c.query)); got != c.want {
			t.Errorf("fuzzyMatch(%q,%q) = %v，期望 %v", c.name, c.query, got, c.want)
		}
	}
}

func TestSanitizeFileName(t *testing.T) {
	cases := map[string]string{
		"合同/2026":    "合同-2026",
		"a\\b:c*d?e": "a-b-c-d-e",
		"  合同  ":     "合同",
		"":           "合同",
		"CON":        "_CON",
		"合同\n模板":     "合同模板", // 控制字符被移除
		"..":         "合同",
	}
	for in, want := range cases {
		if got := SanitizeFileName(in); got != want {
			t.Errorf("SanitizeFileName(%q) = %q，期望 %q", in, got, want)
		}
	}
	long := strings.Repeat("字", 300)
	if got := SanitizeFileName(long); len([]rune(got)) > 120 {
		t.Errorf("超长文件名未被截断: %d", len([]rune(got)))
	}
}

func TestUniquePathNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	first := UniquePath(dir, "合同", ".docx")
	if filepath.Base(first) != "合同.docx" {
		t.Fatalf("首个路径异常: %s", first)
	}
	if err := os.WriteFile(first, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	second := UniquePath(dir, "合同", ".docx")
	if second == first {
		t.Fatal("重名时必须给出新路径")
	}
	if filepath.Base(second) != "合同 (2).docx" {
		t.Fatalf("第二个路径异常: %s", second)
	}
	if err := os.WriteFile(second, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := filepath.Base(UniquePath(dir, "合同", ".docx")); got != "合同 (3).docx" {
		t.Fatalf("第三个路径异常: %s", got)
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	cfg, err := config.At(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestTemplateLifecycleAndSearch(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	src := filepath.Join(t.TempDir(), "模板.docx")
	if err := os.WriteFile(src, []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	tpl := &Template{Name: "标准合同模板", StoredFile: "abc.docx"}
	if err := st.InsertTemplate(ctx, tpl); err != nil {
		t.Fatal(err)
	}

	list, err := st.ListTemplates(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "标准合同模板" {
		t.Fatalf("列表异常: %+v", list)
	}

	// 模糊搜索
	hit, err := st.ListTemplates(ctx, "标准")
	if err != nil {
		t.Fatal(err)
	}
	if len(hit) != 1 {
		t.Fatalf("关键词应命中: %+v", hit)
	}
	miss, err := st.ListTemplates(ctx, "劳动")
	if err != nil {
		t.Fatal(err)
	}
	if len(miss) != 0 {
		t.Fatalf("无关关键词不应命中: %+v", miss)
	}

	// 重命名
	if err := st.RenameTemplate(ctx, tpl.ID, "新版合同模板"); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetTemplate(ctx, tpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "新版合同模板" {
		t.Fatalf("重命名未生效: %s", got.Name)
	}

	// 软删除后不可见
	if err := st.DeleteTemplate(ctx, tpl.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetTemplate(ctx, tpl.ID); err == nil {
		t.Fatal("删除后不应再读到模板")
	}
	after, err := st.ListTemplates(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("删除后列表应为空: %+v", after)
	}
}

func TestContractRecordsStoreFieldNamesOnly(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.AddContract(ctx, &Contract{
		ID:           "c1",
		TemplateID:   "t1",
		TemplateName: "标准合同模板",
		FileName:     "合同.docx",
		FilePath:     "/tmp/合同.docx",
		SHA256:       "deadbeef",
		FieldNames:   []string{"联系人姓名", "电话"},
	}); err != nil {
		t.Fatal(err)
	}
	records, err := st.ListContracts(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("记录数量 = %d", len(records))
	}
	if len(records[0].FieldNames) != 2 {
		t.Fatalf("字段名列表异常: %+v", records[0].FieldNames)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if _, ok := st.Setting(ctx, "lastOutputDir"); ok {
		t.Fatal("未写入的设置不应存在")
	}
	if err := st.SetSetting(ctx, "lastOutputDir", "/tmp/contracts"); err != nil {
		t.Fatal(err)
	}
	v, ok := st.Setting(ctx, "lastOutputDir")
	if !ok || v != "/tmp/contracts" {
		t.Fatalf("设置读取异常: %q %v", v, ok)
	}
	if err := st.SetSetting(ctx, "lastOutputDir", "/tmp/other"); err != nil {
		t.Fatal(err)
	}
	v, _ = st.Setting(ctx, "lastOutputDir")
	if v != "/tmp/other" {
		t.Fatalf("设置更新失败: %q", v)
	}
}

func TestMigrateRejectsNewerSchema(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if _, err := st.db.ExecContext(ctx, `UPDATE meta SET value='99' WHERE key='schema_version'`); err != nil {
		t.Fatal(err)
	}
	if err := st.migrate(); err == nil {
		t.Fatal("数据库版本高于应用时应报错，避免误操作新数据")
	}
}

func TestSanitizeFileNameRespectsByteLimit(t *testing.T) {
	// Linux 文件系统限制单个文件名 255 字节；中文 3 字节/字、emoji 4 字节/字。
	// 这里保证产物名（含扩展名与重名后缀）有充足余量。
	cases := []string{
		strings.Repeat("很长的合同名称", 50),
		strings.Repeat("📄", 100),
		strings.Repeat("ＡＢＣ", 60),
	}
	for _, in := range cases {
		got := SanitizeFileName(in)
		if len(got) > MaxNameBytes {
			t.Errorf("文件名未被限制到 %d 字节：实际 %d 字节", MaxNameBytes, len(got))
		}
		if !utf8.ValidString(got) {
			t.Errorf("截断破坏了 UTF-8 编码：%q", got)
		}
		if got == "" {
			t.Errorf("截断后不应为空")
		}
	}
	// 常见长度的中文名不受影响
	short := "合同-示例科技（北京）有限公司-20261016"
	if got := SanitizeFileName(short); got != short {
		t.Errorf("正常长度文件名不应被改动：%q → %q", short, got)
	}
}

func TestUniquePathWithLongChineseNameIsCreatable(t *testing.T) {
	dir := t.TempDir()
	base := SanitizeFileName(strings.Repeat("很长的合同名称", 50))
	path := UniquePath(dir, base, ".docx")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("按清洗后的文件名创建文件失败（文件名 %d 字节）：%v", len(filepath.Base(path)), err)
	}
	second := UniquePath(dir, base, ".docx")
	if err := os.WriteFile(second, []byte("x"), 0o644); err != nil {
		t.Fatalf("重名后缀后仍应可创建：%v", err)
	}
}
