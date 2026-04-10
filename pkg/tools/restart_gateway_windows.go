//go:build windows
// +build windows

package tools

import (
	"os"
)

// signalRestartProcess 在 Windows 上直接退出进程
func signalRestartProcess() error {
	// Windows 上没有 SIGUSR2，直接退出
	go func() {
		os.Exit(0)
	}()
	return nil
}
