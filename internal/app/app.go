// Package app 是 Wails 的绑定层：只暴露窄接口给前端，业务逻辑在 internal 各包中。
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/sheep-ah/inspiration/internal/config"
	"github.com/sheep-ah/inspiration/internal/contract"
	"github.com/sheep-ah/inspiration/internal/converter"
	"github.com/sheep-ah/inspiration/internal/store"
	"github.com/sheep-ah/inspiration/internal/template"
	"github.com/sheep-ah/inspiration/internal/version"
)

// 事件名（前端监听同名事件更新界面状态）。
const (
	EventGenerateStart = "generate:start"
	EventGenerateDone  = "generate:done"
	EventGenerateFail  = "generate:fail"
)

// App 是绑定到前端的应用对象。
type App struct {
	ctx       context.Context
	log       *slog.Logger
	cfg       *config.Config
	store     *store.Store
	templates *template.Service
	contracts *contract.Service
	converter *converter.Converter
}

// New 构造应用对象。
func New(log *slog.Logger, cfg *config.Config, st *store.Store, conv *converter.Converter, tpl *template.Service, con *contract.Service) *App {
	return &App{log: log, cfg: cfg, store: st, templates: tpl, contracts: con, converter: conv}
}

// Startup 由 Wails 在应用启动时调用。
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.log.Info("应用启动", "version", version.String(), "dataDir", a.cfg.Root)
}

// Shutdown 由 Wails 在应用退出时调用。
func (a *App) Shutdown(ctx context.Context) {
	a.log.Info("应用退出")
	if err := a.store.Close(); err != nil {
		a.log.Warn("关闭数据库失败", "err", err)
	}
}

// ---------------------------------------------------------------------------
// 基础信息

// VersionInfo 返回版本信息（界面上必须可见）。
func (a *App) VersionInfo() version.Info { return version.Get() }

// DataDir 返回数据目录（界面“打开数据目录”按钮使用）。
func (a *App) DataDir() string { return a.cfg.Root }

// FieldDefs 返回十个基础字段的定义。
func (a *App) FieldDefs() []contract.FieldDef { return contract.Fields }

// ValidateForm 校验表单（前端在提交前调用，用于逐字段提示）。
func (a *App) ValidateForm(values map[string]string) []contract.FieldError {
	errs := contract.ValuesFromMap(values).Validate()
	if errs == nil {
		return []contract.FieldError{}
	}
	return errs
}

// ---------------------------------------------------------------------------
// 模板管理

// ListTemplates 返回模板列表（query 支持模糊搜索）。
func (a *App) ListTemplates(query string) ([]store.Template, error) {
	return a.templates.List(a.ctx, query)
}

// ImportTemplate 打开文件选择框并导入模板。
func (a *App) ImportTemplate() (*template.Detail, error) {
	path, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: "选择 Word 模板文件（.docx 或 .doc）",
		Filters: []wruntime.FileFilter{
			{DisplayName: "Word 模板 (*.docx;*.doc)", Pattern: "*.docx;*.doc"},
			{DisplayName: "Word 文档 (*.docx)", Pattern: "*.docx"},
			{DisplayName: "旧版 Word 文档 (*.doc)", Pattern: "*.doc"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("无法打开文件选择框：%w", err)
	}
	if strings.TrimSpace(path) == "" {
		return nil, nil // 用户取消
	}
	a.log.Info("导入模板", "file", filepath.Base(path))
	return a.templates.Import(a.ctx, path, "")
}

// TemplateDetail 返回模板详情与占位符信息。
func (a *App) TemplateDetail(id string) (*template.Detail, error) {
	return a.templates.Detail(a.ctx, id)
}

// RenameTemplate 修改模板名称。
func (a *App) RenameTemplate(id, name string) error {
	a.log.Info("重命名模板", "id", id)
	return a.templates.Rename(a.ctx, id, name)
}

// DeleteTemplate 删除模板（不影响已生成的合同）。
func (a *App) DeleteTemplate(id string) error {
	a.log.Info("删除模板", "id", id)
	return a.templates.Delete(a.ctx, id)
}

// SupportedFields 返回支持的字段名列表。
func (a *App) SupportedFields() []string { return a.templates.SupportedFields() }

// ---------------------------------------------------------------------------
// 合同生成

// DefaultOutputDir 返回默认保存目录。
func (a *App) DefaultOutputDir() string { return a.cfg.ContractsDir }

// ConverterStatus 返回 .doc 转换器可用性（界面据此提示能否选择 .doc 输出）。
func (a *App) ConverterStatus() converter.Status {
	if a.converter == nil {
		return converter.Status{Detail: "转换器未初始化"}
	}
	return a.converter.Detect()
}

// PickOutputDir 选择保存目录。
func (a *App) PickOutputDir(current string) (string, error) {
	opts := wruntime.OpenDialogOptions{Title: "选择合同保存位置", DefaultDirectory: current}
	if opts.DefaultDirectory == "" {
		opts.DefaultDirectory = a.cfg.ContractsDir
	}
	dir, err := wruntime.OpenDirectoryDialog(a.ctx, opts)
	if err != nil {
		return "", fmt.Errorf("无法打开目录选择框：%w", err)
	}
	return strings.TrimSpace(dir), nil
}

// GenerateContract 生成合同。
func (a *App) GenerateContract(req contract.GenerateRequest) (*contract.GenerateResult, error) {
	a.emit(EventGenerateStart, map[string]string{"templateId": req.TemplateID})
	res, err := a.contracts.Generate(a.ctx, req)
	if err != nil {
		a.log.Warn("生成合同失败", "err", err.Error())
		a.emit(EventGenerateFail, map[string]string{"message": err.Error()})
		return nil, err
	}
	a.log.Info("生成合同成功", "file", res.FileName, "dir", res.Dir)
	a.emit(EventGenerateDone, map[string]string{"filePath": res.FilePath})
	return res, nil
}

// ListContracts 返回最近的生成记录。
func (a *App) ListContracts(limit int) ([]store.Contract, error) {
	items, err := a.store.ListContracts(a.ctx, limit)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []store.Contract{}
	}
	return items, nil
}

// ---------------------------------------------------------------------------
// 文件操作

// OpenFile 用系统默认程序打开文件或目录。
func (a *App) OpenFile(path string) error {
	if err := checkPath(path); err != nil {
		return err
	}
	return openPath(path)
}

// RevealInFolder 在文件管理器中定位文件。
func (a *App) RevealInFolder(path string) error {
	if err := checkPath(path); err != nil {
		return err
	}
	return revealPath(path)
}

func checkPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("路径为空")
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return errors.New("文件不存在，可能已被移动或删除")
		}
		return fmt.Errorf("无法访问文件：%w", err)
	}
	return nil
}

func (a *App) emit(name string, payload any) {
	if a.ctx == nil {
		return
	}
	wruntime.EventsEmit(a.ctx, name, payload)
}
