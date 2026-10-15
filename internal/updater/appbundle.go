package updater

// 本文件里的 .app 包处理逻辑与平台无关（只做 zip 解包与目录校验），
// 与 macOS 相关的只有"替换整包"这一步（见 install_darwin.go）。
// 放在无构建约束的文件里，便于在 Linux/Windows 上也能编译并测试这部分逻辑。

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

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
