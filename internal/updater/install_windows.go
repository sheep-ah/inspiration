package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// applyPlatform（Windows）：
//   - 下载的是 NSIS 安装包（..._setup.exe）→ 退出后用 /S 静默安装，再启动应用；
//   - 下载的是便携版（..._portable.exe）→ 退出后覆盖当前 exe，再启动。
//
// Windows 会锁定正在运行的可执行文件，因此不能在进程内自我覆盖：
// 必须写一个批处理脚本，等当前进程退出后再动手。
func applyPlatform(ctx context.Context, opts ApplyOptions) (*ApplyResult, error) {
	exePath := opts.CurrentAppPath
	if exePath == "" {
		var err error
		exePath, err = os.Executable()
		if err != nil {
			return nil, fmt.Errorf("无法确定当前程序路径：%w", err)
		}
	}
	if !strings.HasSuffix(strings.ToLower(exePath), ".exe") {
		return nil, fmt.Errorf("当前程序路径不是 .exe：%s", exePath)
	}

	if opts.WorkDir == "" {
		opts.WorkDir = os.TempDir()
	}
	scriptPath := filepath.Join(opts.WorkDir, fmt.Sprintf("inspiration-update-%d.cmd", time.Now().Unix()))
	pid := os.Getpid()
	lower := strings.ToLower(opts.DownloadedPath)

	var method, script string
	if strings.HasSuffix(lower, "_setup.exe") || strings.HasSuffix(lower, "setup.exe") {
		method = "nsis-silent"
		script = fmt.Sprintf(`@echo off
setlocal
:wait
tasklist /FI "PID eq %d" 2>nul | find "%d" >nul
if not errorlevel 1 (
  timeout /t 1 /nobreak >nul
  goto wait
)
start "" /wait "%s" /S
if exist "%s" start "" "%s"
del "%%~f0"
`, pid, pid, opts.DownloadedPath, exePath, exePath)
	} else {
		method = "portable-swap"
		script = fmt.Sprintf(`@echo off
setlocal
:wait
tasklist /FI "PID eq %d" 2>nul | find "%d" >nul
if not errorlevel 1 (
  timeout /t 1 /nobreak >nul
  goto wait
)
copy /y "%s" "%s" >nul
if exist "%s" start "" "%s"
del "%%~f0"
`, pid, pid, opts.DownloadedPath, exePath, exePath, exePath)
	}

	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		return nil, fmt.Errorf("无法创建更新脚本：%w", err)
	}
	opts.Progress("正在准备重启并完成更新…")

	cmd := exec.Command("cmd", "/c", "start", "", "/min", scriptPath)
	cmd.SysProcAttr = detachedProcess()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("无法启动更新脚本：%w", err)
	}
	go func() { _ = cmd.Wait() }()

	return &ApplyResult{
		Method:       method,
		NeedsRestart: true,
		Message:      "更新包已下载并校验通过。应用将退出并在后台完成安装，随后自动重新启动。",
	}, nil
}
