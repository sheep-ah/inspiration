// Package updater 实现应用内更新：检查 GitHub Releases、下载、校验完整性、按平台安装。
//
// 安全与隐私约束（来自任务书）：
//   - 只访问配置仓库的 releases/latest，拒绝 draft / prerelease；
//   - 只接受 HTTPS 且来源为 github.com 的下载地址；
//   - 下载后必须校验 sha256（优先用发布产物里的 checksums.txt）；
//   - 校验失败立即删除文件并报错，绝不安装；
//   - 失败可重试；更新过程不清空用户数据（模板与数据库在用户数据目录，不随应用包更新）。
package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

// DefaultRepo 是发布仓库。
const DefaultRepo = "sheep-ah/inspiration"

// DefaultBaseURL 是 GitHub API 根地址（测试可替换）。
const DefaultBaseURL = "https://api.github.com"

// DefaultDownloadBaseURL 是发布资产（latest.json 与安装包）的基地址。
// 走 github.com 的发布 CDN，不受 API 速率限制。
const DefaultDownloadBaseURL = "https://github.com"

// maxDownloadBytes 单个下载文件的大小上限（防止磁盘被写满）。
const maxDownloadBytes = 600 << 20

// 超时策略（都踩过真实的坑）：
//
//   - **不要设 http.Client.Timeout**。它覆盖"建立连接 + 重定向 + 读完整个响应体"，
//     而更新包有 6~18 MB：60 秒上限意味着必须 ≥166 KB/s（Windows 安装包 9.97 MB）才能
//     下完——跨境链路上稍慢就必然报"下载超时"。
//   - 传输时长交给调用方的 context（应用层给 30 分钟），建连与首字节单独设限。
//   - 真正该放弃下载的信号是"卡住不动"，由 downloadStallTimeout 处理。
const (
	downloadStallTimeout  = 90 * time.Second // 连续这么久没有收到任何数据 → 判定停滞并中止
	dialTimeout           = 30 * time.Second
	tlsHandshakeTimeout   = 15 * time.Second
	responseHeaderTimeout = 60 * time.Second // 只约束首字节，不限制传输总时长
)

// Asset 是发布资产。
type Asset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"` // 形如 "sha256:..."
}

// Release 是裁剪后的 GitHub Release。
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
	Assets      []Asset   `json:"assets"`
}

// Info 是"检查更新"的结果，直接给界面使用。
type Info struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	UpToDate       bool   `json:"upToDate"`
	Notes          string `json:"notes"`
	PublishedAt    string `json:"publishedAt"`
	ReleaseURL     string `json:"releaseUrl"`
	AssetName      string `json:"assetName"`
	AssetSize      int64  `json:"assetSize"`
	AssetURL       string `json:"assetUrl"`
	SHA256         string `json:"sha256"`
	Message        string `json:"message"`
}

// Client 是与 GitHub 交互的客户端。
type Client struct {
	Repo    string
	BaseURL string
	// DownloadBaseURL 是发布资产（latest.json / 安装包）的基地址；生产为 github.com，
	// 单测可指向 httptest 服务器。
	DownloadBaseURL string
	HTTP            *http.Client
	GOOS            string
	GOARCH          string
	currentV        string
	// allowLocalHTTP 仅供单元测试使用：允许访问本机 http 测试服务器。
	// 生产代码永远不会设置它，因此对外行为始终是"仅 HTTPS + 仅 GitHub 域名"。
	allowLocalHTTP bool
	// stallTimeout 是"多久没有收到数据就判定停滞"（默认 downloadStallTimeout）；
	// 单测会把它调小来构造停滞场景。
	stallTimeout time.Duration
}

// New 创建默认客户端。
func New(currentVersion string) *Client {
	return &Client{
		Repo:            DefaultRepo,
		BaseURL:         DefaultBaseURL,
		DownloadBaseURL: DefaultDownloadBaseURL,
		HTTP:            newHTTPClient(),
		GOOS:            runtime.GOOS,
		GOARCH:          runtime.GOARCH,
		currentV:        currentVersion,

		stallTimeout: downloadStallTimeout,
	}
}

// newHTTPClient 返回"只限制建连与首字节、不限制传输时长"的 HTTP 客户端。
func newHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   tlsHandshakeTimeout,
			ResponseHeaderTimeout: responseHeaderTimeout,
			IdleConnTimeout:       90 * time.Second,
			ExpectContinueTimeout: time.Second,
			ForceAttemptHTTP2:     true,
		},
	}
}

