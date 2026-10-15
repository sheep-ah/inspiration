package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ensureWritableDir 确认目录存在且可写（不存在时尝试创建）。
func ensureWritableDir(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return fmt.Errorf("保存位置不能为空")
	}
	info, err := os.Stat(dir)
	switch {
	case err == nil:
		if !info.IsDir() {
			return fmt.Errorf("保存位置不是一个文件夹：%s", dir)
		}
	case os.IsNotExist(err):
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("无法创建保存位置 %s：%w", dir, err)
		}
	default:
		return fmt.Errorf("无法访问保存位置 %s：%w", dir, err)
	}

	// 试写一个临时文件，尽早暴露权限/磁盘问题
	probe, err := os.CreateTemp(dir, ".write-probe-*")
	if err != nil {
		return fmt.Errorf("保存位置不可写（%s）：%w", dir, err)
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return nil
}

// samePath 判断两个路径是否指向同一个文件。
func samePath(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return a == b
	}
	if absA == absB {
		return true
	}
	infoA, errA := os.Stat(absA)
	infoB, errB := os.Stat(absB)
	if errA != nil || errB != nil {
		return false
	}
	return os.SameFile(infoA, infoB)
}

func removeFile(path string) error {
	err := os.Remove(path)
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return err
}

func renameFile(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return fmt.Errorf("保存生成结果失败：%w", err)
	}
	return nil
}

// fileSHA256 计算文件摘要（用于生成记录与后续完整性校验）。
func fileSHA256(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Map 把字段值转换为占位符替换需要的 map。
func (v Values) Map() map[string]string {
	out := make(map[string]string, len(Fields))
	for _, f := range Fields {
		out[f.Key] = strings.TrimSpace(v[f.Key])
	}
	return out
}
