//go:build !windows
// +build !windows

package tools

import (
	"os"
	"syscall"
)

// signalRestartProcess 向当前进程发送重启信号
func signalRestartProcess() error {
	pid := os.Getpid()
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(syscall.SIGUSR2)
}