// Check 查询最新稳定版本，并与当前版本比较；同时挑选当前平台需要下载的资产。
func (c *Client) Check(ctx context.Context) (*Info, error) {
	rel, err := c.latestRelease(ctx)
	if err != nil {
		return nil, err
	}
	info := &Info{
		CurrentVersion: NormalizeTag(c.currentV),
		LatestVersion:  NormalizeTag(rel.TagName),
		Notes:          strings.TrimSpace(rel.Body),
		ReleaseURL:     rel.HTMLURL,
	}
	if !rel.PublishedAt.IsZero() {
		info.PublishedAt = rel.PublishedAt.Format(time.RFC3339)
	}
	if !IsNewer(c.currentV, rel.TagName) {
		info.UpToDate = true
		info.Message = "已是最新版本"
		return info, nil
	}
	asset, err := pickAsset(rel.Assets, c.GOOS, c.GOARCH)
	if err != nil {
		// 有新版但当前平台没有对应资产：如实告知，不静默失败
		info.Message = fmt.Sprintf("发现新版本 %s，但未找到适用于 %s/%s 的安装包；请前往发布页手动下载",
			FormatVersion(rel.TagName), c.GOOS, c.GOARCH)
		return info, nil
	}
	info.AssetName = asset.Name
	info.AssetSize = asset.Size
	info.AssetURL = asset.BrowserDownloadURL
	info.SHA256 = assetSHA256(*asset)
	info.Message = fmt.Sprintf("发现新版本 %s", FormatVersion(rel.TagName))
	return info, nil
}

func (c *Client) latestRelease(ctx context.Context) (*Release, error) {
	if c.Repo == "" {
		return nil, errors.New("未配置发布仓库")
	}
	// 优先读发布资产里的 latest.json：它由发布流水线生成，含版本、标签、更新说明与
	// 各平台资产的 sha256/体积，信息足够挑选与校验，而且走的是 github.com 的发布 CDN，
	// **不受 API 速率限制**（真实故障：用户点"检查更新"报 403 Forbidden——未登录调用
	// GitHub API 每小时只有 60 次、按 IP 计，同一出口 IP 的其它工具用光后应用就失败）。
	rel, manifestErr := c.latestReleaseFromManifest(ctx)
	if manifestErr == nil {
		return rel, nil
	}
	// 兜底：老版本发布没有 latest.json（或清单不可用）时回到 API
	rel, apiErr := c.latestReleaseFromAPI(ctx)
	if apiErr == nil {
		return rel, nil
	}
	return nil, fmt.Errorf("获取最新版本失败：%v；备用接口：%v", manifestErr, apiErr)
}

// releaseManifest 是 latest.json 的结构（由 release.yml 生成）。
type releaseManifest struct {
	Version     string `json:"version"`
	Tag         string `json:"tag"`
	ReleaseURL  string `json:"releaseUrl"`
	PublishedAt string `json:"publishedAt"`
	Notes       string `json:"notes"`
	Assets      []struct {
		Name   string `json:"name"`
		OS     string `json:"os"`
		Arch   string `json:"arch"`
		Kind   string `json:"kind"`
		Size   int64  `json:"size"`
		SHA256 string `json:"sha256"`
	} `json:"assets"`
}

// latestReleaseFromManifest 通过发布资产 latest.json 获取最新正式版本。
// 稳定地址：https://github.com/<repo>/releases/latest/download/latest.json
func (c *Client) latestReleaseFromManifest(ctx context.Context) (*Release, error) {
	base := strings.TrimRight(c.DownloadBaseURL, "/")
	if base == "" {
		base = DefaultDownloadBaseURL
	}
	endpoint := fmt.Sprintf("%s/%s/releases/latest/download/latest.json", base, c.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "inspiration-updater")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法连接更新服务（请检查网络）：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取版本清单失败：%s%s", resp.Status, httpStatusDetail(resp.Body))
	}

	var m releaseManifest
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&m); err != nil {
		return nil, fmt.Errorf("解析版本清单失败：%w", err)
	}
	if m.Tag == "" || len(m.Assets) == 0 {
		return nil, errors.New("版本清单内容不完整")
	}
	if err := validateTag(m.Tag); err != nil {
		return nil, err
	}

	rel := &Release{
		TagName: m.Tag,
		Name:    "灵感 " + m.Tag,
		Body:    strings.TrimSpace(m.Notes),
		HTMLURL: m.ReleaseURL,
	}
	if t, err := time.Parse(time.RFC3339, m.PublishedAt); err == nil {
		rel.PublishedAt = t
	}
	for _, a := range m.Assets {
		rel.Assets = append(rel.Assets, Asset{
			Name:               a.Name,
			Size:               a.Size,
			BrowserDownloadURL: fmt.Sprintf("%s/%s/releases/download/%s/%s", base, c.Repo, m.Tag, url.PathEscape(a.Name)),
			Digest:             "sha256:" + a.SHA256,
		})
	}
	return rel, nil
}

