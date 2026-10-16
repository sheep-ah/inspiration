// Package converter 通过本机已有的文档转换器，把 .docx 转成真正的 Word 97-2003 .doc（以及反向导入）。
//
// 「最强兼容」策略：
//  1. 多通道，按保真度排序尝试：Microsoft Word（原厂引擎，几乎不走样）→ WPS Office → LibreOffice；
//  2. 产物强校验：必须是真 OLE2/CFB 结构（下游按文件头解析也能通过），并回读文本比对，防止静默损坏；
//  3. 任何通道都不可用时：明确报错，绝不产出"改了扩展名"的假 .doc；
//  4. 只调用本机进程，不联网、不上传任何内容。
package converter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sheep-ah/inspiration/internal/docx"
	"github.com/sheep-ah/inspiration/internal/sysproc"
)

// EnvSoffice 可用于指定 LibreOffice 可执行文件路径（测试或非标准安装）。
const EnvSoffice = "INSPIRATION_SOFFICE"

// EnvChannel 可强制指定转换通道（word / wps / libreoffice），便于排查问题。
const EnvChannel = "INSPIRATION_CONVERTER"

// cfbMagic 是 OLE2 复合文档文件头；真正的 .doc 必须是这个格式。
var cfbMagic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

const convertTimeout = 120 * time.Second

// 通道标识。
const (
	KindWord        = "word"
	KindWPS         = "wps"
	KindLibreOffice = "libreoffice"
)

// 单通道状态。
type ChannelStatus struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Path      string `json:"path"`
	Detail    string `json:"detail"`
}

// Status 是转换能力总览：Available 表示"至少有可用通道"，Kind/Name 表示会实际使用哪个。
type Status struct {
	Available bool            `json:"available"`
	Kind      string          `json:"kind"`
	Name      string          `json:"name"`
	Path      string          `json:"path"`
	Detail    string          `json:"detail"`
	Channels  []ChannelStatus `json:"channels"`
}

// Converter 封装外部转换器。
type Converter struct {
	// profileDir 是 LibreOffice 专用用户配置目录（位于应用数据目录下）。
	profileDir string
	// bin 覆盖 LibreOffice 可执行文件（测试用）。
	bin string
	// forceKind 强制使用某通道（测试或排查用）。
	forceKind string
}

// New 创建转换器。profileDir 建议传 <应用数据目录>/convert。
func New(profileDir string) *Converter {
	return &Converter{profileDir: profileDir}
}

// NewWithBinary 指定 LibreOffice 路径（测试用）。
func NewWithBinary(profileDir, bin string) *Converter {
	return &Converter{profileDir: profileDir, bin: bin}
}

// Channels 返回所有通道的探测结果（按优先级排序）。
func (c *Converter) Channels() []ChannelStatus {
	names := map[string]string{
		KindWord:        "Microsoft Word",
		KindWPS:         "WPS Office",
		KindLibreOffice: "LibreOffice",
	}
	var out []ChannelStatus
	for _, kind := range []string{KindWord, KindWPS, KindLibreOffice} {
		st := c.detectChannel(kind)
		st.Name = names[kind]
		out = append(out, st)
	}
	return out
}

// Detect 返回将会使用的通道（未强制指定时取优先级最高的可用通道）。
func (c *Converter) Detect() Status {
	channels := c.Channels()
	status := Status{Channels: channels}
	order := []string{KindWord, KindWPS, KindLibreOffice}
	if forced := c.forcedKind(); forced != "" {
		order = []string{forced}
	}
	for _, kind := range order {
		for _, ch := range channels {
			if ch.Kind == kind && ch.Available {
				status.Available = true
				status.Kind = ch.Kind
				status.Name = ch.Name
				status.Path = ch.Path
				status.Detail = ch.Detail
				return status
			}
		}
	}
	status.Detail = "未检测到可用的 Word/WPS/LibreOffice，无法生成 .doc 文件"
	return status
}

// Available 表示至少有一个可用通道。
func (c *Converter) Available() bool { return c.Detect().Available }

func (c *Converter) forcedKind() string {
	if c.forceKind != "" {
		return c.forceKind
	}
	return strings.ToLower(strings.TrimSpace(os.Getenv(EnvChannel)))
}

// resolve 返回实际使用的通道。
func (c *Converter) resolve() (kind, bin string, err error) {
	st := c.Detect()
	if !st.Available {
		return "", "", errors.New("未检测到可用的 Word/WPS/LibreOffice，无法生成 .doc 文件")
	}
	return st.Kind, st.Path, nil
}

// ---------------------------------------------------------------------------
// 通道探测

