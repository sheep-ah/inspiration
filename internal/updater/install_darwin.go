package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// applyPlatform（macOS）：把更新包里的 .app 整包替换当前应用。
//
// 为什么不改包内文件：macOS 上整个 .app 都被签名，改包内内容会使签名失效
// （Apple 官方文档与 TN2206）。整包替换是唯一安全做法。
func applyPlatform(ctx context.Context, opts ApplyOptions) (*ApplyResult, error) {
	if !strings.HasSuffix(strings.ToLower(opts.CurrentAppPath), ".app") {
		return nil, fmt.Errorf("当前应用路径不是 .app：%s", opts.CurrentAppPath)
	}
	opts.Progress("正在解压更新包…")
	extractDir, err := os.MkdirTemp(opts.WorkDir, "inspiration-update-*")
	if err != nil {
		return nil, fmt.Errorf("无法创建临时目录：%w", err)
	}
	defer os.RemoveAll(extractDir)

	bundleName, err := extractAppBundle(opts.DownloadedPath, extractDir)
	if err != nil {
		return nil, err
	}
	extracted := filepath.Join(extractDir, bundleName)

	// 统一成当前的应用名，避免压缩包内文件名编码差异带来的不一致
	canonicalName := filepath.Base(opts.CurrentAppPath)
	canonical := filepath.Join(extractDir, canonicalName)
	if extracted != canonical {
		if err := os.Rename(extracted, canonical); err != nil {
			return nil, fmt.Errorf("无法整理更新包内容：%w", err)
		}
	}
	if err := validateAppBundle(canonical); err != nil {
		return nil, err
	}

	targetDir := filepath.Dir(opts.CurrentAppPath)
	if err := ensureWritable(targetDir); err != nil {
		// 目标目录不可写（例如装在 /Applications 且当前用户无权限）→ 引导手动安装
		manual := opts.DownloadedPath
		_ = exec.Command("open", "-R", manual).Start()
		return &ApplyResult{
			Method:         "manual",
			RequiresManual: true,
			ManualPath:     manual,
			Message: fmt.Sprintf("当前没有权限自动替换 %s（%v）。已为你打开更新包所在位置，请手动把「%s」拖到「应用程序」文件夹覆盖安装。",
				opts.CurrentAppPath, err, canonicalName),
		}, nil
	}

	cleanupStaleBackups(targetDir, canonicalName+".backup-*")

	backup := fmt.Sprintf("%s.backup-%d", opts.CurrentAppPath, time.Now().Unix())
	opts.Progress("正在替换应用…")
	if err := os.Rename(opts.CurrentAppPath, backup); err != nil {
		return nil, fmt.Errorf("无法备份当前版本（更新已中止，原版本未受影响）：%w", err)
	}
	if err := os.Rename(canonical, opts.CurrentAppPath); err != nil {
		// 回滚：把备份放回去
		if rbErr := os.Rename(backup, opts.CurrentAppPath); rbErr != nil {
			return nil, fmt.Errorf("替换失败且回滚失败：%v（备份位于 %s，请手动恢复）", err, backup)
		}
		return nil, fmt.Errorf("替换失败，已回滚到原版本：%w", err)
	}

	// 新版本就位后，去隔离属性（我们自己下载的文件通常没有 quarantine，这里做一次兜底）
	_ = exec.Command("xattr", "-dr", "com.apple.quarantine", opts.CurrentAppPath).Run()

	opts.Progress("正在重启应用…")
	// 由脚本在稍后重启，避免与当前进程退出竞争
	restart := filepath.Join(extractDir, "restart.sh")
	script := fmt.Sprintf("#!/bin/sh\nsleep 1\nopen -n %q\nrm -rf %q\n", opts.CurrentAppPath, extractDir)
	if err := os.WriteFile(restart, []byte(script), 0o700); err == nil {
		cmd := exec.Command("/bin/sh", restart)
		if err := cmd.Start(); err != nil {
			return &ApplyResult{
				Method:       "app-bundle-replace",
				BackupPath:   backup,
				NeedsRestart: false,
				Message:      fmt.Sprintf("已更新到新版本，但自动重启失败（%v）；请手动重新打开应用。", err),
			}, nil
		}
		go func() { _ = cmd.Wait() }()
		return &ApplyResult{
			Method:       "app-bundle-replace",
			BackupPath:   backup,
			NeedsRestart: true,
			Message:      "更新完成，应用将自动重启。旧版本已备份，可在确认新版本正常后删除。",
		}, nil
	}
	return &ApplyResult{
		Method:     "app-bundle-replace",
		BackupPath: backup,
		Message:    "更新完成，请手动重新打开应用。",
	}, nil
}
