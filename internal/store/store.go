// Package store 负责 SQLite 元数据与模板文件的持久化。
//
// 设计约束（来自任务书）：
//   - 数据文件放在用户级应用数据目录，不在安装目录或临时目录；
//   - 应用重启后数据仍存在，升级不清空数据；
//   - 模板文件与元数据关联稳定，导入失败要有清理/回滚；
//   - 删除模板不影响已生成的合同。
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/sheep-ah/inspiration/internal/config"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动，交叉编译无需 cgo
)

// ErrNotFound 表示记录不存在。
var ErrNotFound = errors.New("记录不存在")

// 模板来源格式。
const (
	// SourceFormatDocx 表示直接上传的 .docx 模板。
	SourceFormatDocx = "docx"
	// SourceFormatDoc 表示由 .doc 自动转换而来的模板（原始 .doc 会保留备查）。
	SourceFormatDoc = "doc"
)

const schemaVersion = 2

// Store 是数据访问入口。
type Store struct {
	db  *sql.DB
	cfg *config.Config
}

// Open 打开（必要时创建）数据库并执行迁移。
func Open(cfg *config.Config) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)", cfg.DBPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	// SQLite 写入串行化，单连接可避免写锁竞争与 "database is locked"
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("数据库不可用: %w", err)
	}
	s := &Store{db: db, cfg: cfg}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

// Root 返回数据根目录。
func (s *Store) Root() string { return s.cfg.Root }

// TemplatesDir 返回模板目录。
func (s *Store) TemplatesDir() string { return s.cfg.TemplatesDir }

// ContractsDir 返回默认合同输出目录。
func (s *Store) ContractsDir() string { return s.cfg.ContractsDir }

func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS templates (
  id           TEXT PRIMARY KEY,
  name         TEXT NOT NULL,
  stored_file  TEXT NOT NULL,
  size_bytes   INTEGER NOT NULL,
  sha256       TEXT NOT NULL,
  placeholders TEXT NOT NULL DEFAULT '[]',
  field_usage  TEXT NOT NULL DEFAULT '{}',
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL,
  deleted_at   INTEGER,
  -- source_format: 模板来源（docx=直接上传；doc=由 .doc 自动转换而来）
  source_format TEXT NOT NULL DEFAULT 'docx',
  -- original_file: 由 .doc 转换时保留的原始文件名（位于 templates/ 目录）
  original_file TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_templates_name ON templates(name);

CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS contracts (
  id            TEXT PRIMARY KEY,
  template_id   TEXT,
  template_name TEXT NOT NULL,
  file_name     TEXT NOT NULL,
  file_path     TEXT NOT NULL,
  sha256        TEXT NOT NULL,
  field_names   TEXT NOT NULL DEFAULT '[]',
  created_at    INTEGER NOT NULL
);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("初始化数据库结构失败: %w", err)
	}
	var current int
	err := s.db.QueryRow(`SELECT CAST(value AS INTEGER) FROM meta WHERE key='schema_version'`).Scan(&current)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := s.db.Exec(`INSERT INTO meta(key,value) VALUES('schema_version',?)`, schemaVersion); err != nil {
			return fmt.Errorf("写入 schema_version 失败: %w", err)
		}
	case err != nil:
		return fmt.Errorf("读取 schema_version 失败: %w", err)
	case current > schemaVersion:
		return fmt.Errorf("数据库版本(%d)高于当前应用支持的版本(%d)，请升级应用", current, schemaVersion)
	}
	return s.ensureColumns()
}

// ensureColumns 为旧版本数据库补齐新增列（老用户升级时不丢数据）。
func (s *Store) ensureColumns() error {
	columns := []struct{ table, name, ddl string }{
		{"templates", "source_format", `ALTER TABLE templates ADD COLUMN source_format TEXT NOT NULL DEFAULT 'docx'`},
		{"templates", "original_file", `ALTER TABLE templates ADD COLUMN original_file TEXT NOT NULL DEFAULT ''`},
	}
	for _, c := range columns {
		exists, err := s.columnExists(c.table, c.name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := s.db.Exec(c.ddl); err != nil {
			return fmt.Errorf("升级数据库结构失败(%s.%s): %w", c.table, c.name, err)
		}
	}
	return nil
}

func (s *Store) columnExists(table, column string) (bool, error) {
	rows, err := s.db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false, fmt.Errorf("读取表结构失败: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		if strings.EqualFold(name, column) {
			return true, nil
		}
	}
	return false, rows.Err()
}

// ---------------------------------------------------------------------------
// 模板

