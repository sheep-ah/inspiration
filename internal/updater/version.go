package updater

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// CompareVersions 比较两个版本号，返回 -1 / 0 / 1。
//
// 规则（够用即可，不追求完整 semver）：
//   - 允许 v 前缀；主版本段按数字比较；
//   - 形如 1.2.3-rc.1 的预发布版本小于同号的 1.2.3；
//   - 预发布后缀按段比较（数字段按数值，字符串段按字典序）。
func CompareVersions(a, b string) int {
	am, ap := splitVersion(a)
	bm, bp := splitVersion(b)
	for i := 0; i < 3; i++ {
		if am[i] != bm[i] {
			if am[i] < bm[i] {
				return -1
			}
			return 1
		}
	}
	// 主版本相同：有预发布后缀的更小
	switch {
	case ap == "" && bp == "":
		return 0
	case ap == "":
		return 1
	case bp == "":
		return -1
	}
	return comparePrerelease(ap, bp)
}

// splitVersion 拆出 [major, minor, patch] 与预发布后缀。
func splitVersion(v string) ([3]int, string) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	pre := ""
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		pre = v[i+1:]
		v = v[:i]
	}
	var out [3]int
	for i, part := range strings.SplitN(v, ".", 3) {
		if i > 2 {
			break
		}
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			// 非法段按 0 处理，避免因为脏数据把更新判成"有新版"
			n = 0
		}
		out[i] = n
	}
	return out, pre
}

func comparePrerelease(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aerr := strconv.Atoi(as[i])
		bn, berr := strconv.Atoi(bs[i])
		switch {
		case aerr == nil && berr == nil:
			if an != bn {
				if an < bn {
					return -1
				}
				return 1
			}
		case aerr == nil:
			return -1 // 数字段 < 字符串段
		case berr == nil:
			return 1
		default:
			if as[i] != bs[i] {
				if as[i] < bs[i] {
					return -1
				}
				return 1
			}
		}
	}
	switch {
	case len(as) == len(bs):
		return 0
	case len(as) < len(bs):
		return -1
	default:
		return 1
	}
}

// IsNewer 判断 latest 是否比 current 新。
func IsNewer(current, latest string) bool {
	if latest == "" {
		return false
	}
	return CompareVersions(current, latest) < 0
}

// NormalizeTag 去掉 v 前缀，用于展示与比较。
func NormalizeTag(tag string) string {
	return strings.TrimPrefix(strings.TrimSpace(tag), "v")
}

// FormatVersion 统一展示格式（带 v 前缀）。
func FormatVersion(tag string) string {
	return "v" + NormalizeTag(tag)
}

// strictVersionRe 只接受 vX.Y.Z / X.Y.Z（与发布工作流的标签规则一致）。
var strictVersionRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// validateTag 校验标签格式，防止把异常字符串（如 1.2.3-rc1、v1.2.3.4）当成版本号。
func validateTag(tag string) error {
	if !strictVersionRe.MatchString(NormalizeTag(tag)) {
		return fmt.Errorf("版本号格式不合法：%s（要求 vX.Y.Z）", tag)
	}
	return nil
}
