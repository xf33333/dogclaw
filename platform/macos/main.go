//go:build darwin
// +build darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/getlantern/systray"
)

var gatewayCmd *exec.Cmd

func main() {
	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetTitle("🐾")
	systray.SetTooltip("DogClaw")

	mStart := systray.AddMenuItem("启动 Gateway", "启动 gateway 模式")
	mStop := systray.AddMenuItem("关闭 Gateway", "关闭 gateway")
	systray.AddSeparator()
	mChat := systray.AddMenuItem("聊天", "在浏览器中打开聊天界面")
	mSettings := systray.AddMenuItem("设置", "打开设置文件")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "退出应用")

	// 初始状态
	mStop.Disable()

	go func() {
		for {
			select {
			case <-mStart.ClickedCh:
				go startGateway(mStart, mStop)
			case <-mStop.ClickedCh:
				go stopGateway(mStart, mStop)
			case <-mChat.ClickedCh:
				openChat()
			case <-mSettings.ClickedCh:
				openSettings()
			case <-mQuit.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

func onExit() {
	if gatewayCmd != nil && gatewayCmd.Process != nil {
		_ = gatewayCmd.Process.Signal(syscall.SIGTERM)
		_ = gatewayCmd.Wait()
	}
}

func getExecutablePath() string {
	// 尝试找到 dogclaw 可执行文件
	// 1. 检查当前工作目录
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		dogclawPath := filepath.Join(exeDir, "dogclaw")
		if _, err := os.Stat(dogclawPath); err == nil {
			return dogclawPath
		}
	}

	// 2. 检查 PATH
	if path, err := exec.LookPath("dogclaw"); err == nil {
		return path
	}

	// 3. 尝试构建路径
	wd, _ := os.Getwd()
	return filepath.Join(wd, "dogclaw")
}

func startGateway(mStart, mStop *systray.MenuItem) {
	if gatewayCmd != nil {
		return
	}

	dogclawPath := getExecutablePath()

	gatewayCmd = exec.Command(dogclawPath, "gateway")
	gatewayCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// 重定向输出
	stdout, _ := gatewayCmd.StdoutPipe()
	stderr, _ := gatewayCmd.StderrPipe()

	if err := gatewayCmd.Start(); err != nil {
		gatewayCmd = nil
		return
	}

	// 读取输出（忽略，但保持管道打开）
	go func() {
		buf := make([]byte, 1024)
		for {
			_, _ = stdout.Read(buf)
		}
	}()
	go func() {
		buf := make([]byte, 1024)
		for {
			_, _ = stderr.Read(buf)
		}
	}()

	mStart.Disable()
	mStop.Enable()

	// 监控进程
	go func() {
		_ = gatewayCmd.Wait()
		gatewayCmd = nil
		mStart.Enable()
		mStop.Disable()
	}()
}

func stopGateway(mStart, mStop *systray.MenuItem) {
	if gatewayCmd == nil || gatewayCmd.Process == nil {
		return
	}

	// 杀掉进程组
	syscall.Kill(-gatewayCmd.Process.Pid, syscall.SIGTERM)
	_ = gatewayCmd.Wait()
	gatewayCmd = nil

	mStart.Enable()
	mStop.Disable()
}

func openChat() {
	_ = openBrowser("http://localhost:10800")
}

func openSettings() {
	homeDir, _ := os.UserHomeDir()
	settingsPath := filepath.Join(homeDir, ".dogclaw", "settings.json")
	_ = openFile(settingsPath)
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return nil
	}
	return cmd.Start()
}

func openFile(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", path)
	default:
		return nil
	}
	return cmd.Start()
}
