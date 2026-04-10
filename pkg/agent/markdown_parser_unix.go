//go:build !windows
// +build !windows

package agent

import (
	"fmt"
	"os"
	"syscall"
)

// GetFileIdentity 获取文件的唯一标识（设备:inode）
func getFileIdentity(filePath string) (string, error) {
	// 使用 os.Stat 获取文件信息
	info, err := os.Stat(filePath)
	if err != nil {
		return "", err
	}

	// 尝试获取底层 stat 信息（Unix/Linux 可靠）
	stat_t, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		// 无法获取底层 stat，返回路径作为 fallback
		return filePath, nil
	}

	// 某些网络文件系统（NFS、FUSE）返回 dev=0 和 ino=0，这会导致每个文件看起来都是重复的
	// 返回空字符串表示不可靠的标识，会跳过去重
	if stat_t.Dev == 0 && stat_t.Ino == 0 {
		return "", nil
	}

	return fmt.Sprintf("%d:%d", stat_t.Dev, stat_t.Ino), nil
}
