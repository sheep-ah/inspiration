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
)

const (
	// CREATE_NEW_CONSOLE + SW_HIDE（见 newUpdateScriptCommand）：给脚本一个**隐藏的控制台**。
	//
	// 不能用 DETACHED_PROCESS/CREATE_NO_WINDOW——那样 cmd 自己没有控制台，它随后启动的每个
	// 控制台子命令（ping 等）都会被系统新开一个窗口（真实故障：更新时弹出一个标题为
	// `find "45152"` 的窗口并卡住，用户以为程序死了）。有了隐藏控制台，子命令共用它，
	// 既不弹窗、管道与延时命令也都能正常工作。
	createNewConsole      = 0x00000010 // CREATE_NEW_CONSOLE
	createNewProcessGroup = 0x00000200 // CREATE_NEW_PROCESS_GROUP：脱离父进程的 Ctrl 事件
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

	// 路径通过环境变量传入（脚本本身保持纯 ASCII）：cmd.exe 按控制台代码页解码脚本
	// 字节，中文安装路径（「灵感」）写进 .cmd 会变成乱码，导致覆盖/重启指向不存在的路径。
	cmd := newUpdateScriptCommand(scriptPath, append(os.Environ(),
		envUpdatePkg+"="+opts.DownloadedPath,
		envUpdateExe+"="+exePath,
		envUpdateExeName+"="+filepath.Base(exePath),
	))
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

// newUpdateScriptCommand 组装"以隐藏控制台运行更新脚本"的命令。
// 生产与 Windows 实测用例共用它，保证测试跑在与生产完全相同的启动方式下
// （否则会漏掉"控制台子命令各自开窗/管道行为不同"这类只在生产环境暴露的问题）。
func newUpdateScriptCommand(scriptPath string, env []string) *exec.Cmd {
	cmd := exec.Command("cmd", "/c", scriptPath)
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNewConsole | createNewProcessGroup,
		HideWindow:    true, // STARTF_USESHOWWINDOW + SW_HIDE：新建的控制台窗口隐藏
	}
	return cmd
}
