package usage

import (
	"fmt"
	"strings"
)

// TokenUsage represents token usage from a single API response
type TokenUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}

// AccumulatedUsage tracks total usage across multiple turns
type AccumulatedUsage struct {
	TotalInput         int
	TotalOutput        int
	TotalCacheRead     int
	TotalCacheCreation int
	Turns              int
}

// Add accumulates usage from a single response
func (a *AccumulatedUsage) Add(u TokenUsage) {
	a.TotalInput += u.InputTokens
	a.TotalOutput += u.OutputTokens
	a.TotalCacheRead += u.CacheReadInputTokens
	a.TotalCacheCreation += u.CacheCreationInputTokens
	a.Turns++
}

// TotalTokens returns the sum of all tokens
func (a *AccumulatedUsage) TotalTokens() int {
	return a.TotalInput + a.TotalOutput + a.TotalCacheRead + a.TotalCacheCreation
}

// PricingModel defines cost per 1M tokens
type PricingModel struct {
	InputPrice      float64
	OutputPrice     float64
	CacheReadPrice  float64
	CacheWritePrice float64
}

// DefaultPricing returns default pricing (Claude 3.5 Sonnet rates as fallback)
func DefaultPricing() PricingModel {
	return PricingModel{
		InputPrice:      3.00,  // $3.00 / 1M tokens
		OutputPrice:     15.00, // $15.00 / 1M tokens
		CacheReadPrice:  0.30,  // $0.30 / 1M tokens
		CacheWritePrice: 3.75,  // $3.75 / 1M tokens
	}
}

// ModelPricingMap stores pricing for known models
var ModelPricingMap = map[string]PricingModel{
	"sonnet": DefaultPricing(),
	"opus": {
		InputPrice: 15.00, OutputPrice: 75.00,
		CacheReadPrice: 1.50, CacheWritePrice: 18.75,
	},
	"haiku": {
		InputPrice: 0.80, OutputPrice: 4.00,
		CacheReadPrice: 0.08, CacheWritePrice: 1.00,
	},
}

// GetPricingForModel returns pricing for the given model name
func GetPricingForModel(modelName string) PricingModel {
	name := strings.ToLower(modelName)
	for key, pricing := range ModelPricingMap {
		if strings.Contains(name, key) {
			return pricing
		}
	}
	return DefaultPricing()
}

// CalculateCost computes the total cost based on usage and pricing
func (a *AccumulatedUsage) CalculateCost(pricing PricingModel) float64 {
	cost := 0.0
	cost += float64(a.TotalInput) / 1_000_000.0 * pricing.InputPrice
	cost += float64(a.TotalOutput) / 1_000_000.0 * pricing.OutputPrice
	cost += float64(a.TotalCacheRead) / 1_000_000.0 * pricing.CacheReadPrice
	cost += float64(a.TotalCacheCreation) / 1_000_000.0 * pricing.CacheWritePrice
	return cost
}

// FormatCost returns formatted cost string
func (a *AccumulatedUsage) FormatCost(pricing PricingModel) string {
	return fmt.Sprintf("$%.4f", a.CalculateCost(pricing))
}
