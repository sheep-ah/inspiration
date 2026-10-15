package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// MaxTemplateBytes 单个模板文件大小上限（50 MB）。
const MaxTemplateBytes = 50 << 20

// copyFileAtomic 把 srcPath 复制到 dir/name（先写临时文件再 rename），返回实际路径、大小与 sha256。
func copyFileAtomic(srcPath, dir, name string, maxBytes int64) (string, int64, string, error) {
	in, err := os.Open(srcPath)
	if err != nil {
		return "", 0, "", fmt.Errorf("无法读取文件: %w", err)
	}
	defer in.Close()

	if info, err := in.Stat(); err == nil {
		if info.IsDir() {
			return "", 0, "", errors.New("请选择 DOCX 文件而不是文件夹")
		}
		if info.Size() > maxBytes {
			return "", 0, "", fmt.Errorf("文件过大（%d MB），模板上限为 %d MB", info.Size()>>20, maxBytes>>20)
		}
	}

	tmp, err := os.CreateTemp(dir, ".tmp-*.docx")
	if err != nil {
		return "", 0, "", fmt.Errorf("无法在数据目录创建临时文件: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	hasher := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hasher), io.LimitReader(in, maxBytes+1))
	if err != nil {
		cleanup()
		return "", 0, "", fmt.Errorf("复制文件失败: %w", err)
	}
	if size > maxBytes {
		cleanup()
		return "", 0, "", fmt.Errorf("文件过大（超过 %d MB）", maxBytes>>20)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return "", 0, "", fmt.Errorf("写入磁盘失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, "", fmt.Errorf("关闭文件失败: %w", err)
	}

	finalPath := filepath.Join(dir, name)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", 0, "", fmt.Errorf("保存文件失败: %w", err)
	}
	return finalPath, size, hex.EncodeToString(hasher.Sum(nil)), nil
}

func removeQuiet(path string) error {
	if path == "" {
		return nil
	}
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// normalizeQuery 归一化搜索关键字（去空格、转小写）。
func normalizeQuery(q string) string {
	return strings.ToLower(strings.TrimSpace(q))
}

// fuzzyMatch 依次匹配 query 中的每个字符（顺序敏感、允许中间跳字），适合模板名模糊搜索。
func fuzzyMatch(name, query string) bool {
	name = strings.ToLower(name)
	if query == "" {
		return true
	}
	if strings.Contains(name, query) {
		return true
	}
	i := 0
	for _, r := range query {
		idx := strings.IndexRune(name[i:], r)
		if idx < 0 {
			return false
		}
		i += idx + len(string(r))
	}
	return true
}

// SanitizeFileName 清洗用户提供的文件名：去掉路径分隔符、控制字符与 Windows 保留名。
func SanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' ||
			r == '<' || r == '>' || r == '|' || r == 0:
			return '-'
		case unicode.IsControl(r):
			return -1
		default:
			return r
		}
	}, name)
	name = strings.Trim(name, " .")
	if name == "" {
		return "合同"
	}
	if len([]rune(name)) > 120 {
		name = string([]rune(name)[:120])
	}
	base := strings.ToUpper(strings.TrimSuffix(name, filepath.Ext(name)))
	switch base {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		name = "_" + name
	}
	return name
}

// UniquePath 返回不存在的目标路径：重名时追加 (2)、(3)…，绝不静默覆盖。
func UniquePath(dir, baseName, ext string) string {
	candidate := filepath.Join(dir, baseName+ext)
	if !fileExists(candidate) {
		return candidate
	}
	for i := 2; i < 1000; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", baseName, i, ext))
		if !fileExists(candidate) {
			return candidate
		}
	}
	// 极端情况：加时间戳保证唯一
	return filepath.Join(dir, fmt.Sprintf("%s-%d%s", baseName, time.Now().UnixNano(), ext))
}
