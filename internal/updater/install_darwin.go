package updater

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
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

// extractAppBundle 解压更新包并返回 .app 的顶层目录名。
func extractAppBundle(zipPath, dstDir string) (string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("更新包不是有效的 zip（%v），请重新下载", err)
	}
	defer zr.Close()

	var bundle string
	for _, f := range zr.File {
		if f.Mode()&os.ModeSymlink != 0 {
			continue // 我们的包内没有符号链接，跳过以避免越界写入
		}
		name := strings.TrimPrefix(filepath.ToSlash(f.Name), "./")
		if name == "" {
			continue
		}
		top := strings.SplitN(name, "/", 2)[0]
		if strings.HasSuffix(strings.ToLower(top), ".app") {
			bundle = top
			break
		}
	}
	if bundle == "" {
		return "", errors.New("更新包里没有找到 .app，可能是下载了错误的平台安装包")
	}

	base := filepath.Join(dstDir, filepath.FromSlash(bundle))
	for _, f := range zr.File {
		name := strings.TrimPrefix(filepath.ToSlash(f.Name), "./")
		if name == "" || strings.Contains(name, "..") {
			continue
		}
		target := filepath.Join(dstDir, filepath.FromSlash(name))
		if !strings.HasPrefix(target, dstDir+string(os.PathSeparator)) {
			continue // 防目录穿越
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, f.Mode().Perm()|0o700); err != nil {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", err
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode().Perm())
		if err != nil {
			rc.Close()
			return "", err
		}
		if _, err := io.Copy(out, rc); err != nil {
			rc.Close()
			out.Close()
			return "", err
		}
		rc.Close()
		if err := out.Close(); err != nil {
			return "", err
		}
	}
	return filepath.Base(base), nil
}

// validateAppBundle 粗校验：必须是合法的 .app 结构，且可执行文件存在。
func validateAppBundle(appPath string) error {
	info := filepath.Join(appPath, "Contents", "Info.plist")
	if _, err := os.Stat(info); err != nil {
		return fmt.Errorf("更新包结构不完整（缺少 Info.plist）：%w", err)
	}
	macOSDir := filepath.Join(appPath, "Contents", "MacOS")
	entries, err := os.ReadDir(macOSDir)
	if err != nil {
		return fmt.Errorf("更新包结构不完整（缺少可执行目录）：%w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fi, err := e.Info()
		if err == nil && fi.Mode()&0o111 != 0 {
			return nil
		}
	}
	return errors.New("更新包里没有可执行文件，拒绝安装")
}
