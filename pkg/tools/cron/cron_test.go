package cron

import (
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
