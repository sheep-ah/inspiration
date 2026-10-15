package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ApplyOptions 描述一次更新安装。
type ApplyOptions struct {
	// DownloadedPath 是已经通过 sha256 校验的安装包/更新包。
	DownloadedPath string
	// CurrentAppPath 是当前运行的应用路径：macOS 为 xxx.app，Windows 为 xxx.exe。
	CurrentAppPath string
	// WorkDir 是解压/脚本的工作目录（默认系统临时目录）。
	WorkDir string
	// Progress 用于向界面报告当前步骤（可选）。
	Progress func(step string)
}

// ApplyResult 是安装结果。
type ApplyResult struct {
	// Method 说明采用的方式：app-bundle-replace / nsis-silent / portable-swap / manual
	Method string `json:"method"`
	// Message 给用户看的说明。
	Message string `json:"message"`
	// NeedsRestart 表示安装脚本会在退出后继续执行并重启应用。
	NeedsRestart bool `json:"needsRestart"`
	// RequiresManual 表示需要用户手动完成（例如无写权限时的 dmg 安装）。
	RequiresManual bool `json:"requiresManual"`
	// ManualPath 需要用户手动处理的文件路径。
	ManualPath string `json:"manualPath"`
	// BackupPath 旧版本备份路径（保留以便回滚）。
	BackupPath string `json:"backupPath"`
}

// Apply 按平台执行更新安装。调用方应在返回 NeedsRestart 后退出应用。
func Apply(ctx context.Context, opts ApplyOptions) (*ApplyResult, error) {
	if strings.TrimSpace(opts.DownloadedPath) == "" {
		return nil, errors.New("缺少已下载的更新包")
	}
	if _, err := os.Stat(opts.DownloadedPath); err != nil {
		return nil, fmt.Errorf("更新包不存在：%w", err)
	}
	if opts.WorkDir == "" {
		opts.WorkDir = os.TempDir()
	}
	if err := os.MkdirAll(opts.WorkDir, 0o755); err != nil {
		return nil, fmt.Errorf("无法创建更新工作目录：%w", err)
	}
	if opts.Progress == nil {
		opts.Progress = func(string) {}
	}
	return applyPlatform(ctx, opts)
}

// cleanupStaleBackups 清理历史备份（保留 7 天），避免长期占用磁盘。
func cleanupStaleBackups(dir, pattern string) {
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.RemoveAll(m)
	}
}
