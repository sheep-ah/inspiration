package updater

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 更新包有 6~18 MB，因此客户端**不能**设 http.Client.Timeout：它把"读响应体"也算进去，
// 60 秒上限等于要求 ≥166 KB/s（Windows 安装包 9.97 MB），跨境链路上必然报"下载超时"。
func TestNewClientHasNoOverallTimeout(t *testing.T) {
	c := New("v1.0.0")
	if c.HTTP.Timeout != 0 {
		t.Errorf("http.Client.Timeout 必须为 0（整体超时会杀死正常但较慢的下载），得到 %v", c.HTTP.Timeout)
	}
	tr, ok := c.HTTP.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport 类型异常：%T", c.HTTP.Transport)
	}
	if tr.ResponseHeaderTimeout <= 0 {
		t.Error("应单独设置 ResponseHeaderTimeout，避免服务器不回响应时无限等待")
	}
	if tr.DialContext == nil {
		t.Error("应设置带超时的 DialContext")
	}
	if c.stallTimeout <= 0 {
		t.Error("应设置停滞超时（downloadStallTimeout）")
	}
}

// 停滞（连续很久收不到数据）必须中止，给出可读原因，并删掉半成品文件。
func TestDownloadAbortsWhenStalled(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		_, _ = w.Write([]byte("start")) // 先给一点数据，证明确实开始下载了
		w.(http.Flusher).Flush()
		<-release // 然后彻底不动
	}))
	defer srv.Close()
	defer close(release)

	c := newTestClient(srv, "1.0.0", "darwin", "arm64")
	c.stallTimeout = 300 * time.Millisecond

	dst := filepath.Join(t.TempDir(), "update.zip")
	started := time.Now()
	_, err := c.Download(context.Background(), srv.URL+"/file.zip", dst, 1000000, nil)
	if err == nil {
		t.Fatal("停滞的下载必须报错")
	}
	if !strings.Contains(err.Error(), "停滞") {
		t.Errorf("错误信息应说明是停滞：%v", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Errorf("停滞应在 stallTimeout 后很快中止，实际耗时 %v", elapsed)
	}
	if _, statErr := os.Stat(dst); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("半成品文件应被删除，stat err=%v", statErr)
	}
}

// 慢但一直在走（每段间隔都小于 stall 超时）不能被杀掉——这是"慢网络"与"卡死"的区别。
func TestDownloadToleratesSlowButProgressingTransfer(t *testing.T) {
	const chunks, chunkSize = 5, 64 << 10
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(chunks*chunkSize))
		for range chunks {
			_, _ = w.Write(bytes.Repeat([]byte("x"), chunkSize))
			w.(http.Flusher).Flush()
			time.Sleep(60 * time.Millisecond)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv, "1.0.0", "darwin", "arm64")
	c.stallTimeout = 600 * time.Millisecond

	dst := filepath.Join(t.TempDir(), "slow.zip")
	got, err := c.Download(context.Background(), srv.URL+"/slow.zip", dst, chunks*chunkSize, nil)
	if err != nil {
		t.Fatalf("慢但持续有数据的下载不应失败：%v", err)
	}
	if err := VerifySHA256(dst, got); err != nil {
		t.Errorf("摘要校验失败：%v", err)
	}
	info, statErr := os.Stat(dst)
	if statErr != nil || info.Size() != chunks*chunkSize {
		t.Errorf("文件大小应为 %d，err=%v", chunks*chunkSize, statErr)
	}
}
