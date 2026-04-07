package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	configDirName    = ".dogclaw"
	settingsFileName = "setting.json"
)

// ProviderModel represents a configured model with its provider, URL and alias
type ProviderModel struct {
	Alias    string `json:"alias"`    // 别名，用于快速引用
	Provider string `json:"provider"` // 提供商名称，如 anthropic, openrouter, openai
	Model    string `json:"model"`    // 模型名称，如 claude-sonnet-4, gpt-4
	URL      string `json:"url"`      // API 地址
}

// ChannelSettings holds configuration for different channels
type ChannelSettings struct {
	// QQ holds the QQ channel configuration
	QQ *QQSettings `json:"qq,omitempty"`

	// Weixin holds the Weixin channel configuration
	Weixin *WeixinSettings `json:"weixin,omitempty"`
}

// Settings holds user's persistent configuration stored in ~/.docclaw/setting.json
type Settings struct {
	// ActiveAlias is the alias of the currently active model
	ActiveAlias string `json:"activeAlias"`

	// Providers is the list of configured provider models
	Providers []ProviderModel `json:"providers"`

	// Channel holds channel-specific configurations
	Channel *ChannelSettings `json:"channel,omitempty"`

	// Other parameters
	MaxTurns             int     `json:"maxTurns"`
	MaxTokens            int     `json:"maxTokens"` // 单次响应最大 token 数
	MaxBudgetUSD         float64 `json:"maxBudgetUSD"`
	PermissionMode       string  `json:"permissionMode"`
	Verbose              bool    `json:"verbose"`
	Temperature          float64 `json:"temperature"`
	TopP                 float64 `json:"topP"`
	ThinkingBudget       int     `json:"thinkingBudget"`       // 思考模式 token 预算，0 表示关闭
	ShowToolUsageInReply bool    `json:"showToolUsageInReply"` // 是否在会话中回复tool使用说明
	ShowThinkingInLog    bool    `json:"showThinkingInLog"`    // 是否在日志中输出LLM的思考内容
}

// GetSettingsDir returns the path to the config directory ~/.docclaw
func GetSettingsDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, configDirName), nil
}

// GetSettingsPath returns the full path to the settings file ~/.docclaw/setting.json
func GetSettingsPath() (string, error) {
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
			},
		},
		MaxTurns:             1000,
		MaxTokens:            8192,
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

// LoadSettings loads settings from ~/.docclaw/setting.json.
// If the file does not exist, it creates the directory and returns default settings.
func LoadSettings() (*Settings, error) {
	path, err := GetSettingsPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, return defaults
			return DefaultSettings(), nil
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

	return &settings, nil
}

// SaveSettings persists the settings to ~/.docclaw/setting.json
func (s *Settings) SaveSettings() error {
	dir, err := GetSettingsDir()
	if err != nil {
		return err
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	path := filepath.Join(dir, settingsFileName)
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

// GetByAlias returns the ProviderModel matching the given alias.
func (s *Settings) GetByAlias(alias string) (*ProviderModel, error) {
	for _, pm := range s.Providers {
		if pm.Alias == alias {
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
