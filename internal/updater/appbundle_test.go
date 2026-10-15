package updater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// zipDirToFile 是 zipDir 的别名（便于测试中表达意图：把目录打成 zip 文件）。
func zipDirToFile(t *testing.T, srcDir, zipPath string) {
	t.Helper()
	if err := zipDir(srcDir, zipPath); err != nil {
		t.Fatal(err)
	}
}

// 这些用例覆盖与平台无关的 .app 包处理逻辑，因此在 Linux/Windows 的 CI 上同样会执行，
// 避免"只在 macOS 能编译"的测试代码（这正是第一次 CI 暴露的问题）。

func TestExtractAppBundleFromZip(t *testing.T) {
	work := t.TempDir()
	src := makeFakeApp(t, filepath.Join(work, "src"), "灵感.app", "v9")
	zipPath := filepath.Join(work, "app.zip")
	zipDirToFile(t, src, zipPath)

	dst := filepath.Join(work, "out")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	name, err := extractAppBundle(zipPath, dst)
	if err != nil {
		t.Fatalf("解压失败：%v", err)
	}
	if name != "灵感.app" {
		t.Errorf("顶层 .app 名称 = %q，期望 灵感.app", name)
	}
	extracted := filepath.Join(dst, name)
	if err := validateAppBundle(extracted); err != nil {
		t.Errorf("解压结果应通过结构校验：%v", err)
	}
	// 可执行权限必须保留（否则替换后应用无法启动）
	entries, err := os.ReadDir(filepath.Join(extracted, "Contents", "MacOS"))
	if err != nil {
		t.Fatal(err)
	}
	ok := false
	for _, e := range entries {
		if fi, err := e.Info(); err == nil && fi.Mode()&0o111 != 0 {
			ok = true
		}
	}
	if !ok {
		t.Error("解压后缺少可执行权限")
	}
}

func TestExtractAppBundleRejectsPlainZip(t *testing.T) {
	work := t.TempDir()
	plain := filepath.Join(work, "plain")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(plain, "a.txt"), "x"); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(work, "plain.zip")
	zipDirToFile(t, plain, zipPath)

	dst := filepath.Join(work, "out")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := extractAppBundle(zipPath, dst); err == nil {
		t.Fatal("不含 .app 的 zip 必须报错")
	}
}

func TestValidateAppBundleDetectsBrokenStructure(t *testing.T) {
	work := t.TempDir()
	// 缺 Info.plist
	noPlist := filepath.Join(work, "noPlist", "灵感.app")
	if err := os.MkdirAll(filepath.Join(noPlist, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := validateAppBundle(noPlist); err == nil {
		t.Error("缺少 Info.plist 应被判定为不完整")
	}
	// 缺可执行文件
	noExe := filepath.Join(work, "noExe", "灵感.app")
	if err := os.MkdirAll(filepath.Join(noExe, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(noExe, "Contents", "Info.plist"), "x"); err != nil {
		t.Fatal(err)
	}
	if err := validateAppBundle(noExe); err == nil {
		t.Error("缺少可执行文件应被判定为不完整")
	} else if !strings.Contains(err.Error(), "可执行") {
		t.Errorf("错误信息应说明缺少可执行文件：%v", err)
	}
	// 合法结构
	good := makeFakeApp(t, work, "Good.app", "v1")
	if err := validateAppBundle(good); err != nil {
		t.Errorf("合法结构不应报错：%v", err)
	}
}

func TestCleanupStaleBackups(t *testing.T) {
	dir := t.TempDir()
	fresh := filepath.Join(dir, "灵感.app.backup-2000000000")
	stale := filepath.Join(dir, "灵感.app.backup-1000000000")
	for _, p := range []string{fresh, stale} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// 把 stale 的修改时间设成 10 天前
	old := time.Now().Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	cleanupStaleBackups(dir, "灵感.app.backup-*")
	if _, err := os.Stat(stale); err == nil {
		t.Error("超过 7 天的备份应被清理")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Error("新的备份不应被清理")
	}
}