// latestReleaseFromAPI 走 GitHub API（受未登录速率限制）。
func (c *Client) latestReleaseFromAPI(ctx context.Context) (*Release, error) {

	if c.Repo == "" {
		return nil, errors.New("未配置发布仓库")
	}
	endpoint := fmt.Sprintf("%s/repos/%s/releases/latest", strings.TrimRight(c.BaseURL, "/"), c.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "inspiration-updater")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("无法连接更新服务（请检查网络）：%w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, errors.New("更新服务返回 404：仓库或 Release 不存在")
	default:
		return nil, fmt.Errorf("更新服务返回异常状态：%s%s", resp.Status, httpStatusDetail(resp.Body))
	}

	var rel Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil {
		return nil, fmt.Errorf("解析更新信息失败：%w", err)
	}
	if rel.Draft || rel.Prerelease {
		return nil, errors.New("最新 Release 是草稿或预发布版本，已忽略")
	}
	if err := validateTag(rel.TagName); err != nil {
		return nil, err
	}
	return &rel, nil
}

// pickAsset 按平台挑选要下载的资产。
// 优先顺序：安装包（setup）> 便携版；macOS 优先 zip（可整包替换）> dmg（需人工安装）。
// httpStatusDetail 尽力从错误响应体里取出服务端说明（例如速率限制的原因），
// 让用户看到"为什么失败"而不是只有一个 403。
func httpStatusDetail(body io.Reader) string {
	data, err := io.ReadAll(io.LimitReader(body, 8<<10))
	if err != nil || len(data) == 0 {
		return ""
	}
	var payload struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &payload) != nil {
		return ""
	}
	msg := strings.TrimSpace(payload.Message)
	if msg == "" {
		return ""
	}
	if strings.Contains(strings.ToLower(msg), "rate limit") {
		return "（" + msg + "；GitHub 未登录接口调用次数超限，稍后重试即可）"
	}
	return "（" + msg + "）"
}

func pickAsset(assets []Asset, goos, goarch string) (*Asset, error) {
	var preferred, fallback []string
	switch goos {
	case "darwin":
		preferred = []string{fmt.Sprintf("_darwin_%s.zip", goarch)}
		fallback = []string{fmt.Sprintf("_darwin_%s.dmg", goarch)}
	case "windows":
		preferred = []string{"_windows_amd64_setup.exe", "_windows_amd64_portable.exe"}
	default:
		return nil, fmt.Errorf("当前平台 %s/%s 不支持应用内更新", goos, goarch)
	}
	find := func(suffixes []string) *Asset {
		for _, suffix := range suffixes {
			for i := range assets {
				if strings.HasSuffix(strings.ToLower(assets[i].Name), suffix) {
					return &assets[i]
				}
			}
		}
		return nil
	}
	if a := find(preferred); a != nil {
		return a, nil
	}
	if a := find(fallback); a != nil {
		return a, nil
	}
	return nil, fmt.Errorf("未找到 %s/%s 的安装包", goos, goarch)
}

// assetSHA256 返回资产声明的 sha256（GitHub 的 digest 字段，形如 sha256:xxxx），没有则为空。
func assetSHA256(a Asset) string {
	if strings.HasPrefix(a.Digest, "sha256:") {
		return strings.TrimPrefix(a.Digest, "sha256:")
	}
	return ""
}

// checkHost 校验下载地址来源，防止被引导到第三方站点。
func (c *Client) checkHost(rawURL string) error {
	return checkHostWith(rawURL, c.allowLocalHTTP)
}

func checkHost(rawURL string) error { return checkHostWith(rawURL, false) }

func checkHostWith(rawURL string, allowLocalHTTP bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("下载地址无法解析：%w", err)
	}
	host := strings.ToLower(u.Hostname())
	if allowLocalHTTP && u.Scheme == "http" && (host == "127.0.0.1" || host == "localhost" || host == "::1") {
		return nil // 仅测试用
	}
	if u.Scheme != "https" {
		return fmt.Errorf("下载地址必须是 HTTPS：%s", rawURL)
	}
	allowed := []string{"github.com", "objects.githubusercontent.com", "release-assets.githubusercontent.com"}
	for _, a := range allowed {
		if host == a || strings.HasSuffix(host, "."+a) {
			return nil
		}
	}
	return fmt.Errorf("下载地址来源不可信：%s", host)
}

