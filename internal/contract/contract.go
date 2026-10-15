// Package contract 定义合同字段模型、校验与生成流程。
package contract

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sheep-ah/inspiration/internal/converter"
	"github.com/sheep-ah/inspiration/internal/docx"
	"github.com/sheep-ah/inspiration/internal/id"
	"github.com/sheep-ah/inspiration/internal/store"
)

// 输出格式。
const (
	FormatDocx = "docx"
	FormatDoc  = "doc"
)

// normalizeFormat 归一化输出格式；空值默认为 docx。
func normalizeFormat(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", FormatDocx:
		return FormatDocx, nil
	case FormatDoc:
		return FormatDoc, nil
	default:
		return "", fmt.Errorf("不支持的输出格式：%s（仅支持 docx / doc）", v)
	}
}

// FieldDef 描述一个基础字段。
type FieldDef struct {
	Key      string `json:"key"`      // 占位符中的字段名，如 联系人姓名
	Label    string `json:"label"`    // 界面标签
	Hint     string `json:"hint"`     // 输入提示
	MaxRunes int    `json:"maxRunes"` // 合理长度上限（防止异常长文本）
}

// Fields 是十个必填基础字段（顺序即界面顺序）。
var Fields = []FieldDef{
	{Key: "联系人姓名", Label: "联系人姓名", Hint: "如：张三", MaxRunes: 64},
	{Key: "法人代表", Label: "法人代表", Hint: "如：李四", MaxRunes: 64},
	{Key: "实际经营地址", Label: "实际经营地址", Hint: "如：北京市海淀区中关村大街1号院3号楼5层501室", MaxRunes: 300},
	{Key: "联系人邮箱", Label: "联系人邮箱", Hint: "如：zhangsan@example.com", MaxRunes: 128},
	{Key: "营业执照名称", Label: "营业执照名称", Hint: "与营业执照完全一致的公司全称", MaxRunes: 200},
	{Key: "纳税人识别号", Label: "纳税人识别号", Hint: "15/17/18/20 位字母数字组合", MaxRunes: 40},
	{Key: "电话", Label: "电话", Hint: "固定电话，如：010-88886666", MaxRunes: 40},
	{Key: "联系人电话", Label: "联系人电话", Hint: "联系人手机号，如：13800000000", MaxRunes: 40},
	{Key: "开户银行", Label: "开户银行", Hint: "含支行全称，如：中国工商银行股份有限公司北京中关村支行", MaxRunes: 200},
	{Key: "账户号", Label: "账户号", Hint: "银行账号按文本处理，前导零会保留", MaxRunes: 64},
	{Key: "开票地址", Label: "开票地址", Hint: "如：北京市海淀区中关村大街1号院3号楼5层", MaxRunes: 300},
}

// FieldNames 返回全部字段名。
func FieldNames() []string {
	out := make([]string, 0, len(Fields))
	for _, f := range Fields {
		out = append(out, f.Key)
	}
	return out
}

// Values 是用户填写的字段值（字段名 -> 值）。
type Values map[string]string

// ValuesFromMap 从任意 map 中提取本应用支持的字段（忽略未知键）。
func ValuesFromMap(m map[string]string) Values {
	v := Values{}
	for _, f := range Fields {
		v[f.Key] = strings.TrimSpace(m[f.Key])
	}
	return v
}

// FieldError 描述一条字段级校验错误。
type FieldError struct {
	Key     string `json:"key"`
	Message string `json:"message"`
}