func (c *Converter) detectChannel(kind string) ChannelStatus {
	switch kind {
	case KindWord:
		return detectWordAutomation()
	case KindWPS:
		return detectWPS()
	case KindLibreOffice:
		return c.detectLibreOffice()
	}
	return ChannelStatus{Kind: kind, Detail: "未知通道"}
}

// detectWordAutomation：macOS 看是否安装了 Microsoft Word；Windows 看是否注册了 Word COM 组件。
func detectWordAutomation() ChannelStatus {
	st := ChannelStatus{Kind: KindWord}
	switch runtime.GOOS {
	case "darwin":
		const appPath = "/Applications/Microsoft Word.app"
		if !dirExists(appPath) {
			st.Detail = "未安装 Microsoft Word"
			return st
		}
		if _, err := exec.LookPath("osascript"); err != nil {
			st.Detail = "系统缺少 osascript（无法驱动 Word）"
			return st
		}
		st.Available = true
		st.Path = appPath
		st.Detail = "Microsoft Word 已安装（使用 AppleScript 另存为 Word 97-2003）"
		return st
	case "windows":
		ps, err := exec.LookPath("powershell.exe")
		if err != nil {
			if ps, err = exec.LookPath("powershell"); err != nil {
				st.Detail = "系统缺少 PowerShell"
				return st
			}
		}
		if !regProgIDExists("Word.Application") {
			st.Detail = "未安装 Microsoft Word（未注册 Word.Application）"
			return st
		}
		st.Available = true
		st.Path = ps
		st.Detail = "Microsoft Word 已安装（使用 COM 另存为 Word 97-2003）"
		return st
	default:
		st.Detail = "当前平台不支持 Word 自动化"
		return st
	}
}

// detectWPS：仅 Windows 支持（WPS 注册了 COM 组件）。
func detectWPS() ChannelStatus {
	st := ChannelStatus{Kind: KindWPS}
	if runtime.GOOS != "windows" {
		st.Detail = "当前平台不支持 WPS 自动化"
		return st
	}
	ps, err := exec.LookPath("powershell.exe")
	if err != nil {
		if ps, err = exec.LookPath("powershell"); err != nil {
			st.Detail = "系统缺少 PowerShell"
			return st
		}
	}
	for _, progID := range []string{"KWPS.Application", "WPS.Application"} {
		if regProgIDExists(progID) {
			st.Available = true
			st.Path = ps
			st.Detail = "WPS Office 已安装（使用 COM 另存为 Word 97-2003）"
			return st
		}
	}
	// WPS 有时会占用 Word.Application 这个 ProgID
	if regProgIDExists("Word.Application") && !dirExists(`C:\Program Files\Microsoft Office`) {
		st.Available = true
		st.Path = ps
		st.Detail = "检测到 WPS 注册了 Word.Application（使用 COM 另存为 Word 97-2003）"
		return st
	}
	st.Detail = "未安装 WPS Office"
	return st
}

func (c *Converter) detectLibreOffice() ChannelStatus {
	st := ChannelStatus{Kind: KindLibreOffice}
	if c.bin != "" {
		if !fileExists(c.bin) {
			st.Detail = fmt.Sprintf("指定的转换器不存在：%s", c.bin)
			return st
		}
		st.Available = true
		st.Path = c.bin
		st.Detail = "LibreOffice 已就绪（文本框/图文框位置可能与原稿不同）"
		return st
	}
	if v := strings.TrimSpace(os.Getenv(EnvSoffice)); v != "" {
		if !fileExists(v) {
			st.Detail = fmt.Sprintf("环境变量 %s 指向的文件不存在：%s", EnvSoffice, v)
			return st
		}
		st.Available = true
		st.Path = v
		st.Detail = "LibreOffice 已就绪（文本框/图文框位置可能与原稿不同）"
		return st
	}
	for _, cand := range libreOfficePaths() {
		if fileExists(cand) {
			st.Available = true
			st.Path = cand
			st.Detail = "LibreOffice 已就绪（文本框/图文框位置可能与原稿不同）"
			return st
		}
	}
	if p, err := exec.LookPath("soffice"); err == nil {
		st.Available = true
		st.Path = p
		st.Detail = "LibreOffice 已就绪（文本框/图文框位置可能与原稿不同）"
		return st
	}
	st.Detail = "未安装 LibreOffice"
	return st
}

