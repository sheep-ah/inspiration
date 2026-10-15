// Package config 负责应用数据目录的解析与初始化。
//
// 数据目录使用系统用户级目录，绝不写入程序安装目录或临时目录：
//
//	macOS   ~/Library/Application Support/灵感/
//	Windows %APPDATA%\灵感\
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// AppDirName 是应用在用户配置目录下的子目录名。
const AppDirName = "灵感"

// Config 描述应用运行时需要的目录。
type Config struct {
	// Root 数据根目录。
	Root string
	// TemplatesDir 模板 DOCX 存放目录（文件名由应用生成）。
	TemplatesDir string
	// ContractsDir 合同历史索引默认存放目录（用户可选择其它输出目录）。
	ContractsDir string
	// LogsDir 日志目录。
	LogsDir string
	// UpdateDir 更新下载临时目录。
	UpdateDir string
	// DBPath SQLite 数据库路径。
	DBPath string
}

// Default 返回默认配置并确保目录存在。
func Default() (*Config, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("无法定位用户配置目录: %w", err)
	}
	return At(filepath.Join(base, AppDirName))
}

// At 在指定根目录下构造配置并创建目录。
func At(root string) (*Config, error) {
	cfg := &Config{
		Root:         root,
		TemplatesDir: filepath.Join(root, "templates"),
		ContractsDir: filepath.Join(root, "contracts"),
		LogsDir:      filepath.Join(root, "logs"),
		UpdateDir:    filepath.Join(root, "update"),
		DBPath:       filepath.Join(root, "inspiration.db"),
	}
	if err := cfg.Ensure(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Ensure 创建全部目录。
func (c *Config) Ensure() error {
	for _, dir := range []string{c.Root, c.TemplatesDir, c.ContractsDir, c.LogsDir, c.UpdateDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建目录失败 %s: %w", dir, err)
		}
	}
	return nil
}
