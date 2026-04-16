package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dogclaw/pkg/compact"
)

const (
	configDirName    = ".dogclaw"
	settingsFileName = "setting.json"
)

// customConfigPath holds the path to a custom config file, if specified
var customConfigPath string

// workingDir holds the working directory for config lookup, if specified
var workingDir string

// SetConfigPath sets a custom path for the configuration file
func SetConfigPath(path string) {
	customConfigPath = path
}

// SetWorkingDir sets the working directory for config lookup
func SetWorkingDir(dir string) {
	workingDir = dir
}

// GetConfigPath returns the current configuration file path
func GetConfigPath() string {
	if customConfigPath != "" {
		return customConfigPath
	}
	path, _ := GetSettingsPath()
	return path
}

// ProviderModel represents a configured model with its provider, URL and alias
type ProviderModel struct {
	Alias    string `json:"alias"`    // 别名，用于快速引用
	Provider string `json:"provider"` // 提供商名称，如 anthropic, openrouter, openai
	Model    string `json:"model"`    // 模型名称，如 claude-sonnet-4, gpt-4
	URL      string `json:"url"`      // API 地址
	APIKey   string `json:"apiKey"`   // API 密钥
}

// ChannelSettings holds configuration for different channels
type ChannelSettings struct {
	// QQ holds the QQ channel configuration
	QQ *QQSettings `json:"qq,omitempty"`

	// Weixin holds the Weixin channel configuration
	Weixin *WeixinSettings `json:"weixin,omitempty"`

	// Gateway holds the gateway HTTP server configuration
	Gateway *GatewaySettings `json:"gateway,omitempty"`
}

// GatewaySettings holds configuration for the gateway HTTP server
type GatewaySettings struct {
	Enabled bool `json:"enabled"` // 是否启用 gateway HTTP 服务器
	Port    int  `json:"port"`    // HTTP 服务器监听端口，默认 10086
}

// Settings holds user's persistent configuration stored in ~/.docclaw/setting.json
type Settings struct {
	// ActiveAlias is the alias of the currently active model
	ActiveAlias string `json:"activeAlias"`

	// Providers is the list of configured provider models
	Providers []ProviderModel `json:"providers"`

	// Channel holds channel-specific configurations
	Channel *ChannelSettings `json:"channel,omitempty"`

	// MCP holds MCP (Model Context Protocol) server configurations
	MCP *MCPSettings `json:"mcp,omitempty"`

	// AutoCompact configuration (LLM-assisted context compression)
	AutoCompact *AutoCompactSettings `json:"autoCompact,omitempty"`

	// Heartbeat configuration
	EnableHeartbeat   bool `json:"enableHeartbeat"`   // 是否启用心跳
	HeartbeatInterval int  `json:"heartbeatInterval"` // 心跳间隔(秒)，默认300秒(5分钟)

	// Other parameters
	MaxTurns             int     `json:"maxTurns"`
	MaxTokens            int     `json:"maxTokens"`        // 单次响应最大 token 数
	MaxContextLength     int     `json:"maxContextLength"` // 最大上下文长度（对话历史总 token 数）
	MaxBudgetUSD         float64 `json:"maxBudgetUSD"`
	PermissionMode       string  `json:"permissionMode"`
	Verbose              bool    `json:"verbose"`
	Temperature          float64 `json:"temperature"`
	TopP                 float64 `json:"topP"`
	ThinkingBudget       int     `json:"thinkingBudget"`       // 思考模式 token 预算，0 表示关闭
	ShowToolUsageInReply bool    `json:"showToolUsageInReply"` // 是否在会话中回复tool使用说明
	ShowThinkingInLog    bool    `json:"showThinkingInLog"`    // 是否在日志中输出LLM的思考内容
}

// AutoCompactSettings holds configuration for LLM-assisted context compression
type AutoCompactSettings struct {
	Enabled          bool    `json:"enabled"`          // 是否启用自动压缩
	ThresholdRatio   float64 `json:"thresholdRatio"`   // 触发压缩的上下文比例（默认 0.75）
	WarningRatio     float64 `json:"warningRatio"`     // 显示警告的上下文比例（默认 0.65）
	MaxContextTokens int     `json:"maxContextTokens"` // 阻塞前的最大上下文 token 数
}

// MCPSettings holds MCP (Model Context Protocol) configuration
type MCPSettings struct {
	// Enabled controls whether MCP integration is enabled
	Enabled bool `json:"enabled"`

	// ConfigPath is the path to the MCP servers configuration file
	ConfigPath string `json:"configPath,omitempty"`
}

// GetSettingsDir returns the path to the config directory ~/.docclaw
func GetSettingsDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, configDirName), nil
}

// GetSettingsPath returns the full path to the settings file.
// Priority order:
// 1. customConfigPath (if set via SetConfigPath)
// 2. workingDir/.dogclaw/setting.json (if workingDir is set and file exists)
// 3. ~/.dogclaw/setting.json (default)
func GetSettingsPath() (string, error) {
	if customConfigPath != "" {
		return customConfigPath, nil
	}

	// Check working directory first if set
	if workingDir != "" {
		workDirConfig := filepath.Join(workingDir, configDirName, settingsFileName)
		if _, err := os.Stat(workDirConfig); err == nil {
			return workDirConfig, nil
		}
	}

	// Fallback to home directory
	dir, err := GetSettingsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, settingsFileName), nil
}

