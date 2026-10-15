// Package template 提供模板管理能力：导入（含 .doc 自动转换）、列表、详情、重命名、删除。
package template

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sheep-ah/inspiration/internal/converter"
	"github.com/sheep-ah/inspiration/internal/docx"
	"github.com/sheep-ah/inspiration/internal/id"
	"github.com/sheep-ah/inspiration/internal/store"
)

// 支持的扩展名：.docx 直接导入；.doc 通过本机 LibreOffice 自动转换为 .docx 后导入。
const (
	ExtDocx = ".docx"
	ExtDoc  = ".doc"
)

// Service 是模板服务。
type Service struct {
	store     *store.Store
	supported []string
	converter *converter.Converter
}

// NewService 创建模板服务。supported 是应用支持的字段名列表，conv 用于 .doc 导入（可为 nil）。
func NewService(st *store.Store, supported []string, conv *converter.Converter) *Service {
	return &Service{store: st, supported: supported, converter: conv}
}

// PlaceholderView 是界面展示用的占位符信息。
type PlaceholderView struct {
	Name    string   `json:"name"`
	Count   int      `json:"count"`
	Parts   []string `json:"parts"`
	Unknown bool     `json:"unknown"`
}

// Detail 是模板详情（含占位符分布与校验结论）。
type Detail struct {
	Template     store.Template    `json:"template"`
	Placeholders []PlaceholderView `json:"placeholders"`
	Problems     []string          `json:"problems"`
	Missing      []string          `json:"missing"`
	// Warning 非空表示"导入成功但需要注意"（例如 .doc 转换而来、未识别到占位符）。
	Warning string `json:"warning"`
}

// ErrUnsupportedType 表示文件类型不被支持。
var ErrUnsupportedType = errors.New("只支持 .docx 或 .doc 格式的模板文件")

// Import 导入模板：校验 → 复制到数据目录 → 扫描占位符 → 入库。
// .doc 会先用本机 LibreOffice 转换成 .docx（原始 .doc 一并保留备查）。
// 任一步失败都会清理已落盘的文件，不留下"有记录没文件"或"有文件没记录"的状态。
func (s *Service) Import(ctx context.Context, srcPath, name string) (*Detail, error) {
	srcPath = strings.TrimSpace(srcPath)
	if srcPath == "" {
		return nil, errors.New("未选择模板文件")
	}
	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, fmt.Errorf("无法读取所选文件：%w", err)
	}
	if info.IsDir() {
		return nil, errors.New("请选择 Word 文件而不是文件夹")
	}
	if info.Size() == 0 {
		return nil, errors.New("所选文件为空")
	}
	if info.Size() > store.MaxTemplateBytes {
		return nil, fmt.Errorf("文件过大（%d MB），模板上限为 %d MB", info.Size()>>20, store.MaxTemplateBytes>>20)
	}

	ext := strings.ToLower(filepath.Ext(srcPath))
	if ext != ExtDocx && ext != ExtDoc {
		return nil, ErrUnsupportedType
	}

	display := strings.TrimSpace(name)
	if display == "" {
		display = strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath))
	}
	if len([]rune(display)) > 100 {
		return nil, errors.New("模板名称过长（最多 100 个字符）")
	}

	// .doc：先转成 .docx，再做后续（扫描/入库）流程
	importPath := srcPath
	sourceFormat := store.SourceFormatDocx
	var cleanup []string
	warning := ""
	if ext == ExtDoc {
		if s.converter == nil || !s.converter.Available() {
			return nil, errors.New("导入 .doc 模板需要本机安装 LibreOffice（用于自动转换为 .docx）；" +
				"也可以先在 Word / WPS 中另存为 .docx 再上传")
		}
		tmpDir, err := os.MkdirTemp("", "inspiration-doc-import-*")
		if err != nil {
			return nil, fmt.Errorf("无法创建临时目录：%w", err)
		}
		defer os.RemoveAll(tmpDir)

		converted := filepath.Join(tmpDir, "converted.docx")
		if err := s.converter.ToDocx(ctx, srcPath, converted); err != nil {
			return nil, fmt.Errorf("自动转换 .doc 失败：%w", err)
		}
		importPath = converted
		sourceFormat = store.SourceFormatDoc
		warning = "该模板由 .doc 自动转换而来：文字与占位符已保留，但版式（尤其是文本框、图文框位置）可能与原文件不同，" +
			"请先用 Word / WPS 打开数据目录中的转换结果核对后再使用。原始 .doc 已一并保留。"
	}

	tpl := &store.Template{
		ID:           id.New(),
		Name:         display,
		StoredFile:   id.New() + ExtDocx,
		SourceFormat: sourceFormat,
	}

	size, sum, err := s.store.PutTemplateFile(importPath, tpl.StoredFile)
	if err != nil {
		return nil, err
	}
	tpl.SizeBytes, tpl.SHA256 = size, sum
	storedPath := s.store.TemplatePath(tpl)
	cleanup = append(cleanup, storedPath)

	inspection, err := docx.Inspect(storedPath, s.supported)
	if err != nil {
		removeAll(cleanup)
		return nil, fmt.Errorf("模板无法读取：%w", err)
	}

	// .doc 导入时保留原始文件
	if sourceFormat == store.SourceFormatDoc {
		originalFile := id.New() + ExtDoc
		if err := s.store.PutOriginalFile(srcPath, originalFile); err != nil {
			removeAll(cleanup)
			return nil, err
		}
		tpl.OriginalFile = originalFile
		cleanup = append(cleanup, s.store.OriginalFilePath(tpl))
	}

	tpl.Placeholders = toStorePlaceholders(inspection)
	tpl.FieldUsage = inspection.FieldUsage
	if err := s.store.InsertTemplate(ctx, tpl); err != nil {
		removeAll(cleanup)
		return nil, err
	}

	detail := &Detail{
		Template:     *tpl,
		Placeholders: toViews(inspection),
		Problems:     inspection.Problems(),
		Missing:      inspection.Missing,
		Warning:      warning,
	}
	if len(inspection.Found) == 0 {
		extra := "模板中没有识别到任何 {{字段名}} 占位符，无法用于生成合同。" +
			"请在模板中按「{{联系人姓名}}」这样的写法插入占位符（字段名必须与应用的十个字段一致）后重新上传。"
		if detail.Warning != "" {
			detail.Warning += "\n" + extra
		} else {
			detail.Warning = extra
		}
	}
	return detail, nil
}

