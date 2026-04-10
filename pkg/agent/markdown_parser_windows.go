//go:build windows
// +build windows

package agent

import (
	"os"
)

// GetFileIdentity 获取文件的唯一标识
// 在 Windows 上简单返回文件路径
func getFileIdentity(filePath string) (string, error) {
	// 使用 os.Stat 检查文件是否存在
	_, err := os.Stat(filePath)
	if err != nil {
		return "", err
	}

	// Windows 上直接返回路径作为标识
	return filePath, nil
}