var emailRe = regexp.MustCompile(`^[A-Za-z0-9._%+\-']+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)

// Validate 校验必填与格式；返回空切片表示通过。
func (v Values) Validate() []FieldError {
	var errs []FieldError
	for _, f := range Fields {
		val := strings.TrimSpace(v[f.Key])
		switch {
		case val == "":
			errs = append(errs, FieldError{Key: f.Key, Message: f.Label + "为必填项"})
		case f.MaxRunes > 0 && len([]rune(val)) > f.MaxRunes:
			errs = append(errs, FieldError{Key: f.Key, Message: fmt.Sprintf("%s长度超出限制（最多 %d 个字符）", f.Label, f.MaxRunes)})
		}
	}
	if email := strings.TrimSpace(v["联系人邮箱"]); email != "" && !emailRe.MatchString(email) {
		errs = append(errs, FieldError{Key: "联系人邮箱", Message: "联系人邮箱格式不正确"})
	}
	if tax := strings.TrimSpace(v["纳税人识别号"]); tax != "" && len([]rune(tax)) < 15 {
		errs = append(errs, FieldError{Key: "纳税人识别号", Message: "纳税人识别号长度不足（至少 15 位）"})
	}
	return errs
}

// 生成请求与结果

// GenerateRequest 是一次合同生成请求。
type GenerateRequest struct {
	TemplateID string `json:"templateId"`
	Values     Values `json:"values"`
	OutputDir  string `json:"outputDir"` // 为空时使用默认目录
	FileName   string `json:"fileName"`  // 不带扩展名；为空时按规则生成
	// OutputFormat 为空或 docx 时输出 .docx；doc 时通过本机 Word/WPS/LibreOffice 转换为 .doc。
	OutputFormat string `json:"outputFormat"`
	// KeepDocxCopy 为 true 且输出 .doc 时，额外保留一份同名 .docx 备份（推荐：
	// .doc 存在版式差异风险，备份可用于打印/存档/核对）。
	KeepDocxCopy bool `json:"keepDocxCopy"`
}

// GenerateResult 是生成结果。
type GenerateResult struct {
	FilePath     string    `json:"filePath"`
	FileName     string    `json:"fileName"`
	Dir          string    `json:"dir"`
	SHA256       string    `json:"sha256"`
	TemplateName string    `json:"templateName"`
	CreatedAt    time.Time `json:"createdAt"`
	// Format 是实际产出的格式（docx / doc）。当 .doc 生成失败降级时会返回 docx。
	Format string `json:"format"`
	// Warning 非空表示"文件已生成但有需要注意的地方"（例如 .doc 转换失败已降级为 .docx）。
	Warning string `json:"warning"`
	// BackupPath 是同时保留的 .docx 备份路径（生成 .doc 且开启备份时非空）。
	BackupPath string `json:"backupPath"`
	// ConverterName 是实际使用的 .doc 转换通道（如 Microsoft Word / WPS Office / LibreOffice）。
	ConverterName string `json:"converterName"`
}

// Service 提供合同生成能力。
type Service struct {
	store     *store.Store
	supported []string
	converter *converter.Converter
}

// NewService 创建合同服务。conv 用于 .doc 输出（可为 nil，表示不支持 .doc）。
func NewService(st *store.Store, conv *converter.Converter) *Service {
	return &Service{store: st, supported: FieldNames(), converter: conv}
}

// DefaultFileName 生成默认文件名：合同-公司名-日期。
func DefaultFileName(v Values) string {
	company := store.SanitizeFileName(strings.TrimSpace(v["营业执照名称"]))
	if company == "" {
		company = "合同"
	}
	return fmt.Sprintf("合同-%s-%s", company, time.Now().Format("20060102"))
}

// Generate 执行生成：校验 → 渲染 → 原子落盘 → 记录。
// 不覆盖原始模板，不覆盖同名文件。
func (s *Service) Generate(ctx context.Context, req GenerateRequest) (*GenerateResult, error) {
	if errs := req.Values.Validate(); len(errs) > 0 {
		return nil, &ValidationError{Errors: errs}
	}

	tpl, err := s.store.GetTemplate(ctx, req.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("模板不存在或已被删除")
	}
	tplPath := s.store.TemplatePath(tpl)
	if !tpl.FileExists {
		return nil, fmt.Errorf("模板文件已丢失，请重新上传模板“%s”", tpl.Name)
	}

	inspection, err := docx.Inspect(tplPath, s.supported)
	if err != nil {
		return nil, fmt.Errorf("模板无法读取：%w", err)
	}
	if problems := inspection.Problems(); len(problems) > 0 {
		return nil, &TemplateError{Problems: problems}
	}
	// 模板中出现的字段必须全部有值（校验已保证十个字段都有值，这里防御未知字段）
	for _, name := range inspection.UsedFields() {
		if !contains(s.supported, name) {
			return nil, &TemplateError{Problems: []string{fmt.Sprintf("模板中存在不支持的占位符 {{%s}}", name)}}
		}
	}

	outDir := strings.TrimSpace(req.OutputDir)
	if outDir == "" {
		outDir = s.store.ContractsDir()
	}
	if err := ensureWritableDir(outDir); err != nil {
		return nil, err
	}

	format, err := normalizeFormat(req.OutputFormat)
	if err != nil {
		return nil, err
	}
	if format == FormatDoc && (s.converter == nil || !s.converter.Available()) {
		return nil, errors.New("当前电脑没有可用的 Word / WPS / LibreOffice，无法生成 .doc 文件；" +
			"可改用 .docx 格式，或安装其中任意一个后重试（推荐 Microsoft Word，保真度最高）")
	}

	baseName := strings.TrimSpace(req.FileName)
	if baseName == "" {
		baseName = DefaultFileName(req.Values)
	}
	baseName = strings.TrimSuffix(strings.TrimSuffix(store.SanitizeFileName(baseName), ".docx"), ".doc")

	res := &GenerateResult{
		Dir:          outDir,
		TemplateName: tpl.Name,
		CreatedAt:    time.Now(),
		Format:       FormatDocx,
	}

	if format == FormatDoc {
		res.Warning = s.renderDoc(ctx, tplPath, outDir, baseName, req.Values, res, req.KeepDocxCopy)
	} else {
		if err := s.renderDocx(tplPath, outDir, baseName, req.Values, res, tplPath); err != nil {
			return nil, err
		}
	}

	res.SHA256 = fileSHA256(res.FilePath)
	// 记录生成历史（只记录字段名，不保存字段值）
	_ = s.store.AddContract(ctx, &store.Contract{
		ID:           id.New(),
		TemplateID:   tpl.ID,
		TemplateName: tpl.Name,
		FileName:     res.FileName,
		FilePath:     res.FilePath,
		SHA256:       res.SHA256,
		FieldNames:   s.usedFieldNames(inspection),
	})
	return res, nil
}

// renderDocx 渲染 .docx 并原子落盘到 outDir。guardPath 用于防止覆盖原始模板。
func (s *Service) renderDocx(tplPath, outDir, baseName string, values Values, res *GenerateResult, guardPath string) error {
	target := store.UniquePath(outDir, baseName, ".docx")
	if samePath(target, guardPath) {
		return errors.New("输出路径与模板文件相同，请更换保存位置或文件名")
	}
	// 先渲染到同目录的临时文件，再原子重命名，避免产生半成品文件
	tmpPath := target + ".tmp"
	if err := docx.Render(tplPath, tmpPath, values.Map()); err != nil {
		_ = removeFile(tmpPath)
		return fmt.Errorf("生成失败：%w", err)
	}
	if err := renameFile(tmpPath, target); err != nil {
		_ = removeFile(tmpPath)
		return err
	}
	res.FilePath = target
	res.FileName = filepath.Base(target)
	res.Format = FormatDocx
	return nil
}

// renderDoc 生成 .doc：先在临时目录渲染 .docx，再由本机转换器转成 .doc 并自检；
// 转换失败时降级为把 .docx 交付给用户，并在结果里带上 Warning（返回值即 Warning 文案）。
func (s *Service) renderDoc(ctx context.Context, tplPath, outDir, baseName string, values Values, res *GenerateResult, keepDocx bool) string {
	if status := s.converter.Detect(); status.Available {
		res.ConverterName = status.Name
	}
	tmpDir, err := os.MkdirTemp(outDir, ".convert-*")
	if err != nil {
		// 临时目录都建不出来时，退化为直接输出 .docx
		if renderErr := s.renderDocx(tplPath, outDir, baseName, values, res, tplPath); renderErr != nil {
			res.Warning = fmt.Sprintf("生成失败：%v", renderErr)
		}
		return fmt.Sprintf("无法创建临时目录（%v），已改为生成 .docx", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	staged := &GenerateResult{}
	if err := s.renderDocx(tplPath, tmpDir, ".staged", values, staged, ""); err != nil {
		if renderErr := s.renderDocx(tplPath, outDir, baseName, values, res, tplPath); renderErr != nil {
			res.Warning = fmt.Sprintf("生成失败：%v", renderErr)
		}
		return fmt.Sprintf("渲染失败（%v），已改为生成 .docx", err)
	}

	docTarget := store.UniquePath(outDir, baseName, ".doc")
	stagedDoc := filepath.Join(tmpDir, ".staged.doc")
	if err := s.converter.ToDoc(ctx, staged.FilePath, stagedDoc); err != nil {
		if renderErr := s.renderDocx(tplPath, outDir, baseName, values, res, tplPath); renderErr != nil {
			res.Warning = fmt.Sprintf("生成失败：%v", renderErr)
		}
		return fmt.Sprintf("未能转换为 .doc（%v）；已改为生成 .docx，可安装 Microsoft Word / WPS / LibreOffice 后重试", err)
	}
	if err := renameFile(stagedDoc, docTarget); err != nil {
		if renderErr := s.renderDocx(tplPath, outDir, baseName, values, res, tplPath); renderErr != nil {
			res.Warning = fmt.Sprintf("生成失败：%v", renderErr)
		}
		return fmt.Sprintf("保存 .doc 失败（%v），已改为生成 .docx", err)
	}
	// 可选：把中间 .docx 作为备份保留下来（.doc 有版式差异风险时很有用）
	if keepDocx {
		backup := store.UniquePath(outDir, baseName, ".docx")
		if err := os.Rename(staged.FilePath, backup); err == nil {
			res.BackupPath = backup
			staged.FilePath = ""
		}
	}
	res.FilePath = docTarget
	res.FileName = filepath.Base(docTarget)
	res.Format = FormatDoc
	return ""
}

func (s *Service) usedFieldNames(i *docx.Inspection) []string {
	return i.UsedFields()
}

// ValidationError 表示字段校验失败。
type ValidationError struct {
	Errors []FieldError `json:"errors"`
}

func (e *ValidationError) Error() string {
	if len(e.Errors) == 0 {
		return "表单校验失败"
	}
	return e.Errors[0].Message
}

// TemplateError 表示模板本身存在问题。
type TemplateError struct {
	Problems []string `json:"problems"`
}

func (e *TemplateError) Error() string {
	if len(e.Problems) == 0 {
		return "模板存在问题"
	}
	return strings.Join(e.Problems, "；")
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
