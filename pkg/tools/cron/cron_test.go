package cron

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCronConfig(t *testing.T) {
	config := &CronConfig{
		Tasks: []CronJob{
			{Schedule: "*/1 * * * *", Description: "Test Task"},
		},
	}

	err := SaveConfig(config)
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if len(loaded.Tasks) != 1 || loaded.Tasks[0].Description != "Test Task" {
		t.Errorf("Loaded config mismatch: %+v", loaded)
	}
}

func TestLoadConfig_FileNotExist(t *testing.T) {
	// 先清理配置文件，确保测试环境干净
	dir, err := GetSettingsDir()
	if err != nil {
		t.Fatalf("Failed to get settings dir: %v", err)
	}
	configPath := filepath.Join(dir, configFileName)
	_ = os.Remove(configPath)

	// 测试配置文件不存在的情况
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() should not return error when file not exist: %v", err)
	}
	if config == nil {
		t.Fatal("config should not be nil")
	}
	if config.Tasks == nil {
		t.Error("config.Tasks should not be nil")
	}
	if len(config.Tasks) != 0 {
		t.Errorf("Expected empty tasks, got %d", len(config.Tasks))
	}
}

func TestLoadConfig_EmptyFile(t *testing.T) {
	// 测试空文件
	dir, err := GetSettingsDir()
	if err != nil {
		t.Fatalf("Failed to get settings dir: %v", err)
	}

	configPath := filepath.Join(dir, configFileName)
	// 确保目录存在
	os.MkdirAll(dir, 0755)
	// 创建空文件
	if err := os.WriteFile(configPath, []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create empty file: %v", err)
	}
	defer func() {
		_ = os.Remove(configPath)
	}()

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() should not return error for empty file: %v", err)
	}
	if config == nil {
		t.Fatal("config should not be nil")
	}
	if config.Tasks == nil {
		t.Error("config.Tasks should not be nil")
	}
	if len(config.Tasks) != 0 {
		t.Errorf("Expected empty tasks, got %d", len(config.Tasks))
	}
}

func TestLoadConfig_WhitespaceOnlyFile(t *testing.T) {
	// 测试仅包含空白的文件
	dir, err := GetSettingsDir()
	if err != nil {
		t.Fatalf("Failed to get settings dir: %v", err)
	}

	configPath := filepath.Join(dir, configFileName)
	os.MkdirAll(dir, 0755)
	// 创建仅包含空白的文件
	if err := os.WriteFile(configPath, []byte("   \n  \t  "), 0644); err != nil {
		t.Fatalf("Failed to create whitespace file: %v", err)
	}
	defer func() {
		_ = os.Remove(configPath)
	}()

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() should not return error for whitespace file: %v", err)
	}
	if config == nil {
		t.Fatal("config should not be nil")
	}
	if config.Tasks == nil {
		t.Error("config.Tasks should not be nil")
	}
	if len(config.Tasks) != 0 {
		t.Errorf("Expected empty tasks, got %d", len(config.Tasks))
	}
}

func TestLoadConfig_NullTasks(t *testing.T) {
	// 测试 tasks 为 null 的情况
	dir, err := GetSettingsDir()
	if err != nil {
		t.Fatalf("Failed to get settings dir: %v", err)
	}

	configPath := filepath.Join(dir, configFileName)
	os.MkdirAll(dir, 0755)

	// 创建 tasks 为 null 的配置文件
	nullConfig := `{"tasks": null}`
	if err := os.WriteFile(configPath, []byte(nullConfig), 0644); err != nil {
		t.Fatalf("Failed to create config with null tasks: %v", err)
	}
	defer func() {
		_ = os.Remove(configPath)
	}()

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() should not return error for null tasks: %v", err)
	}
	if config == nil {
		t.Fatal("config should not be nil")
	}
	if config.Tasks == nil {
		t.Error("config.Tasks should not be nil after LoadConfig")
	}
	if len(config.Tasks) != 0 {
		t.Errorf("Expected empty tasks after handling null, got %d", len(config.Tasks))
	}
}

func TestLoadConfig_EmptyTasksArray(t *testing.T) {
	// 测试 tasks 为空数组的情况
	dir, err := GetSettingsDir()
	if err != nil {
		t.Fatalf("Failed to get settings dir: %v", err)
	}

	configPath := filepath.Join(dir, configFileName)
	os.MkdirAll(dir, 0755)

	// 创建 tasks 为空数组的配置文件
	emptyConfig := `{"tasks": []}`
	if err := os.WriteFile(configPath, []byte(emptyConfig), 0644); err != nil {
		t.Fatalf("Failed to create config with empty tasks: %v", err)
	}
	defer func() {
		_ = os.Remove(configPath)
	}()

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() should not return error for empty tasks array: %v", err)
	}
	if config == nil {
		t.Fatal("config should not be nil")
	}
	if config.Tasks == nil {
		t.Error("config.Tasks should not be nil")
	}
	if len(config.Tasks) != 0 {
		t.Errorf("Expected empty tasks, got %d", len(config.Tasks))
	}
}

func TestSaveConfig_NilConfig(t *testing.T) {
	// 测试保存 nil 配置
	err := SaveConfig(nil)
	if err == nil {
		t.Error("SaveConfig(nil) should return an error")
	}
}

func TestSaveConfig_EmptyConfig(t *testing.T) {
	// 测试保存空配置
	config := &CronConfig{
		Tasks: []CronJob{},
	}

	err := SaveConfig(config)
	if err != nil {
		t.Fatalf("Failed to save empty config: %v", err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if len(loaded.Tasks) != 0 {
		t.Errorf("Expected empty tasks, got %d", len(loaded.Tasks))
	}
}