// PlaceholderInfo 是占位符在模板中的分布。
type PlaceholderInfo struct {
	Name  string   `json:"name"`
	Count int      `json:"count"`
	Parts []string `json:"parts"`
}

// Template 是模板元数据。
type Template struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	StoredFile   string            `json:"-"`
	SizeBytes    int64             `json:"sizeBytes"`
	SHA256       string            `json:"sha256"`
	Placeholders []PlaceholderInfo `json:"placeholders"`
	FieldUsage   map[string]int    `json:"fieldUsage"`
	CreatedAt    time.Time         `json:"createdAt"`
	UpdatedAt    time.Time         `json:"updatedAt"`
	// FileExists 表示磁盘上的文件是否仍然存在（界面上标记缺失/损坏）。
	FileExists bool `json:"fileExists"`
	// SourceFormat 模板来源：docx（直接上传）或 doc（由 .doc 自动转换而来）。
	SourceFormat string `json:"sourceFormat"`
	// OriginalFile 由 .doc 转换时保留的原始文件名（空表示无）。
	OriginalFile string `json:"originalFile"`
}

// TemplatePath 返回模板文件在磁盘上的绝对路径。
func (s *Store) TemplatePath(t *Template) string {
	return filepath.Join(s.cfg.TemplatesDir, t.StoredFile)
}

// PutTemplateFile 把模板文件原子复制到数据目录，返回大小与 sha256。
// 只负责文件落盘，不写数据库；调用方在扫描校验通过后再调用 InsertTemplate。
func (s *Store) PutTemplateFile(srcPath, storedFile string) (int64, string, error) {
	_, size, sum, err := copyFileAtomic(srcPath, s.cfg.TemplatesDir, storedFile, MaxTemplateBytes)
	if err != nil {
		return 0, "", err
	}
	return size, sum, nil
}

// PutOriginalFile 保存由 .doc 转换而来的原始文件（备查，不参与生成）。
func (s *Store) PutOriginalFile(srcPath, storedFile string) error {
	_, _, _, err := copyFileAtomic(srcPath, s.cfg.TemplatesDir, storedFile, MaxTemplateBytes)
	return err
}

// OriginalFilePath 返回保留的原始 .doc 路径（空字符串表示没有）。
func (s *Store) OriginalFilePath(t *Template) string {
	if t.OriginalFile == "" {
		return ""
	}
	return filepath.Join(s.cfg.TemplatesDir, t.OriginalFile)
}

// InsertTemplate 写入模板元数据。
func (s *Store) InsertTemplate(ctx context.Context, t *Template) error {
	ph, _ := json.Marshal(t.Placeholders)
	usage, _ := json.Marshal(t.FieldUsage)
	now := time.Now()
	t.CreatedAt, t.UpdatedAt = now, now
	if t.SourceFormat == "" {
		t.SourceFormat = "docx"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO templates(id,name,stored_file,size_bytes,sha256,placeholders,field_usage,created_at,updated_at,source_format,original_file)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.Name, t.StoredFile, t.SizeBytes, t.SHA256, string(ph), string(usage),
		now.Unix(), now.Unix(), t.SourceFormat, t.OriginalFile)
	if err != nil {
		return fmt.Errorf("保存模板信息失败: %w", err)
	}
	return nil
}

