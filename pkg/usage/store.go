package usage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// UsageRecord represents a single usage record
type UsageRecord struct {
	Timestamp          int64  `json:"timestamp"`
	Model              string `json:"model"`
	InputTokens        int    `json:"input_tokens"`
	OutputTokens       int    `json:"output_tokens"`
	CacheRead          int    `json:"cache_read,omitempty"`
	CacheCreation      int    `json:"cache_creation,omitempty"`
	ReasoningInput     int    `json:"reasoning_input,omitempty"`
	ReasoningOutput    int    `json:"reasoning_output,omitempty"`
	SessionID          string `json:"session_id,omitempty"`
}

// UsageStats represents aggregated usage statistics
type UsageStats struct {
	InputTokens        int     `json:"input_tokens"`
	OutputTokens       int     `json:"output_tokens"`
	CacheRead          int     `json:"cache_read"`
	CacheCreation      int     `json:"cache_creation"`
	ReasoningInput     int     `json:"reasoning_input"`
	ReasoningOutput    int     `json:"reasoning_output"`
	TotalTokens        int     `json:"total_tokens"`
	CallCount          int     `json:"call_count"`
	Cost               float64 `json:"cost"`
}

// ModelUsageStats represents usage statistics per model
type ModelUsageStats struct {
	Model string      `json:"model"`
	Stats UsageStats  `json:"stats"`
}

// TimeRangeStats represents usage statistics for a specific time range
type TimeRangeStats struct {
	Label  string            `json:"label"`
	Models []ModelUsageStats `json:"models"`
	Total  UsageStats        `json:"total"`
}

// UsageStore manages persistent usage storage
type UsageStore struct {
	dataDir string
	mu      sync.RWMutex
}

var (
	globalStore *UsageStore
	once        sync.Once
)

// GetUsageStore returns the global usage store instance
func GetUsageStore() *UsageStore {
	once.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			// Fallback to current directory if home dir can't be found
			home = "."
		}
		dataDir := filepath.Join(home, ".dogclaw", "usage")
		globalStore = NewUsageStore(dataDir)
	})
	return globalStore
}

// NewUsageStore creates a new usage store
func NewUsageStore(dataDir string) *UsageStore {
	return &UsageStore{
		dataDir: dataDir,
	}
}

// ensureDataDir ensures the data directory exists
func (s *UsageStore) ensureDataDir() error {
	return os.MkdirAll(s.dataDir, 0755)
}

// getFilePath returns the file path for a given date
func (s *UsageStore) getFilePath(t time.Time) string {
	date := t.Format("2006-01-02")
	return filepath.Join(s.dataDir, fmt.Sprintf("usage-%s.json", date))
}

