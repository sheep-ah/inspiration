// Package sysproc 统一处理子进程在 Windows 上的控制台窗口行为。
//
// 背景：灵感是 GUI 子系统程序（自身没有控制台）。Windows 上启动控制台程序
// （cmd、powershell、reg 等）时，若不在 STARTUPINFO 里指定隐藏窗口，系统会为
// 子进程分配一个新的控制台窗口——用户会看到黑框一闪而过，或者整个转换期间挂着
// 一个"命令行执行脚本"的窗口。对不懂技术的用户来说，这看起来像程序在偷偷执行
// 可疑脚本。
//
// 因此**所有启动子进程的地方都必须调用 HideWindow**；
// 例外只有更新流程里的批处理（它需要 `timeout /t` 等待当前进程退出，而
// timeout.exe 依赖控制台，见 internal/updater）。
package sysproc
