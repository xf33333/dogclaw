package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultSettings(t *testing.T) {
	s := DefaultSettings()
	if s.ActiveAlias != "default" {
		t.Errorf("expected default active alias, got %q", s.ActiveAlias)
	}
	if len(s.Providers) == 0 {
		t.Error("expected at least one default provider")
	}
	if s.MaxTurns != 1000 {
		t.Errorf("expected default MaxTurns=1000, got %d", s.MaxTurns)
	}
}

func TestSaveAndLoadSettings(t *testing.T) {
	// Create a temp directory to simulate ~/.docclaw
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	settings := DefaultSettings()
	settings.ActiveAlias = "test-alias"
	settings.Providers = []ProviderModel{
		{
			Alias:    "test-alias",
			Provider: "anthropic",
			Model:    "claude-sonnet-4-20250514",
			URL:      "https://api.anthropic.com",
		},
		{
			Alias:    "test-alias-2",
			Provider: "openai",
			Model:    "gpt-4o",
			URL:      "https://api.openai.com/v1",
		},
	}
	settings.MaxTurns = 500
	settings.Verbose = true

	// Save
	if err := settings.SaveSettings(); err != nil {
		t.Fatalf("failed to save settings: %v", err)
	}

	// Verify file exists
	path := filepath.Join(tmpDir, configDirName, settingsFileName)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("settings file was not created")
	}

	// Load
	loaded, err := LoadSettings()
	if err != nil {
		t.Fatalf("failed to load settings: %v", err)
	}

	if loaded.ActiveAlias != "test-alias" {
		t.Errorf("expected ActiveAlias=%q, got %q", "test-alias", loaded.ActiveAlias)
	}
	if loaded.MaxTurns != 500 {
		t.Errorf("expected MaxTurns=500, got %d", loaded.MaxTurns)
	}
	if len(loaded.Providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(loaded.Providers))
	}

	// Verify JSON is valid and readable
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read raw settings file: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("settings file is not valid JSON: %v", err)
	}
}

func TestLoadSettingsNonExistent(t *testing.T) {
	// Use a non-existent path
	t.Setenv("HOME", t.TempDir())

	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("expected no error for non-existent file, got: %v", err)
	}
	if s.ActiveAlias != "default" {
		t.Errorf("expected default ActiveAlias, got %q", s.ActiveAlias)
	}
}

func TestGetActive(t *testing.T) {
	s := DefaultSettings()
	s.ActiveAlias = "default"

	active, err := s.GetActive()
	if err != nil {
		t.Fatalf("failed to get active: %v", err)
	}
	if active.Alias != "default" {
		t.Errorf("expected alias=default, got %q", active.Alias)
	}

	// Test non-existent active alias
	s.ActiveAlias = "nonexistent"
	_, err = s.GetActive()
	if err == nil {
		t.Error("expected error for non-existent active alias")
	}
}

func TestGetByAlias(t *testing.T) {
	s := DefaultSettings()
	s.Providers = append(s.Providers, ProviderModel{
		Alias:    "secondary",
		Provider: "openai",
		Model:    "gpt-4o",
		URL:      "https://api.openai.com/v1",
	})

	pm, err := s.GetByAlias("secondary")
	if err != nil {
		t.Fatalf("failed to get by alias: %v", err)
	}
	if pm.Model != "gpt-4o" {
		t.Errorf("expected model=gpt-4o, got %q", pm.Model)
	}

	// Test non-existent alias
	_, err = s.GetByAlias("does-not-exist")
	if err == nil {
		t.Error("expected error for non-existent alias")
	}
}

func TestAddOrUpdateProvider(t *testing.T) {
	s := DefaultSettings()

	// Add new
	s.AddOrUpdateProvider(ProviderModel{
		Alias:    "new-one",
		Provider: "anthropic",
		Model:    "claude-opus-4",
		URL:      "https://api.anthropic.com",
	})

	if len(s.Providers) != 2 {
		t.Errorf("expected 2 providers after add, got %d", len(s.Providers))
	}

	// Update existing
	s.AddOrUpdateProvider(ProviderModel{
		Alias:    "new-one",
		Provider: "anthropic",
		Model:    "claude-opus-4-updated",
		URL:      "https://api.anthropic.com",
	})

	if len(s.Providers) != 2 {
		t.Errorf("expected still 2 providers after update, got %d", len(s.Providers))
	}

	pm, _ := s.GetByAlias("new-one")
	if pm.Model != "claude-opus-4-updated" {
		t.Errorf("expected updated model, got %q", pm.Model)
	}
}

func TestRemoveProvider(t *testing.T) {
	s := DefaultSettings()
	s.Providers = append(s.Providers, ProviderModel{
		Alias:    "to-remove",
		Provider: "openai",
		Model:    "gpt-4o",
		URL:      "https://api.openai.com/v1",
	})

	// Remove existing
	if !s.RemoveProvider("to-remove") {
		t.Error("expected RemoveProvider to return true")
	}

	// Remove non-existent
	if s.RemoveProvider("does-not-exist") {
		t.Error("expected RemoveProvider to return false for non-existent alias")
	}
}

func TestRemoveActiveProvider(t *testing.T) {
	s := DefaultSettings()
	s.Providers = append(s.Providers, ProviderModel{
		Alias:    "another",
		Provider: "openai",
		Model:    "gpt-4o",
		URL:      "https://api.openai.com/v1",
	})

	// The active alias is "default", remove it
	s.RemoveProvider("default")

	// ActiveAlias should have been reset to first available
	if s.ActiveAlias != "another" {
		t.Errorf("expected ActiveAlias to reset to 'another', got %q", s.ActiveAlias)
	}
}

func TestConfigFromSettings(t *testing.T) {
	s := DefaultSettings()
	s.ActiveAlias = "default"

	cfg, err := ConfigFromSettings(s)
	if err != nil {
		t.Fatalf("ConfigFromSettings failed: %v", err)
	}
	if cfg.Model != "qwen/qwen3.6-plus:free" {
		t.Errorf("expected Model=qwen/qwen3.6-plus:free, got %q", cfg.Model)
	}
	if cfg.MaxTurns != 1000 {
		t.Errorf("expected MaxTurns=1000, got %d", cfg.MaxTurns)
	}
}

func TestExampleJSONIsValid(t *testing.T) {
	data, err := os.ReadFile("testdata/setting.example.json")
	if err != nil {
		t.Fatalf("failed to read example file: %v", err)
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("example JSON is not valid: %v", err)
	}

	if s.ActiveAlias != "claude-sonnet" {
		t.Errorf("expected ActiveAlias=claude-sonnet, got %q", s.ActiveAlias)
	}
	if len(s.Providers) != 4 {
		t.Errorf("expected 4 providers in example, got %d", len(s.Providers))
	}

	// Verify the active provider can be resolved
	active, err := s.GetActive()
	if err != nil {
		t.Fatalf("failed to get active provider from example: %v", err)
	}
	if active.Model != "openrouter/anthropic/claude-sonnet-4.5" {
		t.Errorf("expected claude-sonnet model, got %q", active.Model)
	}
}
