//go:build darwin
// +build darwin

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/getlantern/systray"
)

// ProviderModel 对应配置文件中的 provider model
type ProviderModel struct {
	Alias    string `json:"alias"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	URL      string `json:"url"`
	APIKey   string `json:"apiKey"`
}

// GatewaySettings 对应配置文件中的 gateway 配置
type GatewaySettings struct {
	Enabled bool `json:"enabled"`
	Port    int  `json:"port"`
}

// QQSettings 对应配置文件中的 QQ 配置
type QQSettings struct {
	Enabled      bool   `json:"enabled"`
	AppID        string `json:"appId"`
	AppSecret    string `json:"appSecret"`
	AllowFrom    string `json:"allowFrom"`
	SendMarkdown bool   `json:"sendMarkdown"`
}

// WeixinSettings 对应配置文件中的 Weixin 配置
type WeixinSettings struct {
	Enabled bool   `json:"enabled"`
	Token   string `json:"token"`
}

// ChannelSettings 对应配置文件中的 channel 配置
type ChannelSettings struct {
	QQ      *QQSettings      `json:"qq,omitempty"`
	Weixin  *WeixinSettings  `json:"weixin,omitempty"`
	Gateway *GatewaySettings `json:"gateway,omitempty"`
}

// FullSettings 完整的设置结构体，用于验证和保存
type FullSettings struct {
	ActiveAlias          string           `json:"activeAlias"`
	Providers            []ProviderModel  `json:"providers"`
	Channel              *ChannelSettings `json:"channel,omitempty"`
	EnableHeartbeat      bool             `json:"enableHeartbeat"`
	HeartbeatPeriod      int              `json:"heartbeatPeriod"`
	HeartbeatTimeout     int              `json:"heartbeatTimeout"`
	MaxTurns             int              `json:"maxTurns"`
	MaxTokens            int              `json:"maxTokens"`
	MaxContextLength     int              `json:"maxContextLength"`
	MaxBudgetUSD         float64          `json:"maxBudgetUSD"`
	PermissionMode       string           `json:"permissionMode"`
	Verbose              bool             `json:"verbose"`
	Temperature          float64          `json:"temperature"`
	TopP                 float64          `json:"topP"`
	ThinkingBudget       int              `json:"thinkingBudget"`
	ShowToolUsageInReply bool             `json:"showToolUsageInReply"`
	ShowThinkingInLog    bool             `json:"showThinkingInLog"`
}

// Settings 简化版的设置结构体，用于读取端口配置
type Settings struct {
	Channel *ChannelSettings `json:"channel,omitempty"`
}

// getGatewayPort 从配置文件中读取 gateway 端口
func getGatewayPort() int {
	defaultPort := 10086

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Failed to get home directory: %v", err)
		return defaultPort
	}

	settingsPath := filepath.Join(homeDir, ".dogclaw", "setting.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		log.Printf("Failed to read settings file: %v", err)
		return defaultPort
	}

	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		log.Printf("Failed to parse settings file: %v", err)
		return defaultPort
	}

	if settings.Channel != nil && settings.Channel.Gateway != nil && settings.Channel.Gateway.Port > 0 {
		return settings.Channel.Gateway.Port
	}

	return defaultPort
}

func init() {
	// 设置日志输出到文件，方便调试
	homeDir, err := os.UserHomeDir()
	if err == nil {
		logDir := filepath.Join(homeDir, ".dogclaw")
		if err := os.MkdirAll(logDir, 0755); err == nil {
			logFile := filepath.Join(logDir, "ui.log")
			f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err == nil {
				log.SetOutput(f)
				log.SetFlags(log.LstdFlags | log.Lshortfile)
				log.Println("=== Logging initialized ===")
			}
		}
	}
}

var gatewayCmd *exec.Cmd

func main() {
	log.Println("DogClawUI starting...")

	// 设置信号处理
	setupSignalHandlers()

	systray.Run(onReady, onExit)
	log.Println("DogClawUI exiting...")
}

// setupSignalHandlers 设置信号处理器，确保强制关闭时也能清理 gateway
func setupSignalHandlers() {
	sigChan := make(chan os.Signal, 1)

	// 监听终止信号
	signal.Notify(sigChan,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGHUP,
		syscall.SIGQUIT,
	)

	go func() {
		sig := <-sigChan
		log.Printf("Received signal: %v, cleaning up...", sig)
		stopGateway()
		log.Println("Cleanup done, exiting")
		os.Exit(0)
	}()
}

func onReady() {
	log.Println("onReady called")
	systray.SetTitle("🐾")
	systray.SetTooltip("DogClaw")

	// 预先检查 dogclaw 可执行文件
	dogclawPath := getExecutablePath()
	if _, err := os.Stat(dogclawPath); err != nil {
		log.Printf("WARNING: dogclaw executable not found at %s: %v", dogclawPath, err)
	} else {
		log.Printf("dogclaw executable found at: %s", dogclawPath)
	}

	mStart := systray.AddMenuItem("启动 Gateway", "启动 gateway 模式")
	mStop := systray.AddMenuItem("关闭 Gateway", "关闭 gateway")
	mRestart := systray.AddMenuItem("重启 Gateway", "重启 gateway")
	systray.AddSeparator()
	mChat := systray.AddMenuItem("聊天", "在浏览器中打开聊天界面")
	mSettings := systray.AddMenuItem("设置", "编辑设置文件")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "退出应用")

	// 初始状态
	mStop.Disable()
	mRestart.Disable()

	log.Println("Menu items created, starting event loop")
	go func() {
		for {
			select {
			case <-mStart.ClickedCh:
				log.Println("Start Gateway clicked")
				go startGateway(mStart, mStop, mRestart)
			case <-mStop.ClickedCh:
				log.Println("Stop Gateway clicked")
				go stopGatewayWithMenu(mStart, mStop, mRestart)
			case <-mRestart.ClickedCh:
				log.Println("Restart Gateway clicked")
				go restartGateway(mStart, mStop, mRestart)
			case <-mChat.ClickedCh:
				log.Println("Chat clicked")
				openChat()
			case <-mSettings.ClickedCh:
				log.Println("Settings clicked")
				go editSettings()
			case <-mQuit.ClickedCh:
				log.Println("Quit clicked")
				systray.Quit()
			}
		}
	}()
}

func onExit() {
	log.Println("DogClawUI exiting, cleaning up gateway...")
	stopGateway()
	log.Println("Cleanup complete")
}

// stopGateway 强制停止 gateway 进程
func stopGateway() {
	if gatewayCmd == nil {
		log.Println("Gateway is not running")
		return
	}

	log.Println("Stopping gateway...")

	// 首先尝试使用进程组 ID 杀死整个进程组
	if gatewayCmd.Process != nil && gatewayCmd.SysProcAttr != nil && gatewayCmd.SysProcAttr.Setpgid {
		pgid, err := syscall.Getpgid(gatewayCmd.Process.Pid)
		if err == nil && pgid > 0 {
			log.Printf("Killing process group: -%d", pgid)
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
			// 给一点时间让进程退出
			time.Sleep(500 * time.Millisecond)
			// 如果还没退出，强制杀死
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
	}

	// 然后尝试直接杀死进程
	if gatewayCmd.Process != nil {
		log.Printf("Killing process: %d", gatewayCmd.Process.Pid)
		// 首先尝试优雅退出
		_ = gatewayCmd.Process.Signal(syscall.SIGTERM)
		// 等待一小段时间
		time.Sleep(300 * time.Millisecond)
		// 如果还在运行，强制杀死
		_ = gatewayCmd.Process.Kill()
	}

	// 等待进程完全退出
	if gatewayCmd.Process != nil {
		_, _ = gatewayCmd.Process.Wait()
	}

	gatewayCmd = nil
	log.Println("Gateway stopped")
}

func getExecutablePath() string {
	log.Println("Looking for dogclaw executable...")

	// 尝试找到 dogclaw 可执行文件
	// 1. 首先检查与 DogClawUI 同一目录下的 dogclaw（这是 macOS app 包中的标准位置）
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		dogclawPath := filepath.Join(exeDir, "dogclaw")
		log.Printf("Checking path 1: %s", dogclawPath)
		if _, err := os.Stat(dogclawPath); err == nil {
			log.Printf("Found dogclaw at: %s", dogclawPath)
			return dogclawPath
		}
	}

	// 2. 检查 PATH
	if path, err := exec.LookPath("dogclaw"); err == nil {
		log.Printf("Found dogclaw in PATH: %s", path)
		return path
	}

	// 3. 尝试从应用包结构中查找
	if exe, err := os.Executable(); err == nil {
		// 向上查找，尝试找到 .app 包的根目录
		currentDir := filepath.Dir(exe)
		for i := 0; i < 5; i++ {
			dogclawPath := filepath.Join(currentDir, "dogclaw")
			log.Printf("Checking path 3-%d: %s", i, dogclawPath)
			if _, err := os.Stat(dogclawPath); err == nil {
				log.Printf("Found dogclaw at: %s", dogclawPath)
				return dogclawPath
			}
			parentDir := filepath.Dir(currentDir)
			if parentDir == currentDir {
				break
			}
			currentDir = parentDir
		}
	}

	// 4. 尝试构建路径
	wd, _ := os.Getwd()
	path := filepath.Join(wd, "dogclaw")
	log.Printf("Returning fallback path: %s", path)
	return path
}

func startGateway(mStart, mStop, mRestart *systray.MenuItem) {
	if gatewayCmd != nil {
		log.Println("Gateway already running")
		return
	}

	dogclawPath := getExecutablePath()
	log.Printf("Starting gateway with: %s gateway", dogclawPath)

	gatewayCmd = exec.Command(dogclawPath, "gateway")
	gatewayCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// 重定向输出
	stdout, _ := gatewayCmd.StdoutPipe()
	stderr, _ := gatewayCmd.StderrPipe()

	if err := gatewayCmd.Start(); err != nil {
		log.Printf("Failed to start gateway: %v", err)
		gatewayCmd = nil
		return
	}

	log.Printf("Gateway started with PID: %d", gatewayCmd.Process.Pid)

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
	mRestart.Enable()

	// 监控进程
	go func() {
		err := gatewayCmd.Wait()
		log.Printf("Gateway exited with error: %v", err)
		gatewayCmd = nil
		mStart.Enable()
		mStop.Disable()
		mRestart.Disable()
	}()
}

func stopGatewayWithMenu(mStart, mStop, mRestart *systray.MenuItem) {
	stopGateway()
	mStart.Enable()
	mStop.Disable()
	mRestart.Disable()
}

func restartGateway(mStart, mStop, mRestart *systray.MenuItem) {
	if gatewayCmd == nil || gatewayCmd.Process == nil {
		log.Println("Gateway not running, cannot restart")
		return
	}

	log.Println("Restarting gateway...")

	// 先停止
	stopGatewayWithMenu(mStart, mStop, mRestart)

	// 等待一小会儿
	time.Sleep(500 * time.Millisecond)

	// 再启动
	startGateway(mStart, mStop, mRestart)
}

func openChat() {
	port := getGatewayPort()
	url := fmt.Sprintf("http://localhost:%d", port)
	log.Printf("Opening chat at: %s", url)
	_ = openBrowser(url)
}

// getSettingsPath 获取设置文件路径
func getSettingsPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".dogclaw", "setting.json"), nil
}

// readAndFormatSettings 读取并格式化设置文件
func readAndFormatSettings() (string, error) {
	path, err := getSettingsPath()
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	// 先解析再格式化，确保是正确的 JSON 格式
	var settings FullSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return "", fmt.Errorf("invalid JSON format: %w", err)
	}

	// 重新格式化，带缩进
	formatted, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", err
	}

	return string(formatted), nil
}

// validateSettings 验证设置内容是否有效
func validateSettings(content string) (*FullSettings, error) {
	var settings FullSettings
	if err := json.Unmarshal([]byte(content), &settings); err != nil {
		return nil, fmt.Errorf("JSON 格式错误: %w", err)
	}

	// 验证必要的字段
	if settings.ActiveAlias == "" {
		return nil, fmt.Errorf("activeAlias 不能为空")
	}

	if len(settings.Providers) == 0 {
		return nil, fmt.Errorf("providers 列表不能为空")
	}

	// 验证 activeAlias 是否存在于 providers 中
	found := false
	for _, p := range settings.Providers {
		if p.Alias == settings.ActiveAlias {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("activeAlias %q 不在 providers 列表中", settings.ActiveAlias)
	}

	return &settings, nil
}

// editSettings 编辑设置文件
func editSettings() {
	path, err := getSettingsPath()
	if err != nil {
		log.Printf("Failed to get settings path: %v", err)
		showAlert("错误", "无法获取设置文件路径")
		return
	}

	// 读取并格式化当前设置
	content, err := readAndFormatSettings()
	if err != nil {
		log.Printf("Failed to read settings: %v", err)
		showAlert("错误", "无法读取设置文件")
		return
	}

	// 使用自定义 GUI 窗口编辑
	for {
		newContent, cancelled := showSettingsEditor(content)
		if cancelled {
			log.Println("Settings edit cancelled")
			return
		}

		// 验证内容
		settings, err := validateSettings(newContent)
		if err != nil {
			log.Printf("Invalid settings: %v", err)
			showAlert("设置验证失败", err.Error()+"\n\n请重新编辑")
			content = newContent // 保留用户编辑的内容
			continue
		}

		// 格式化并保存
		formatted, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			log.Printf("Failed to format settings: %v", err)
			showAlert("错误", "无法格式化设置")
			return
		}

		if err := os.WriteFile(path, formatted, 0644); err != nil {
			log.Printf("Failed to save settings: %v", err)
			showAlert("错误", "无法保存设置文件")
			return
		}

		log.Println("Settings saved successfully")
		showAlert("成功", "设置已保存")
		return
	}
}

// showSettingsEditor 显示 Web 设置编辑页面
func showSettingsEditor(initialContent string) (string, bool) {
	// 查找一个可用的端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Printf("Failed to find available port: %v", err)
		showAlert("错误", "无法启动临时服务器")
		return initialContent, true
	}
	port := listener.Addr().(*net.TCPAddr).Port
	defer listener.Close()

	// 创建用于通信的通道
	resultCh := make(chan string, 1)
	cancelCh := make(chan bool, 1)

	// 设置 HTTP 路由
	mux := http.NewServeMux()

	// 主页：显示编辑器
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>DogClaw 设置编辑器</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            min-height: 100vh;
            display: flex;
            justify-content: center;
            align-items: center;
            padding: 20px;
        }
        .container {
            background: white;
            border-radius: 12px;
            box-shadow: 0 20px 60px rgba(0,0,0,0.3);
            width: 100%%;
            max-width: 900px;
            overflow: hidden;
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            color: white;
            padding: 24px 30px;
        }
        .header h1 {
            font-size: 28px;
            font-weight: 600;
            margin-bottom: 8px;
        }
        .header p {
            font-size: 14px;
            opacity: 0.9;
        }
        .content {
            padding: 30px;
        }
        .editor-wrapper {
            margin-bottom: 20px;
        }
        textarea {
            width: 100%%;
            height: 450px;
            font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
            font-size: 14px;
            line-height: 1.6;
            padding: 16px;
            border: 2px solid #e0e0e0;
            border-radius: 8px;
            resize: vertical;
            transition: border-color 0.3s;
        }
        textarea:focus {
            outline: none;
            border-color: #667eea;
        }
        .buttons {
            display: flex;
            gap: 12px;
            justify-content: flex-end;
        }
        button {
            padding: 12px 28px;
            font-size: 15px;
            font-weight: 500;
            border: none;
            border-radius: 8px;
            cursor: pointer;
            transition: all 0.3s;
        }
        .btn-cancel {
            background: #f0f0f0;
            color: #333;
        }
        .btn-cancel:hover {
            background: #e0e0e0;
        }
        .btn-format {
            background: #f0f0f0;
            color: #333;
        }
        .btn-format:hover {
            background: #e0e0e0;
        }
        .btn-validate {
            background: #2196F3;
            color: white;
        }
        .btn-validate:hover {
            background: #1976D2;
        }
        .btn-save {
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            color: white;
        }
        .btn-save:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 12px rgba(102, 126, 234, 0.4);
        }
        .message {
            margin-bottom: 16px;
            padding: 12px 16px;
            border-radius: 8px;
            font-size: 14px;
            display: none;
        }
        .message.success {
            background: #e8f5e9;
            color: #2e7d32;
            border: 1px solid #a5d6a7;
        }
        .message.error {
            background: #ffebee;
            color: #c62828;
            border: 1px solid #ef9a9a;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🐾 DogClaw 设置编辑器</h1>
            <p>编辑您的 DogClaw 配置文件</p>
        </div>
        <div class="content">
            <div id="message" class="message"></div>
            <div class="editor-wrapper">
                <textarea id="jsonEditor" spellcheck="false">%s</textarea>
            </div>
            <div class="buttons">
                <button class="btn-cancel" onclick="cancel()">取消</button>
                <button class="btn-format" onclick="formatJSON()">格式化</button>
                <button class="btn-validate" onclick="validateJSON()">验证</button>
                <button class="btn-save" onclick="saveSettings()">保存</button>
            </div>
        </div>
    </div>
    <script>
        function showMessage(type, text) {
            const msg = document.getElementById('message');
            msg.className = 'message ' + type;
            msg.textContent = text;
            msg.style.display = 'block';
            setTimeout(() => {
                msg.style.display = 'none';
            }, 3000);
        }

        function formatJSON() {
            try {
                const editor = document.getElementById('jsonEditor');
                const data = JSON.parse(editor.value);
                editor.value = JSON.stringify(data, null, 2);
                showMessage('success', 'JSON 格式化成功！');
            } catch (e) {
                showMessage('error', 'JSON 格式错误：' + e.message);
            }
        }

        function validateJSON() {
            try {
                const editor = document.getElementById('jsonEditor');
                JSON.parse(editor.value);
                showMessage('success', 'JSON 格式正确！');
            } catch (e) {
                showMessage('error', 'JSON 格式错误：' + e.message);
            }
        }

        async function saveSettings() {
            try {
                const editor = document.getElementById('jsonEditor');
                const content = editor.value;
                JSON.parse(content);
                
                const response = await fetch('/save', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: content
                });
                
                if (response.ok) {
                    showMessage('success', '保存成功！窗口将关闭...');
                    setTimeout(() => {
                        window.close();
                    }, 1500);
                } else {
                    const error = await response.text();
                    showMessage('error', '保存失败：' + error);
                }
            } catch (e) {
                showMessage('error', '保存失败：' + e.message);
            }
        }

        function cancel() {
            fetch('/cancel', { method: 'POST' });
            window.close();
        }
    </script>
</body>
</html>
`, escapeHTML(initialContent))
	})

	// 保存接口
	mux.HandleFunc("/save", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		content := string(body)

		// 验证 JSON 格式
		var test interface{}
		if err := json.Unmarshal([]byte(content), &test); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		// 发送结果
		resultCh <- content
		w.WriteHeader(http.StatusOK)
	})

	// 取消接口
	mux.HandleFunc("/cancel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		cancelCh <- true
		w.WriteHeader(http.StatusOK)
	})

	// 启动服务器
	server := &http.Server{
		Handler: mux,
	}

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}()

	// 在浏览器中打开页面
	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	log.Printf("Opening settings editor at: %s", url)
	_ = openBrowser(url)

	// 显示提示
	showAlert("提示", fmt.Sprintf("设置编辑器已在浏览器中打开：\n\n%s\n\n完成编辑后请在浏览器中点击保存。", url))

	// 等待结果
	select {
	case result := <-resultCh:
		log.Println("Settings saved via web")
		_ = server.Close()
		return result, false
	case <-cancelCh:
		log.Println("Settings edit cancelled via web")
		_ = server.Close()
		return initialContent, true
	case <-time.After(10 * time.Minute):
		log.Println("Settings edit timeout")
		_ = server.Close()
		return initialContent, true
	}
}

// escapeHTML 转义 HTML 字符串
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// showAlert 显示 macOS 提示框
func showAlert(title, message string) {
	script := fmt.Sprintf(`tell app "System Events" to display dialog "%s" buttons {"OK"} default button 1 with title "%s"`,
		escapeAppleScriptString(message), escapeAppleScriptString(title))
	cmd := exec.Command("osascript", "-e", script)
	_ = cmd.Run()
}

// escapeAppleScriptString 转义 AppleScript 字符串
func escapeAppleScriptString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
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
		cmd = exec.Command("open", "-t", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", path)
	default:
		return nil
	}
	return cmd.Start()
}
