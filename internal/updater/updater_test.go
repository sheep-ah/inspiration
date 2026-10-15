package updater

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// fakeGitHub 起一个假的 GitHub API：/repos/{repo}/releases/latest 返回给定 Release。
func fakeGitHub(t *testing.T, rel Release, files map[string][]byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/test/repo/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rel)
	})
	for name, data := range files {
		data := data
		mux.HandleFunc("/dl/"+name, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", fmt.Sprint(len(data)))
			_, _ = w.Write(data)
		})
	}
	return httptest.NewServer(mux)
}

func newTestClient(server *httptest.Server, current, goos, goarch string) *Client {
	c := New(current)
	c.Repo = "test/repo"
	c.BaseURL = server.URL
	c.GOOS = goos
	c.GOARCH = goarch
	c.HTTP = server.Client()
	c.allowLocalHTTP = true // 单测用本机 httptest 服务器
	return c
}

func TestCheckReportsUpToDate(t *testing.T) {
	srv := fakeGitHub(t, Release{TagName: "v1.0.0", Body: "第一版"}, nil)
	defer srv.Close()
	info, err := newTestClient(srv, "1.0.0", "darwin", "arm64").Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !info.UpToDate || info.Message != "已是最新版本" {
		t.Errorf("应判定为已是最新：%+v", info)
	}
}

func TestCheckFindsNewVersionAndSelectsAsset(t *testing.T) {
	srv := fakeGitHub(t, Release{
		TagName: "v1.2.0",
		Body:    "## 更新\n- 修复若干问题",
		Assets: []Asset{
			{Name: "inspiration_1.2.0_darwin_arm64.zip", Size: 100, BrowserDownloadURL: "https://github.com/x/y.zip"},
			{Name: "inspiration_1.2.0_windows_amd64_setup.exe", Size: 200, BrowserDownloadURL: "https://github.com/x/y.exe"},
		},
	}, nil)
	defer srv.Close()
	info, err := newTestClient(srv, "1.1.0", "darwin", "arm64").Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.UpToDate {
		t.Fatal("应发现新版本")
	}
	if info.LatestVersion != "1.2.0" {
		t.Errorf("最新版本 = %s，期望 1.2.0", info.LatestVersion)
	}
	if !strings.Contains(info.Notes, "修复若干问题") {
		t.Errorf("应带上更新说明：%q", info.Notes)
	}
	if info.AssetName != "inspiration_1.2.0_darwin_arm64.zip" {
		t.Errorf("选中资产错误：%s", info.AssetName)
	}
}

func TestCheckRejectsDraftAndPrerelease(t *testing.T) {
	for _, rel := range []Release{
		{TagName: "v1.2.0", Prerelease: true},
		{TagName: "v1.2.0", Draft: true},
	} {
		srv := fakeGitHub(t, rel, nil)
		_, err := newTestClient(srv, "1.0.0", "darwin", "arm64").Check(context.Background())
		srv.Close()
		if err == nil {
			t.Errorf("草稿/预发布版本必须被拒绝：%+v", rel)
		}
	}
}

func TestCheckReportsMissingPlatformAsset(t *testing.T) {
	srv := fakeGitHub(t, Release{
		TagName: "v1.2.0",
		Assets:  []Asset{{Name: "inspiration_1.2.0_windows_amd64_setup.exe", BrowserDownloadURL: "https://github.com/a.exe"}},
	}, nil)
	defer srv.Close()
	info, err := newTestClient(srv, "1.0.0", "darwin", "arm64").Check(context.Background())
	if err != nil {
		t.Fatalf("缺少平台资产不应报错，而是给出提示：%v", err)
	}
	if info.UpToDate {
		t.Error("有新版本时 UpToDate 应为 false")
	}
	if !strings.Contains(info.Message, "未找到适用于") {
		t.Errorf("应明确提示缺少对应平台安装包，实际：%s", info.Message)
	}
}

func TestDownloadReportsProgressAndChecksum(t *testing.T) {
	payload := []byte(strings.Repeat("inspiration-payload-", 5000))
	sum := sha256.Sum256(payload)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	c := newTestClient(srv, "1.0.0", "darwin", "arm64")
	dst := filepath.Join(t.TempDir(), "update.zip")
	var calls int32
	got, err := c.Download(context.Background(), srv.URL+"/file.zip", dst, int64(len(payload)), func(done, total int64) {
		atomic.AddInt32(&calls, 1)
	})
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&calls) == 0 {
		t.Error("应至少回调一次进度")
	}
	if got != hex.EncodeToString(sum[:]) {
		t.Error("返回的 sha256 不正确")
	}
	if err := VerifySHA256(dst, got); err != nil {
		t.Errorf("自校验应通过：%v", err)
	}
}