// List 返回模板列表（按名称模糊搜索）。
func (s *Service) List(ctx context.Context, query string) ([]store.Template, error) {
	items, err := s.store.ListTemplates(ctx, query)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []store.Template{}
	}
	return items, nil
}

// Detail 返回模板详情；若模板仍可读则顺带刷新占位符信息。
func (s *Service) Detail(ctx context.Context, templateID string) (*Detail, error) {
	tpl, err := s.store.GetTemplate(ctx, templateID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, errors.New("模板不存在或已被删除")
		}
		return nil, err
	}
	detail := &Detail{Template: *tpl, Placeholders: []PlaceholderView{}}
	if tpl.SourceFormat == store.SourceFormatDoc {
		detail.Warning = "该模板由 .doc 自动转换而来，版式可能与原文件有差异。"
	}
	if !tpl.FileExists {
		detail.Problems = []string{"模板文件已丢失，请删除该模板后重新上传"}
		return detail, nil
	}
	inspection, err := docx.Inspect(s.store.TemplatePath(tpl), s.supported)
	if err != nil {
		detail.Problems = []string{fmt.Sprintf("模板无法读取：%v", err)}
		return detail, nil
	}
	detail.Placeholders = toViews(inspection)
	detail.Problems = inspection.Problems()
	detail.Missing = inspection.Missing
	if len(inspection.Found) == 0 {
		detail.Warning = "模板中没有识别到任何 {{字段名}} 占位符，无法用于生成合同。"
	}

	tpl.Placeholders = toStorePlaceholders(inspection)
	tpl.FieldUsage = inspection.FieldUsage
	// 元数据刷新失败不影响本次查看
	_ = s.store.UpdateTemplateInspection(ctx, tpl.ID, tpl.Placeholders, tpl.FieldUsage)
	return detail, nil
}

// Rename 修改模板名称。
func (s *Service) Rename(ctx context.Context, templateID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("模板名称不能为空")
	}
	if len([]rune(name)) > 100 {
		return errors.New("模板名称过长（最多 100 个字符）")
	}
	if err := s.store.RenameTemplate(ctx, templateID, name); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return errors.New("模板不存在或已被删除")
		}
		return err
	}
	return nil
}

// Delete 删除模板（软删除，不影响已生成的合同文件；连同保留的原始 .doc 一并删除）。
func (s *Service) Delete(ctx context.Context, templateID string) error {
	if err := s.store.DeleteTemplate(ctx, templateID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return errors.New("模板不存在或已被删除")
		}
		return err
	}
	return nil
}

// SupportedFields 返回应用支持的字段名（供界面展示）。
func (s *Service) SupportedFields() []string { return s.supported }

// AcceptableExts 返回可上传的扩展名（界面提示用）。
func AcceptableExts() []string { return []string{ExtDocx, ExtDoc} }

func removeAll(paths []string) {
	for _, p := range paths {
		if p == "" {
			continue
		}
		_ = os.Remove(p)
	}
}

func toStorePlaceholders(i *docx.Inspection) []store.PlaceholderInfo {
	byName := map[string]*store.PlaceholderInfo{}
	var order []string
	for _, p := range i.Found {
		info, ok := byName[p.Name]
		if !ok {
			info = &store.PlaceholderInfo{Name: p.Name}
			byName[p.Name] = info
			order = append(order, p.Name)
		}
		info.Count++
		if !containsString(info.Parts, p.Part) {
			info.Parts = append(info.Parts, p.Part)
		}
	}
	out := make([]store.PlaceholderInfo, 0, len(order))
	for _, name := range order {
		info := byName[name]
		sort.Strings(info.Parts)
		out = append(out, *info)
	}
	return out
}

func toViews(i *docx.Inspection) []PlaceholderView {
	unknown := map[string]bool{}
	for _, p := range i.Unknown {
		unknown[p.Name] = true
	}
	byName := map[string]*PlaceholderView{}
	var order []string
	for _, p := range i.Found {
		v, ok := byName[p.Name]
		if !ok {
			v = &PlaceholderView{Name: p.Name, Unknown: unknown[p.Name]}
			byName[p.Name] = v
			order = append(order, p.Name)
		}
		v.Count++
		if !containsString(v.Parts, p.Part) {
			v.Parts = append(v.Parts, p.Part)
		}
	}
	out := make([]PlaceholderView, 0, len(order))
	for _, name := range order {
		v := byName[name]
		sort.Strings(v.Parts)
		out = append(out, *v)
	}
	return out
}

func containsString(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
