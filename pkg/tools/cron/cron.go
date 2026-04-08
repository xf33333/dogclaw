package cron

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// CronJob represents a single scheduled task
type CronJob struct {
	Schedule    string `json:"schedule"`    // Cron expression (e.g., "*/5 * * * *")
	Description string `json:"description"` // Task description in natural language
}

// CronConfig matches the structure of ~/.dogclaw/cron.json
type CronConfig struct {
	Tasks []CronJob `json:"tasks"`
}

const configFileName = "cron.json"

// GetSettingsDir returns the path to the config directory ~/.dogclaw
// Redefined here to avoid circular dependency if internal/config imports this package later
func GetSettingsDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".dogclaw"), nil
}

// GetConfigPath returns the full path to cron.json
func GetConfigPath() (string, error) {
	dir, err := GetSettingsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

// LoadConfig loads the cron configuration from ~/.dogclaw/cron.json
func LoadConfig() (*CronConfig, error) {
	path, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 配置文件不存在，返回空配置
			return &CronConfig{Tasks: []CronJob{}}, nil
		}
		return nil, fmt.Errorf("failed to read cron config: %w", err)
	}

	// 处理空文件
	if len(bytes.TrimSpace(data)) == 0 {
		return &CronConfig{Tasks: []CronJob{}}, nil
	}

	var config CronConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse cron config: %w", err)
	}

	// 确保 Tasks 不为 nil
	if config.Tasks == nil {
		config.Tasks = []CronJob{}
	}

	return &config, nil
}

// SaveConfig saves the cron configuration to ~/.dogclaw/cron.json
func SaveConfig(config *CronConfig) error {
	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	dir, err := GetSettingsDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	path, err := GetConfigPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cron config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write cron config: %w", err)
	}

	return nil
}