func TestDownloadRejectsUntrustedHostEvenIfServerReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("malicious"))
	}))
	defer srv.Close()
	c := newTestClient(srv, "1.0.0", "darwin", "arm64")
	c.allowLocalHTTP = false // 生产模式：只允许 HTTPS + GitHub 域名
	err := func() error {
		_, err := c.Download(context.Background(), srv.URL+"/x.zip", filepath.Join(t.TempDir(), "x.zip"), 0, nil)
		return err
	}()
	if err == nil {
		t.Fatal("非 GitHub 来源的下载必须被拒绝")
	}
	if !strings.Contains(err.Error(), "来源不可信") && !strings.Contains(err.Error(), "HTTPS") {
		t.Errorf("错误信息应说明来源问题：%v", err)
	}
}

func TestFetchChecksums(t *testing.T) {
	body := "d0cf11e0a1b11ae1d0cf11e0a1b11ae1d0cf11e0a1b11ae1d0cf11e0a1b11ae1  inspiration_1.0.0_darwin_arm64.zip\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/test/repo/releases/latest" {
			_ = json.NewEncoder(w).Encode(Release{
				TagName: "v1.0.0",
				Assets:  []Asset{{Name: "checksums.txt", BrowserDownloadURL: "http://example.invalid/checksums.txt"}},
			})
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	c := newTestClient(srv, "0.9.0", "darwin", "arm64")
	// 直接构造 Release 来测解析路径（下载地址指向本地测试服务器会被 host 校验拦下，这正是设计意图）
	rel := &Release{TagName: "v1.0.0", Assets: []Asset{{Name: "checksums.txt", BrowserDownloadURL: srv.URL + "/checksums.txt"}}}
	strict := newTestClient(srv, "0.9.0", "darwin", "arm64")
	strict.allowLocalHTTP = false
	if _, err := strict.FetchChecksums(context.Background(), rel); err == nil {
		t.Fatal("非 GitHub 来源的 checksums.txt 必须被拒绝")
	}
	// 正常路径：允许本机 http 时能取到并解析
	sums, err := c.FetchChecksums(context.Background(), rel)
	if err != nil {
		t.Fatalf("本机测试服务器应当可取：%v", err)
	}
	if len(sums) != 1 {
		t.Fatalf("解析结果数量 = %d", len(sums))
	}
}

// ---------------------------------------------------------------------------
// macOS 安装（整包替换 + 备份 + 失败不动原版本）

func makeFakeApp(t *testing.T, dir, name, marker string) string {
	t.Helper()
	app := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(app, "Contents", "Info.plist"), "<plist>"+marker+"</plist>"); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(app, "Contents", "MacOS", strings.TrimSuffix(name, ".app"))
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return app
}

func zipApp(t *testing.T, srcApp, zipPath string) {
	t.Helper()
	if err := zipDir(srcApp, zipPath); err != nil {
		t.Fatal(err)
	}
}