// DefaultSettings returns a Settings struct with sensible defaults
func DefaultSettings() *Settings {
	return &Settings{
		ActiveAlias: "default",
		Providers: []ProviderModel{
			{
				Alias:    "default",
				Provider: "openrouter",
				Model:    "qwen/qwen3.6-plus:free",
				URL:      "https://",
				APIKey:   "",
			},
		},
		Channel: &ChannelSettings{
			Gateway: &GatewaySettings{
				Enabled: true,
				Port:    10086,
			},
		},
		MCP: &MCPSettings{
			Enabled: false, // MCP is disabled by default
		},
		AutoCompact: &AutoCompactSettings{
			Enabled:          true,
			ThresholdRatio:   0.75, // 75% of context window
			WarningRatio:     0.65, // 65% warning
			MaxContextTokens: 190000,
		},
		MaxTurns:             1000,
		MaxTokens:            8192,
		MaxContextLength:     200000, // 默认最大上下文长度 200K tokens
		MaxBudgetUSD:         0,
		PermissionMode:       "default",
		Verbose:              false,
		Temperature:          0,
		TopP:                 0,
		ThinkingBudget:       0,
		ShowToolUsageInReply: false,
		ShowThinkingInLog:    true,
	}
}

// LoadSettings loads settings from config file.
// If SetConfigPath was called, it loads from that path.
// Otherwise, it loads from ~/.dogclaw/setting.json.
// If the file does not exist, it creates the directory, saves default settings, and returns them.
func LoadSettings() (*Settings, error) {
	var path string
	var err error

	if customConfigPath != "" {
		path = customConfigPath
	} else {
		path, err = GetSettingsPath()
		if err != nil {
			return nil, err
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, create default settings and save to file
			defaultSettings := DefaultSettings()
			if err := defaultSettings.SaveSettings(); err != nil {
				return nil, fmt.Errorf("failed to save default settings: %w", err)
			}
			return defaultSettings, nil
		}
		return nil, fmt.Errorf("failed to read settings file: %w", err)
	}

	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("failed to parse settings file: %w", err)
	}

	// Apply defaults for zero-value fields
	if settings.MaxTurns <= 0 {
		settings.MaxTurns = DefaultSettings().MaxTurns
	}
	if settings.MaxTokens <= 0 {
		settings.MaxTokens = DefaultSettings().MaxTokens
	}
	if settings.MaxContextLength <= 0 {
		settings.MaxContextLength = DefaultSettings().MaxContextLength
	}

	return &settings, nil
}

// SaveSettings persists the settings to the config file.
// Priority order for saving:
// 1. customConfigPath (if set via SetConfigPath)
// 2. workingDir/.dogclaw/setting.json (if workingDir is set)
// 3. ~/.dogclaw/setting.json (default)
func (s *Settings) SaveSettings() error {
	var dir string
	var path string
	var err error

	if customConfigPath != "" {
		path = customConfigPath
		dir = filepath.Dir(path)
	} else if workingDir != "" {
		// Save to working directory if set
		dir = filepath.Join(workingDir, configDirName)
		path = filepath.Join(dir, settingsFileName)
	} else {
		// Default to home directory
		dir, err = GetSettingsDir()
		if err != nil {
			return err
		}
		path = filepath.Join(dir, settingsFileName)
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write settings file: %w", err)
	}

	return nil
}

// GetActive returns the ProviderModel for the currently active alias.
// Returns an error if the active alias is not found in the providers list.
func (s *Settings) GetActive() (*ProviderModel, error) {
	for _, pm := range s.Providers {
		if pm.Alias == s.ActiveAlias {
			return &pm, nil
		}
	}
	return nil, fmt.Errorf("active alias %q not found in providers", s.ActiveAlias)
}

// GetByAlias returns the ProviderModel matching the given alias (case-insensitive).
func (s *Settings) GetByAlias(alias string) (*ProviderModel, error) {
	aliasLower := strings.ToLower(alias)
	for _, pm := range s.Providers {
		if strings.ToLower(pm.Alias) == aliasLower {
			return &pm, nil
		}
	}
	return nil, fmt.Errorf("alias %q not found", alias)
}

// AddOrUpdateProvider adds a new provider model or updates an existing one by alias
func (s *Settings) AddOrUpdateProvider(pm ProviderModel) {
	for i, existing := range s.Providers {
		if existing.Alias == pm.Alias {
			s.Providers[i] = pm
			return
		}
	}
	s.Providers = append(s.Providers, pm)
}

// RemoveProvider removes a provider model by alias
func (s *Settings) RemoveProvider(alias string) bool {
	for i, pm := range s.Providers {
		if pm.Alias == alias {
			s.Providers = append(s.Providers[:i], s.Providers[i+1:]...)
			// If we removed the active one, reset to first available
			if s.ActiveAlias == alias && len(s.Providers) > 0 {
				s.ActiveAlias = s.Providers[0].Alias
			}
			return true
		}
	}
	return false
}

// ToAutoCompactConfig converts AutoCompactSettings to compact.AutoCompactConfig
func (s *Settings) ToAutoCompactConfig() *compact.AutoCompactConfig {
	if s.AutoCompact == nil {
		return compact.DefaultAutoCompactConfig()
	}
	return &compact.AutoCompactConfig{
		Enabled:            s.AutoCompact.Enabled,
		ThresholdRatio:     s.AutoCompact.ThresholdRatio,
		WarningRatio:       s.AutoCompact.WarningRatio,
		MaxContextTokens:   s.AutoCompact.MaxContextTokens,
		ModelContextWindow: s.MaxContextLength,
	}
}
