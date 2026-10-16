package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/sheep-ah/inspiration/internal/sysproc"
)

const (
	createDetachedProcess = 0x00000008 // DETACHED_PROCESS：不继承父进程控制台，父进程退出后继续运行
	createNewProcessGroup = 0x00000200 // CREATE_NEW_PROCESS_GROUP
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
	method, script := windowsUpdateScript(os.Getpid(), opts.DownloadedPath, exePath)

	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		return nil, fmt.Errorf("无法创建更新脚本：%w", err)
	}
	opts.Progress("正在准备重启并完成更新…")

	// 直接执行脚本（不再用 `start /min`）：脚本内改用 ping 做延时，不需要控制台，
	// 因此窗口可以完全隐藏，全程不会出现命令行窗口。
	//
	// 路径通过环境变量传入（脚本本身保持纯 ASCII）：cmd.exe 按控制台代码页解码脚本
	// 字节，中文安装路径（「灵感」）写进 .cmd 会变成乱码，导致覆盖/重启指向不存在的路径。
	cmd := exec.Command("cmd", "/c", scriptPath)
	cmd.Env = append(os.Environ(),
		envUpdatePkg+"="+opts.DownloadedPath,
		envUpdateExe+"="+exePath,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createDetachedProcess | createNewProcessGroup,
	}
	sysproc.HideWindow(cmd)
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