func TestApplyDarwinReplacesBundleAndKeepsBackup(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("仅 macOS 生效")
	}
	work := t.TempDir()
	installDir := filepath.Join(work, "Applications")
	current := makeFakeApp(t, installDir, "灵感.app", "v1")
	// 新版更新包（外部名与当前不同，验证会被统一成当前包名）
	newAppDir := filepath.Join(work, "extract-src")
	newApp := makeFakeApp(t, newAppDir, "Inspiration.app", "v2")
	zipPath := filepath.Join(work, "update.zip")
	zipApp(t, newApp, zipPath)

	res, err := Apply(context.Background(), ApplyOptions{
		DownloadedPath: zipPath,
		CurrentAppPath: current,
		WorkDir:        filepath.Join(work, "tmp"),
	})
	if err != nil {
		t.Fatalf("Apply 失败：%v", err)
	}
	if res.Method != "app-bundle-replace" {
		t.Errorf("方式 = %s，期望 app-bundle-replace", res.Method)
	}
	// 新版已就位
	info, err := os.ReadFile(filepath.Join(current, "Contents", "Info.plist"))
	if err != nil {
		t.Fatalf("新版本未就位：%v", err)
	}
	if !strings.Contains(string(info), "v2") {
		t.Errorf("应用内容未更新：%s", info)
	}
	// Contents/MacOS 下必须存在可执行文件（CFBundleExecutable 指向它，文件名沿用包内原样）
	if err := validateAppBundle(current); err != nil {
		t.Errorf("替换后的包结构不合法：%v", err)
	}
	entries, err := os.ReadDir(filepath.Join(current, "Contents", "MacOS"))
	if err != nil {
		t.Fatal(err)
	}
	executable := false
	for _, e := range entries {
		if fi, err := e.Info(); err == nil && fi.Mode()&0o111 != 0 {
			executable = true
		}
	}
	if !executable {
		t.Error("可执行权限丢失（更新后的应用将无法启动）")
	}
	// 备份保留
	if res.BackupPath == "" || !fileExists(filepath.Join(res.BackupPath, "Contents", "Info.plist")) {
		t.Errorf("旧版本备份未保留：%s", res.BackupPath)
	}
	backupInfo, _ := os.ReadFile(filepath.Join(res.BackupPath, "Contents", "Info.plist"))
	if !strings.Contains(string(backupInfo), "v1") {
		t.Error("备份内容不是旧版本")
	}
	// 新版本自检脚本会清理临时目录，这里不强求
	if res.NeedsRestart && res.Method != "app-bundle-replace" {
		t.Error("NeedsRestart 与方式不匹配")
	}
}

func TestApplyDarwinRejectsInvalidPackageAndKeepsCurrent(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("仅 macOS 生效")
	}
	work := t.TempDir()
	installDir := filepath.Join(work, "Applications")
	current := makeFakeApp(t, installDir, "灵感.app", "v1")

	// 情形 1：不是 zip
	notZip := filepath.Join(work, "broken.zip")
	if err := writeFile(notZip, "not a zip"); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(context.Background(), ApplyOptions{DownloadedPath: notZip, CurrentAppPath: current, WorkDir: work}); err == nil {
		t.Error("坏 zip 必须报错")
	}
	// 情形 2：zip 里没有 .app
	plainDir := filepath.Join(work, "plain")
	if err := os.MkdirAll(plainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(plainDir, "readme.txt"), "hi"); err != nil {
		t.Fatal(err)
	}
	noApp := filepath.Join(work, "noapp.zip")
	zipApp(t, plainDir, noApp)
	if _, err := Apply(context.Background(), ApplyOptions{DownloadedPath: noApp, CurrentAppPath: current, WorkDir: work}); err == nil {
		t.Error("缺少 .app 的包必须报错")
	}
	// 情形 3：结构不完整（缺 MacOS 目录）
	brokenApp := filepath.Join(work, "brokenapp", "灵感.app")
	if err := os.MkdirAll(filepath.Join(brokenApp, "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(brokenApp, "Contents", "Info.plist"), "x"); err != nil {
		t.Fatal(err)
	}
	brokenZip := filepath.Join(work, "brokenapp.zip")
	zipApp(t, brokenApp, brokenZip)
	if _, err := Apply(context.Background(), ApplyOptions{DownloadedPath: brokenZip, CurrentAppPath: current, WorkDir: work}); err == nil {
		t.Error("结构不完整的 .app 必须报错")
	}

	// 三次失败后，当前版本必须完好无损
	info, err := os.ReadFile(filepath.Join(current, "Contents", "Info.plist"))
	if err != nil {
		t.Fatalf("当前版本被破坏：%v", err)
	}
	if !strings.Contains(string(info), "v1") {
		t.Error("当前版本内容被改动")
	}
	// 不应产生备份（说明没有进入替换流程）
	matches, _ := filepath.Glob(current + ".backup-*")
	if len(matches) != 0 {
		t.Errorf("失败路径不应产生备份：%v", matches)
	}
}

// zipDir 把 srcDir 整个目录（含顶层目录名）打包成 zip，模拟发布包结构。
func zipDir(srcDir, zipPath string) error {
	f, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()
	parent := filepath.Dir(srcDir)
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(parent, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		if info.IsDir() {
			_, err := zw.Create(name + "/")
			return err
		}
		info2, err := os.Stat(path)
		if err != nil {
			return err
		}
		hdr, err := zip.FileInfoHeader(info2)
		if err != nil {
			return err
		}
		hdr.Name = name
		hdr.Method = zip.Deflate
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
		return nil
	})
}
