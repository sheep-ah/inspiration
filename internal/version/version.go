// Package version 保存应用版本信息。
//
// 版本号通过构建期注入（-ldflags "-X .../internal/version.Version=v1.0.0"），
// 保证「应用内可见版本 = Release 标签 = 产物文件名」三者一致。
package version

import "fmt"

var (
	// Version 语义化版本号，例如 v1.0.0（本地开发构建为 dev）。
	Version = "dev"
	// Commit 构建时的 git 提交短哈希。
	Commit = "unknown"
	// BuildTime 构建时间（UTC，RFC3339）。
	BuildTime = "unknown"
)

// Info 是给界面使用的版本信息。
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
}

// Info 返回当前版本信息。
func Get() Info {
	return Info{Version: Version, Commit: Commit, BuildTime: BuildTime}
}

// String 返回可读的单行版本描述。
func String() string {
	if Commit == "unknown" || Commit == "" {
		return Version
	}
	return fmt.Sprintf("%s (%s)", Version, Commit)
}

// IsDev 表示本地开发构建。
func IsDev() bool { return Version == "dev" || Version == "" }
