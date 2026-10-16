package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// 更新检查优先读发布资产 latest.json，**完全不碰 API**。
// 这条守的是"403 Forbidden"那次真实故障：未登录调用 GitHub API 每小时只有 60 次、按 IP 计，
// 同一出口 IP 的其它工具用光配额后，用户的「检查更新」就整个失败了。
func TestCheckUsesReleaseManifestWithoutAPI(t *testing.T) {
	payload := []byte("inspiration-payload-for-manifest-test")
	sum := sha256.Sum256(payload)
	var apiHits int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest/download/latest.json"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"version":     "0.2.0",
				"tag":         "v0.2.0",
				"releaseUrl":  "https://example.invalid/v0.2.0",
				"publishedAt": "2026-10-16T13:00:00Z",
				"notes":       "本版说明：修好了 X",
				"assets": []map[string]any{{
					"name": "inspiration_0.2.0_darwin_arm64.zip", "os": "darwin", "arch": "arm64",
					"kind": "zip", "size": len(payload), "sha256": hex.EncodeToString(sum[:]),
				}},
			})
		case strings.HasSuffix(r.URL.Path, "/releases/download/v0.2.0/inspiration_0.2.0_darwin_arm64.zip"):
			_, _ = w.Write(payload)
		case strings.Contains(r.URL.Path, "/repos/"):
			atomic.AddInt32(&apiHits, 1)
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"API rate limit exceeded for 1.2.3.4"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv, "0.1.0", "darwin", "arm64")
	c.DownloadBaseURL = srv.URL

	info, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("清单可用时检查更新不应失败：%v", err)
	}
	if info.UpToDate {
		t.Error("0.1.0 → 0.2.0 应判定为有新版本")
	}
	if info.LatestVersion != "0.2.0" {
		t.Errorf("最新版本应为 0.2.0，得到 %q", info.LatestVersion)
	}
	if info.AssetName != "inspiration_0.2.0_darwin_arm64.zip" {
		t.Errorf("资产选择错误：%q", info.AssetName)
	}
	if info.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("摘要应来自清单：%q", info.SHA256)
	}
	if !strings.Contains(info.Notes, "本版说明") {
		t.Errorf("更新说明应来自清单：%q", info.Notes)
	}
	if info.PublishedAt == "" {
		t.Error("发布时间应来自清单")
	}
	if atomic.LoadInt32(&apiHits) != 0 {
		t.Errorf("清单可用时不应请求 API（此刻 API 是 403），实际请求了 %d 次", apiHits)
	}

	// 清单里构造出来的下载地址也要能用（含 sha256 校验）
	dst := filepath.Join(t.TempDir(), "pkg.zip")
	got, err := c.Download(context.Background(), info.AssetURL, dst, info.AssetSize, nil)
	if err != nil {
		t.Fatalf("按清单地址下载失败：%v", err)
	}
	if got != info.SHA256 {
		t.Errorf("下载摘要不符：%q vs %q", got, info.SHA256)
	}
}

// 清单不可用时回退 API；两条路都失败时，错误里必须带上服务端说明——
// 否则用户只能看到"403 Forbidden"，无从判断是自己网络问题还是接口限额。
func TestCheckSurfacesRateLimitMessageWhenManifestMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repos/") {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"API rate limit exceeded for 1.2.3.4"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound) // 老版本发布没有 latest.json
	}))
	defer srv.Close()

	c := newTestClient(srv, "0.1.0", "darwin", "arm64")
	c.DownloadBaseURL = srv.URL

	_, err := c.Check(context.Background())
	if err == nil {
		t.Fatal("两条路都失败时必须报错")
	}
	msg := err.Error()
	for _, want := range []string{"rate limit", "403", "超限"} {
		if !strings.Contains(msg, want) {
			t.Errorf("错误信息应包含 %q：%v", want, err)
		}
	}
}

// 老版本发布（没有 latest.json）仍能通过 API 正常检查更新。
func TestCheckFallsBackToAPIWhenManifestMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repos/") {
			_ = json.NewEncoder(w).Encode(Release{TagName: "v0.3.0", Name: "灵感 v0.3.0", Body: "说明"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(srv, "0.1.0", "darwin", "arm64")
	c.DownloadBaseURL = srv.URL

	info, err := c.Check(context.Background())
	if err != nil {
		t.Fatalf("回退 API 时应成功：%v", err)
	}
	if info.LatestVersion != "0.3.0" {
		t.Errorf("最新版本应为 0.3.0，得到 %q", info.LatestVersion)
	}
}