// Download 下载资产到 dstPath，并通过 progress 回调报告进度；返回实际 sha256。
func (c *Client) Download(ctx context.Context, assetURL, dstPath string, expectedSize int64, progress func(downloaded, total int64)) (string, error) {
	if err := c.checkHost(assetURL); err != nil {
		return "", err
	}
	// 停滞看门狗：只要还有数据进来就一直续命；连续 stall 时间没有数据就中止请求。
	// 这样既能容忍"慢但一直在走"的下载，又不会让断掉的连接一直挂到 30 分钟。
	//
	// 注意顺序：可取消的 ctx 必须在**构造请求之前**派生，否则 cancel() 影响不到
	// 已经在读的响应体（踩过：请求挂在 persistConn.Read 上直到测试超时）。
	stall := c.stallTimeout
	if stall <= 0 {
		stall = downloadStallTimeout
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var stalled atomic.Bool
	progressSignal := make(chan struct{}, 1)
	stopWatch := make(chan struct{})
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		timer := time.NewTimer(stall)
		defer timer.Stop()
		for {
			select {
			case <-stopWatch:
				return
			case <-progressSignal:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(stall)
			case <-timer.C:
				stalled.Store(true)
				cancel()
				return
			}
		}
	}()
	defer func() {
		close(stopWatch)
		<-watchDone
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "inspiration-updater")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		if stalled.Load() {
			return "", fmt.Errorf("下载停滞超过 %s（网络中断或速度过低），已中止，可重试", stall)
		}
		return "", fmt.Errorf("下载失败（请检查网络后重试）：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载失败，服务返回 %s", resp.Status)
	}
	total := expectedSize
	if total <= 0 {
		total = resp.ContentLength
	}
	if total > maxDownloadBytes {
		return "", fmt.Errorf("安装包体积异常（%d 字节），已中止", total)
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return "", fmt.Errorf("无法创建下载目录：%w", err)
	}
	f, err := os.Create(dstPath)
	if err != nil {
		return "", fmt.Errorf("无法创建下载文件：%w", err)
	}
	defer f.Close()

	hasher := sha256.New()
	writer := io.MultiWriter(f, hasher)
	buf := make([]byte, 256<<10)
	var written int64
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			select {
			case progressSignal <- struct{}{}:
			default:
			}
			if written+int64(n) > maxDownloadBytes {
				_ = os.Remove(dstPath)
				return "", errors.New("安装包超过允许的最大体积，已中止")
			}
			if _, wErr := writer.Write(buf[:n]); wErr != nil {
				_ = os.Remove(dstPath)
				return "", fmt.Errorf("写入下载文件失败：%w", wErr)
			}
			written += int64(n)
			if progress != nil {
				progress(written, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = os.Remove(dstPath)
			if stalled.Load() {
				return "", fmt.Errorf("下载停滞超过 %s（网络中断或速度过低），已中止，可重试", stall)
			}
			return "", fmt.Errorf("下载中断（可重试）：%w", readErr)
		}
	}
	if err := f.Sync(); err != nil {
		_ = os.Remove(dstPath)
		return "", fmt.Errorf("写入磁盘失败：%w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// VerifySHA256 校验文件摘要；want 为空表示跳过（调用方应至少用 checksums.txt 里的值）。
func VerifySHA256(path, want string) error {
	want = strings.TrimSpace(strings.ToLower(strings.TrimPrefix(want, "sha256:")))
	if want == "" {
		return errors.New("缺少校验和，拒绝安装")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		_ = os.Remove(path)
		return fmt.Errorf("文件校验失败（可能下载不完整或被篡改），已删除：期望 %s，实际 %s", want[:12], got[:12])
	}
	return nil
}

// ParseChecksums 解析 "sha256  文件名" 形式的校验和文件。
func ParseChecksums(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 {
			continue
		}
		sum := strings.ToLower(fields[0])
		if len(sum) != 64 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		out[name] = sum
	}
	return out
}

// FetchChecksums 从 Release 资产里取 checksums.txt 并解析。
func (c *Client) FetchChecksums(ctx context.Context, rel *Release) (map[string]string, error) {
	var target string
	for _, a := range rel.Assets {
		if strings.EqualFold(a.Name, "checksums.txt") {
			target = a.BrowserDownloadURL
			break
		}
	}
	if target == "" {
		return nil, errors.New("发布中缺少 checksums.txt，无法校验完整性")
	}
	if err := c.checkHost(target); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "inspiration-updater")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载校验和失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载校验和失败：%s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	sums := ParseChecksums(string(body))
	if len(sums) == 0 {
		return nil, errors.New("checksums.txt 内容为空或格式不正确")
	}
	return sums, nil
}
