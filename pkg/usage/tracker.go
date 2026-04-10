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
	// Reasoning tokens from extended thinking
	ReasoningInputTokens  int `json:"reasoning_input_tokens,omitempty"`
	ReasoningOutputTokens int `json:"reasoning_output_tokens,omitempty"`
}

// AccumulatedUsage tracks total usage across multiple turns
type AccumulatedUsage struct {
	TotalInput          int
	TotalOutput         int
	TotalCacheRead      int
	TotalCacheCreation  int
	TotalReasoningInput int
	TotalReasoningOutput int
	Turns               int
}

// Add accumulates usage from a single response
func (a *AccumulatedUsage) Add(u TokenUsage) {
	a.TotalInput += u.InputTokens
	a.TotalOutput += u.OutputTokens
	a.TotalCacheRead += u.CacheReadInputTokens
	a.TotalCacheCreation += u.CacheCreationInputTokens
	a.TotalReasoningInput += u.ReasoningInputTokens
	a.TotalReasoningOutput += u.ReasoningOutputTokens
	a.Turns++
}

// TotalTokens returns the sum of all tokens
func (a *AccumulatedUsage) TotalTokens() int {
	return a.TotalInput + a.TotalOutput + a.TotalCacheRead + a.TotalCacheCreation + a.TotalReasoningInput + a.TotalReasoningOutput
}

// PricingModel defines cost per 1M tokens
type PricingModel struct {
	InputPrice         float64
	OutputPrice        float64
	CacheReadPrice     float64
	CacheWritePrice    float64
	ReasoningInputPrice  float64
	ReasoningOutputPrice float64
}

// DefaultPricing returns default pricing (Claude 3.5 Sonnet rates as fallback)
func DefaultPricing() PricingModel {
	return PricingModel{
		InputPrice:           3.00,  // $3.00 / 1M tokens
		OutputPrice:          15.00, // $15.00 / 1M tokens
		CacheReadPrice:       0.30,  // $0.30 / 1M tokens
		CacheWritePrice:      3.75,  // $3.75 / 1M tokens
		ReasoningInputPrice:  3.00,  // $3.00 / 1M tokens (同输入)
		ReasoningOutputPrice: 15.00, // $15.00 / 1M tokens (同输出)
	}
}

// ModelPricingMap stores pricing for known models
var ModelPricingMap = map[string]PricingModel{
	"sonnet": DefaultPricing(),
	"opus": {
		InputPrice: 15.00, OutputPrice: 75.00,
		CacheReadPrice: 1.50, CacheWritePrice: 18.75,
		ReasoningInputPrice: 15.00, ReasoningOutputPrice: 75.00,
	},
	"haiku": {
		InputPrice: 0.80, OutputPrice: 4.00,
		CacheReadPrice: 0.08, CacheWritePrice: 1.00,
		ReasoningInputPrice: 0.80, ReasoningOutputPrice: 4.00,
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
	cost += float64(a.TotalReasoningInput) / 1_000_000.0 * pricing.ReasoningInputPrice
	cost += float64(a.TotalReasoningOutput) / 1_000_000.0 * pricing.ReasoningOutputPrice
	return cost
}

// FormatCost returns formatted cost string
func (a *AccumulatedUsage) FormatCost(pricing PricingModel) string {
	return FormatCost(a.CalculateCost(pricing))
}

// FormatCost formats a cost value consistently
func FormatCost(cost float64) string {
	return fmt.Sprintf("$%.4f", cost)
}

// CostBreakdown represents the percentage cost breakdown
type CostBreakdown struct {
	InputPercentage         float64
	OutputPercentage        float64
	CacheReadPercentage     float64
	CacheWritePercentage    float64
	ReasoningInputPercentage  float64
	ReasoningOutputPercentage float64
}

// GetCostBreakdown returns the cost breakdown in percentages
func (a *AccumulatedUsage) GetCostBreakdown(pricing PricingModel) CostBreakdown {
	totalCost := a.CalculateCost(pricing)
	if totalCost <= 0 {
		return CostBreakdown{}
	}

	inputCost := float64(a.TotalInput) / 1_000_000.0 * pricing.InputPrice
	outputCost := float64(a.TotalOutput) / 1_000_000.0 * pricing.OutputPrice
	cacheReadCost := float64(a.TotalCacheRead) / 1_000_000.0 * pricing.CacheReadPrice
	cacheWriteCost := float64(a.TotalCacheCreation) / 1_000_000.0 * pricing.CacheWritePrice
	reasoningInputCost := float64(a.TotalReasoningInput) / 1_000_000.0 * pricing.ReasoningInputPrice
	reasoningOutputCost := float64(a.TotalReasoningOutput) / 1_000_000.0 * pricing.ReasoningOutputPrice

	return CostBreakdown{
		InputPercentage:         (inputCost / totalCost) * 100,
		OutputPercentage:        (outputCost / totalCost) * 100,
		CacheReadPercentage:     (cacheReadCost / totalCost) * 100,
		CacheWritePercentage:    (cacheWriteCost / totalCost) * 100,
		ReasoningInputPercentage:  (reasoningInputCost / totalCost) * 100,
		ReasoningOutputPercentage: (reasoningOutputCost / totalCost) * 100,
	}
}