func libreOfficePaths() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/LibreOffice.app/Contents/MacOS/soffice",
			"/opt/homebrew/bin/soffice",
			"/usr/local/bin/soffice",
		}
	case "windows":
		out := []string{
			`C:\Program Files\LibreOffice\program\soffice.exe`,
			`C:\Program Files (x86)\LibreOffice\program\soffice.exe`,
		}
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			out = append([]string{filepath.Join(la, "Programs", "LibreOffice", "program", "soffice.exe")}, out...)
		}
		return out
	default:
		return []string{
			"/usr/bin/soffice",
			"/usr/lib/libreoffice/program/soffice",
			"/snap/bin/libreoffice",
			"/usr/local/bin/soffice",
		}
	}
}

// regProgIDExists 检查 Windows 注册表中是否存在某个 COM ProgID。
func regProgIDExists(progID string) bool {
	cmd := exec.Command("reg", "query", `HKCR\`+progID, "/ve")
	sysproc.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return bytes.Contains(out, []byte(progID))
}

// ---------------------------------------------------------------------------
// 对外转换接口

// ToDoc 把 .docx 转换为真正的 Word 97-2003 .doc，并做完整性自检。
func (c *Converter) ToDoc(ctx context.Context, srcDocx, dstDoc string) error {
	kind, bin, err := c.resolve()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dstDoc), 0o755); err != nil {
		return fmt.Errorf("创建输出目录失败：%w", err)
	}
	if err := c.convert(ctx, kind, bin, srcDocx, filepath.Dir(dstDoc), "doc"); err != nil {
		return err
	}
	produced := filepath.Join(filepath.Dir(dstDoc), replaceExt(filepath.Base(srcDocx), ".doc"))
	if produced != dstDoc {
		if err := os.Rename(produced, dstDoc); err != nil {
			return fmt.Errorf("保存转换结果失败：%w", err)
		}
	}
	return c.selfCheck(ctx, srcDocx, dstDoc)
}

// ToDocx 把 .doc 转换为 .docx（用于 .doc 模板导入与自检回读）。
func (c *Converter) ToDocx(ctx context.Context, srcDoc, dstDocx string) error {
	kind, bin, err := c.resolve()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dstDocx), 0o755); err != nil {
		return fmt.Errorf("创建输出目录失败：%w", err)
	}
	if err := c.convert(ctx, kind, bin, srcDoc, filepath.Dir(dstDocx), "docx"); err != nil {
		return err
	}
	produced := filepath.Join(filepath.Dir(dstDocx), replaceExt(filepath.Base(srcDoc), ".docx"))
	if produced != dstDocx {
		if err := os.Rename(produced, dstDocx); err != nil {
			return fmt.Errorf("保存转换结果失败：%w", err)
		}
	}
	return nil
}

// convert 按通道执行一次转换。format 为 "doc" 或 "docx"。
func (c *Converter) convert(ctx context.Context, kind, bin, srcPath, outDir, format string) error {
	ctx, cancel := context.WithTimeout(ctx, convertTimeout)
	defer cancel()

	switch kind {
	case KindWord:
		return c.runWord(ctx, bin, srcPath, outDir, format)
	case KindWPS:
		return c.runWPS(ctx, bin, srcPath, outDir, format)
	default:
		return c.runLibreOffice(ctx, bin, srcPath, outDir, format)
	}
}

// ---------------------------------------------------------------------------
// 各通道实现

func (c *Converter) runLibreOffice(ctx context.Context, bin, srcPath, outDir, format string) error {
	filter := "docx"
	if format == "doc" {
		filter = `doc:"MS Word 97"`
	}
	profileDir := c.profileDir
	if profileDir == "" {
		profileDir = filepath.Join(os.TempDir(), "inspiration-convert-profile")
	}
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		return fmt.Errorf("创建转换器配置目录失败：%w", err)
	}
	// 注意：LibreOffice 只认单横线的 -env:Name=Value（写成 --env: 会报 Invalid URL）
	args := []string{
		"--headless", "--norestore",
		"-env:UserInstallation=" + fileURL(profileDir),
		"--convert-to", filter,
		"--outdir", outDir,
		srcPath,
	}
	err := runCommand(ctx, bin, args...)
	if err != nil && format == "doc" {
		// 个别 LibreOffice 版本对过滤器名不敏感，按扩展名再试一次（注意 args[4] 才是过滤器）
		args[4] = "doc"
		return runCommand(ctx, bin, args...)
	}
	return err
}

// runWord：macOS 用 AppleScript 驱动 Microsoft Word；Windows 用 COM。
func (c *Converter) runWord(ctx context.Context, bin, srcPath, outDir, format string) error {
	dst := filepath.Join(outDir, replaceExt(filepath.Base(srcPath), "."+format))
	if runtime.GOOS == "darwin" {
		return c.runAppleScriptWord(ctx, srcPath, dst, format)
	}
	return c.runPowerShellWord(ctx, bin, "Word.Application", srcPath, dst, format)
}

func (c *Converter) runWPS(ctx context.Context, bin, srcPath, outDir, format string) error {
	dst := filepath.Join(outDir, replaceExt(filepath.Base(srcPath), "."+format))
	var lastErr error
	for _, progID := range []string{"KWPS.Application", "WPS.Application", "Word.Application"} {
		if err := c.runPowerShellWord(ctx, bin, progID, srcPath, dst, format); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if fileExists(dst) {
			_ = os.Remove(dst)
		}
	}
	return fmt.Errorf("WPS 转换失败：%w", lastErr)
}

// runPowerShellWord 通过 Word/WPS 的 COM 接口另存为指定格式。
// wdFormatDocument(0) = Word 97-2003；wdFormatXMLDocument(12) = .docx。
func (c *Converter) runPowerShellWord(ctx context.Context, powerShell, progID, srcPath, dstPath, format string) error {
	scriptDir, err := os.MkdirTemp("", "inspiration-ps-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败：%w", err)
	}
	defer os.RemoveAll(scriptDir)
	scriptPath := filepath.Join(scriptDir, "convert.ps1")
	if err := os.WriteFile(scriptPath, []byte(powerShellScript), 0o600); err != nil {
		return fmt.Errorf("写入 PowerShell 脚本失败：%w", err)
	}
	formatCode := "0" // wdFormatDocument：Word 97-2003
	if format == "docx" {
		formatCode = "12" // wdFormatXMLDocument
	}
	args := []string{
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-File", scriptPath,
		"-ProgId", progID,
		"-Source", srcPath,
		"-Target", dstPath,
		"-FormatCode", formatCode,
	}
	if err := runCommand(ctx, powerShell, args...); err != nil {
		return err
	}
	if !fileExists(dstPath) {
		return fmt.Errorf("%s 未生成目标文件", progID)
	}
	return nil
}

// runAppleScriptWord：用 AppleScript 让 Microsoft Word 打开并另存。
func (c *Converter) runAppleScriptWord(ctx context.Context, srcPath, dstPath, format string) error {
	scriptDir, err := os.MkdirTemp("", "inspiration-as-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败：%w", err)
	}
	defer os.RemoveAll(scriptDir)
	scriptPath := filepath.Join(scriptDir, "convert.applescript")
	body := appleScriptWordDoc
	if format == "docx" {
		body = appleScriptWordDocx
	}
	if err := os.WriteFile(scriptPath, []byte(body), 0o600); err != nil {
		return fmt.Errorf("写入 AppleScript 失败：%w", err)
	}
	if err := runCommand(ctx, "osascript", scriptPath, srcPath, dstPath); err != nil {
		return err
	}
	if !fileExists(dstPath) {
		return errors.New("未能生成目标文件（Microsoft Word 可能弹出了对话框，或没有自动化权限）")
	}
	return nil
}

// powerShellScript 通过 COM 调用 Word/WPS 另存。
// 安全设置：关闭宏（AutomationSecurity=3）、不弹对话框、只读打开、转换后退出。
const powerShellScript = `param(
  [Parameter(Mandatory=$true)][string]$ProgId,
  [Parameter(Mandatory=$true)][string]$Source,
  [Parameter(Mandatory=$true)][string]$Target,
  [Parameter(Mandatory=$true)][int]$FormatCode
)
$ErrorActionPreference = 'Stop'
$app = $null
$doc = $null
try {
  $app = New-Object -ComObject $ProgId
  try { $app.Visible = $false } catch {}
  try { $app.DisplayAlerts = 0 } catch {}
  try { $app.AutomationSecurity = 3 } catch {}   # msoAutomationSecurityForceDisable：禁止宏
  try { $app.Options.ConfirmConversions = $false } catch {}
  $doc = $app.Documents.Open($Source, $false, $true)   # ConfirmConversions=$false, ReadOnly=$true
  $doc.SaveAs2($Target, $FormatCode)
  $doc.Close(0)
  $doc = $null
  Write-Output "OK"
} catch {
  Write-Error $_.Exception.Message
  exit 1
} finally {
  if ($doc -ne $null) { try { $doc.Close(0) } catch {} }
  if ($app -ne $null) { try { $app.Quit(0) } catch {} }
}
`

// AppleScript：Microsoft Word 另存为 Word 97-2003。
const appleScriptWordDoc = `on run argv
	set srcPath to item 1 of argv
	set dstPath to item 2 of argv
	tell application "Microsoft Word"
		set theDoc to open (POSIX file srcPath)
		save as theDoc file name dstPath file format format document 97
		close theDoc saving no
	end tell
	return "OK"
end run
`

// AppleScript：Microsoft Word 另存为 .docx。
const appleScriptWordDocx = `on run argv
	set srcPath to item 1 of argv
	set dstPath to item 2 of argv
	tell application "Microsoft Word"
		set theDoc to open (POSIX file srcPath)
		save as theDoc file name dstPath file format format document
		close theDoc saving no
	end tell
	return "OK"
end run
`

// ---------------------------------------------------------------------------
// 自检

// selfCheck 验证产物确实是 .doc，并回读比对文本，防止"静默损坏"。
func (c *Converter) selfCheck(ctx context.Context, srcDocx, dstDoc string) error {
	info, err := os.Stat(dstDoc)
	if err != nil {
		return fmt.Errorf("转换未生成文件：%w", err)
	}
	if info.Size() == 0 {
		return errors.New("转换生成的文件为空")
	}
	magic := make([]byte, len(cfbMagic))
	f, err := os.Open(dstDoc)
	if err != nil {
		return err
	}
	n, readErr := f.Read(magic)
	_ = f.Close()
	if readErr != nil || n < len(cfbMagic) || !bytes.Equal(magic, cfbMagic) {
		_ = os.Remove(dstDoc)
		return errors.New("转换结果不是有效的 .doc（OLE2）文件，已丢弃")
	}

	original, err := docx.ExtractText(srcDocx)
	if err != nil {
		return fmt.Errorf("读取原始文件失败：%w", err)
	}
	roundTripDir, err := os.MkdirTemp("", "inspiration-convert-back-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(roundTripDir)

	back := filepath.Join(roundTripDir, "back.docx")
	if err := c.ToDocx(ctx, dstDoc, back); err != nil {
		_ = os.Remove(dstDoc)
		return fmt.Errorf("转换结果无法回读（可能已损坏），已丢弃：%w", err)
	}
	roundTrip, err := docx.ExtractText(back)
	if err != nil {
		_ = os.Remove(dstDoc)
		return fmt.Errorf("转换结果无法回读（可能已损坏），已丢弃：%w", err)
	}
	if missing := missingLines(original, roundTrip); len(missing) > 0 {
		_ = os.Remove(dstDoc)
		return fmt.Errorf("转换结果自检未通过：有 %d 行内容在转换后丢失，已丢弃（例如：%s）",
			len(missing), strings.Join(firstN(missing, 3), " / "))
	}
	return nil
}

// ---------------------------------------------------------------------------
// 工具

func runCommand(ctx context.Context, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	// Windows 上是 GUI 程序：不抑制的话，PowerShell / LibreOffice 会各弹一个
	// 控制台窗口（用户看到的就是"黑框执行脚本一闪"）。
	sysproc.HideWindow(cmd)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("转换超时（超过 %s）", convertTimeout)
	}
	if err != nil {
		return fmt.Errorf("转换失败：%v；输出：%s", err, tailText(out.String(), 400))
	}
	return nil
}

// fileURL 把本地目录转成 LibreOffice 能接受的 file URL（必须做百分号编码：
// 应用数据目录在 macOS 下形如 "~/Library/Application Support/灵感/"，含空格与非 ASCII）。
func fileURL(dir string) string {
	slash := filepath.ToSlash(dir)
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash // Windows: C:/Users/x -> /C:/Users/x
	}
	u := url.URL{Scheme: "file", Path: slash}
	return u.String()
}

// normalizeLines 归一化文本：去 BOM、统一换行、逐行 trim、去空行、压缩连续空白。
func normalizeLines(text string) []string {
	text = strings.TrimPrefix(text, "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.Join(strings.Fields(line), " "))
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

// missingLines 返回"原文有、回读后按行集合找不到"的行。
// 只报告丢失，不报告多出来的行：转换器可能把同一个文本框输出成两个分支（Choice/Fallback）导致重复行，
// 这属于已知且无害的差异。
func missingLines(original, roundTrip string) []string {
	have := map[string]bool{}
	for _, line := range normalizeLines(roundTrip) {
		have[line] = true
	}
	var missing []string
	for _, line := range normalizeLines(original) {
		if !have[line] {
			missing = append(missing, line)
		}
	}
	return missing
}

func replaceExt(name, ext string) string {
	return strings.TrimSuffix(name, filepath.Ext(name)) + ext
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func tailText(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func firstN(list []string, n int) []string {
	if len(list) <= n {
		return list
	}
	return list[:n]
}