// RecordUsage records a usage event
func (s *UsageStore) RecordUsage(model string, usage TokenUsage, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureDataDir(); err != nil {
		return err
	}

	now := time.Now()
	record := UsageRecord{
		Timestamp:          now.UnixMilli(),
		Model:              model,
		InputTokens:        usage.InputTokens,
		OutputTokens:       usage.OutputTokens,
		CacheRead:          usage.CacheReadInputTokens,
		CacheCreation:      usage.CacheCreationInputTokens,
		ReasoningInput:     usage.ReasoningInputTokens,
		ReasoningOutput:    usage.ReasoningOutputTokens,
		SessionID:          sessionID,
	}

	filePath := s.getFilePath(now)
	
	// Read existing records
	var records []UsageRecord
	if data, err := os.ReadFile(filePath); err == nil {
		_ = json.Unmarshal(data, &records)
	}

	// Append new record
	records = append(records, record)

	// Write back
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

// GetRecordsForDateRange gets all records for the given date range (inclusive)
func (s *UsageStore) GetRecordsForDateRange(start, end time.Time) ([]UsageRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := s.ensureDataDir(); err != nil {
		return nil, err
	}

	var allRecords []UsageRecord

	// Iterate through each day in the range
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		filePath := s.getFilePath(d)
		data, err := os.ReadFile(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		var records []UsageRecord
		if err := json.Unmarshal(data, &records); err != nil {
			continue
		}

		// Filter by timestamp to be precise
		startMs := start.UnixMilli()
		endMs := end.UnixMilli()
		for _, r := range records {
			if r.Timestamp >= startMs && r.Timestamp <= endMs {
				allRecords = append(allRecords, r)
			}
		}
	}

	return allRecords, nil
}

// AggregateStats aggregates usage records into statistics
func AggregateStats(records []UsageRecord) map[string]*UsageStats {
	modelStats := make(map[string]*UsageStats)

	for _, r := range records {
		stats, ok := modelStats[r.Model]
		if !ok {
			stats = &UsageStats{}
			modelStats[r.Model] = stats
		}

		stats.InputTokens += r.InputTokens
		stats.OutputTokens += r.OutputTokens
		stats.CacheRead += r.CacheRead
		stats.CacheCreation += r.CacheCreation
		stats.ReasoningInput += r.ReasoningInput
		stats.ReasoningOutput += r.ReasoningOutput
		stats.CallCount++
	}

	// Calculate total tokens and cost for each model
	for model, stats := range modelStats {
		stats.TotalTokens = stats.InputTokens + stats.OutputTokens + stats.CacheRead + stats.CacheCreation
		pricing := GetPricingForModel(model)
		// Create a temporary AccumulatedUsage to calculate cost
		acc := &AccumulatedUsage{
			TotalInput:         stats.InputTokens,
			TotalOutput:        stats.OutputTokens,
			TotalCacheRead:     stats.CacheRead,
			TotalCacheCreation: stats.CacheCreation,
			TotalReasoningInput: stats.ReasoningInput,
			TotalReasoningOutput: stats.ReasoningOutput,
		}
		stats.Cost = acc.CalculateCost(pricing)
	}

	return modelStats
}

// GetTimeRangeStats gets usage statistics for predefined time ranges
func (s *UsageStore) GetTimeRangeStats() ([]TimeRangeStats, error) {
	now := time.Now()
	location := now.Location()

	// Define time ranges
	ranges := []struct {
		label string
		start time.Time
		end   time.Time
	}{
		{
			label: "今天",
			start: time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location),
			end:   now,
		},
		{
			label: "昨天",
			start: time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, location),
			end:   time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location).Add(-time.Millisecond),
		},
		{
			label: "本周",
			start: func() time.Time {
				weekday := now.Weekday()
				if weekday == time.Sunday {
					weekday = 7
				}
				return time.Date(now.Year(), now.Month(), now.Day()-int(weekday)+1, 0, 0, 0, 0, location)
			}(),
			end: now,
		},
		{
			label: "本月",
			start: time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, location),
			end:   now,
		},
	}

	var result []TimeRangeStats

	for _, r := range ranges {
		records, err := s.GetRecordsForDateRange(r.start, r.end)
		if err != nil {
			return nil, err
		}

		modelStatsMap := AggregateStats(records)

		var modelStats []ModelUsageStats
		var totalStats UsageStats

		for model, stats := range modelStatsMap {
			modelStats = append(modelStats, ModelUsageStats{
				Model: model,
				Stats: *stats,
			})
			totalStats.InputTokens += stats.InputTokens
			totalStats.OutputTokens += stats.OutputTokens
			totalStats.CacheRead += stats.CacheRead
			totalStats.CacheCreation += stats.CacheCreation
			totalStats.ReasoningInput += stats.ReasoningInput
			totalStats.ReasoningOutput += stats.ReasoningOutput
			totalStats.CallCount += stats.CallCount
			totalStats.TotalTokens += stats.TotalTokens
			totalStats.Cost += stats.Cost
		}

		result = append(result, TimeRangeStats{
			Label:  r.label,
			Models: modelStats,
			Total:  totalStats,
		})
	}

	return result, nil
}