// ListTemplates 返回未删除的模板；query 非空时按名称做模糊匹配。
func (s *Store) ListTemplates(ctx context.Context, query string) ([]Template, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,name,stored_file,size_bytes,sha256,placeholders,field_usage,created_at,updated_at,source_format,original_file
		 FROM templates WHERE deleted_at IS NULL ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("查询模板失败: %w", err)
	}
	defer rows.Close()

	q := normalizeQuery(query)
	var out []Template
	for rows.Next() {
		var (
			t                  Template
			ph, usage          string
			createdAt, updated int64
		)
		if err := rows.Scan(&t.ID, &t.Name, &t.StoredFile, &t.SizeBytes, &t.SHA256, &ph, &usage, &createdAt, &updated, &t.SourceFormat, &t.OriginalFile); err != nil {
			return nil, err
		}
		t.Placeholders = decodePlaceholders(ph)
		t.FieldUsage = decodeUsage(usage)
		t.CreatedAt = time.Unix(createdAt, 0)
		t.UpdatedAt = time.Unix(updated, 0)
		t.FileExists = fileExists(s.TemplatePath(&t))
		if q != "" && !fuzzyMatch(t.Name, q) {
			continue
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetTemplate 按 id 读取模板。
func (s *Store) GetTemplate(ctx context.Context, id string) (*Template, error) {
	var (
		t                  Template
		ph, usage          string
		createdAt, updated int64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id,name,stored_file,size_bytes,sha256,placeholders,field_usage,created_at,updated_at,source_format,original_file
		 FROM templates WHERE id=? AND deleted_at IS NULL`, id).
		Scan(&t.ID, &t.Name, &t.StoredFile, &t.SizeBytes, &t.SHA256, &ph, &usage, &createdAt, &updated, &t.SourceFormat, &t.OriginalFile)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.Placeholders = decodePlaceholders(ph)
	t.FieldUsage = decodeUsage(usage)
	t.CreatedAt = time.Unix(createdAt, 0)
	t.UpdatedAt = time.Unix(updated, 0)
	t.FileExists = fileExists(s.TemplatePath(&t))
	return &t, nil
}

// RenameTemplate 修改模板名称。
func (s *Store) RenameTemplate(ctx context.Context, id, name string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE templates SET name=?, updated_at=? WHERE id=? AND deleted_at IS NULL`,
		name, time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("重命名失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteTemplate 软删除模板并删除磁盘文件；已生成的合同记录不受影响。
func (s *Store) DeleteTemplate(ctx context.Context, id string) error {
	t, err := s.GetTemplate(ctx, id)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE templates SET deleted_at=? WHERE id=? AND deleted_at IS NULL`, time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("删除模板失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	// 文件删除失败不阻断流程：数据库已标记删除，界面不再显示该模板
	_ = removeQuiet(s.TemplatePath(t))
	if t.OriginalFile != "" {
		_ = removeQuiet(filepath.Join(s.cfg.TemplatesDir, t.OriginalFile))
	}
	return nil
}

// UpdateTemplateInspection 在模板文件仍然可读时刷新占位符信息。
func (s *Store) UpdateTemplateInspection(ctx context.Context, id string, placeholders []PlaceholderInfo, usage map[string]int) error {
	ph, _ := json.Marshal(placeholders)
	u, _ := json.Marshal(usage)
	_, err := s.db.ExecContext(ctx,
		`UPDATE templates SET placeholders=?, field_usage=?, updated_at=? WHERE id=? AND deleted_at IS NULL`,
		string(ph), string(u), time.Now().Unix(), id)
	return err
}

// ---------------------------------------------------------------------------
// 合同

// Contract 是生成记录（只保存字段名，不保存字段值）。
type Contract struct {
	ID           string    `json:"id"`
	TemplateID   string    `json:"templateId"`
	TemplateName string    `json:"templateName"`
	FileName     string    `json:"fileName"`
	FilePath     string    `json:"filePath"`
	SHA256       string    `json:"sha256"`
	FieldNames   []string  `json:"fieldNames"`
	CreatedAt    time.Time `json:"createdAt"`
}

// AddContract 记录一次成功的合同生成。
func (s *Store) AddContract(ctx context.Context, c *Contract) error {
	fn, _ := json.Marshal(c.FieldNames)
	now := time.Now()
	c.CreatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO contracts(id,template_id,template_name,file_name,file_path,sha256,field_names,created_at)
		 VALUES(?,?,?,?,?,?,?,?)`,
		c.ID, nullIfEmpty(c.TemplateID), c.TemplateName, c.FileName, c.FilePath, c.SHA256, string(fn), now.Unix())
	if err != nil {
		return fmt.Errorf("保存生成记录失败: %w", err)
	}
	return nil
}

// ListContracts 返回最近的生成记录。
func (s *Store) ListContracts(ctx context.Context, limit int) ([]Contract, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,COALESCE(template_id,''),template_name,file_name,file_path,sha256,field_names,created_at
		 FROM contracts ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("查询生成记录失败: %w", err)
	}
	defer rows.Close()
	var out []Contract
	for rows.Next() {
		var (
			c         Contract
			fn        string
			createdAt int64
		)
		if err := rows.Scan(&c.ID, &c.TemplateID, &c.TemplateName, &c.FileName, &c.FilePath, &c.SHA256, &fn, &createdAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(fn), &c.FieldNames)
		c.CreatedAt = time.Unix(createdAt, 0)
		out = append(out, c)
	}
	return out, rows.Err()
}

// Setting 读取设置项。
func (s *Store) Setting(ctx context.Context, key string) (string, bool) {
	var v string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, key).Scan(&v); err != nil {
		return "", false
	}
	return v, true
}

// SetSetting 写入设置项。
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func decodePlaceholders(raw string) []PlaceholderInfo {
	var out []PlaceholderInfo
	_ = json.Unmarshal([]byte(raw), &out)
	if out == nil {
		out = []PlaceholderInfo{}
	}
	return out
}

func decodeUsage(raw string) map[string]int {
	out := map[string]int{}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}
