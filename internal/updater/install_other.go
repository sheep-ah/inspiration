//go:build !darwin && !windows

package updater

import (
	"context"
	"errors"
)

// applyPlatform：前两个阶段只支持 Windows 与 macOS，其它平台引导手动下载。
func applyPlatform(ctx context.Context, opts ApplyOptions) (*ApplyResult, error) {
	return nil, errors.New("当前平台暂不支持应用内更新，请前往发布页手动下载")
}
