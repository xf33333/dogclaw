package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"dogclaw/internal/logger"

	"github.com/sirupsen/logrus"
)

const (
	defaultBaseURL      = "https://api.anthropic.com"
	anthropicAPIVersion = "2023-06-01"

	// Retry configuration for 429 rate limiting and server errors
	maxRetries    = 20
	baseDelay     = 1 * time.Second
	maxDelay      = 10 * time.Second
	backoffFactor = 2.0
	jitterFactor  = 0.5

	// Leaky bucket rate limiter defaults
	defaultRate  = 1.0 // requests per second
	defaultBurst = 3   // max burst size

	// Timeout for context deadline exceeded funnel retry
	defaultTimeout    = 10 * time.Minute // default HTTP client timeout
	maxFunnelDuration = 10 * time.Minute // max total time for retry funnel
)

// doWithFunnelRetry performs timed retries with fixed 1-second delays.
// Only retries on timeout errors. Returns immediately on success or non-timeout errors.
func (c *Client) doWithFunnelRetry(ctx context.Context, fn func(context.Context) (*MessageResponse, error)) (*MessageResponse, error) {
	// Total max time is controlled by ctx (maxFunnelDuration).
	// 600 attempts * 1s = 10 minutes.
	for attempt := 1; attempt <= 600; attempt++ {
		// Check context before waiting
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("funnel retry cancelled: %w", err)
		}

		// Wait 1 second before this attempt
		select {
		case <-time.After(1 * time.Second):
		case <-ctx.Done():
			return nil, fmt.Errorf("funnel retry cancelled during wait: %w", ctx.Err())
		}

		logger.Info("[FunnelRetry] Attempt %d: retrying (waiting 1s)...", attempt)

		resp, err := fn(ctx)
		if err == nil {
			logger.Info("[FunnelRetry] Success at attempt %d", attempt)
			return resp, nil
		}

		if !isContextDeadlineExceeded(err) {
			// Non-timeout error — don't continue funnel
			return nil, err
		}

		logger.Debug("[FunnelRetry] Timeout at attempt %d, continuing...", attempt)
	}

	return nil, fmt.Errorf("timeout: funnel retry exhausted")
}

// HTTP status codes that are retryable (transient server errors)
// 500, 502, 503, 504 are transient; 501 is not (Not Implemented)
var retryableStatusCodes = map[int]bool{
	http.StatusInternalServerError: true, // 500
	http.StatusBadGateway:          true, // 502
	http.StatusServiceUnavailable:  true, // 503
	http.StatusGatewayTimeout:      true, // 504
	http.StatusTooManyRequests:     true, // 429
}

// ProviderType represents the API provider type
type ProviderType string

const (
	ProviderAnthropic  ProviderType = "anthropic"
	ProviderOpenRouter ProviderType = "openrouter"
	ProviderOpenAI     ProviderType = "openai"
	ProviderOllama     ProviderType = "ollama"
)

// Client is the API client (supports Anthropic and OpenRouter/OpenAI compatible APIs)
type Client struct {
	HTTPClient  *http.Client
	APIKey      string
	BaseURL     string
	Model       string
	Provider    ProviderType
	rateLimiter *LeakyBucket
}

// LeakyBucket implements a token-bucket rate limiter
type LeakyBucket struct {
	rate       float64   // tokens per second
	burst      int       // max tokens
	tokens     float64   // current tokens
	lastRefill time.Time // last time tokens were added
	mu         sync.Mutex
}

// NewLeakyBucket creates a new leaky bucket rate limiter
func NewLeakyBucket(rate float64, burst int) *LeakyBucket {
	return &LeakyBucket{
		rate:       rate,
		burst:      burst,
		tokens:     float64(burst), // start full
		lastRefill: time.Now(),
	}
}

// Wait blocks until a token is available or context is cancelled
func (lb *LeakyBucket) Wait(ctx context.Context) error {
	lb.mu.Lock()
	lb.refill()

	if lb.tokens >= 1.0 {
		lb.tokens--
		lb.mu.Unlock()
		return nil
	}
	lb.mu.Unlock()

	// Calculate wait time
	waitTime := time.Duration(float64(time.Second) / lb.rate)

	select {
	case <-time.After(waitTime):
		lb.mu.Lock()
		lb.refill()
		lb.tokens--
		lb.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// refill adds tokens based on elapsed time
func (lb *LeakyBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(lb.lastRefill).Seconds()
	lb.tokens += elapsed * lb.rate
	if lb.tokens > float64(lb.burst) {
		lb.tokens = float64(lb.burst)
	}
	lb.lastRefill = now
}

// NewClient creates a new API client with auto-detection of provider
func NewClient(apiKey, model, baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	provider := detectProvider(baseURL, model)

	return &Client{
		HTTPClient: &http.Client{
			Timeout: defaultTimeout,
		},
		APIKey:      apiKey,
		BaseURL:     baseURL,
		Model:       model,
		Provider:    provider,
		rateLimiter: NewLeakyBucket(defaultRate, defaultBurst),
	}
}

// SetRateLimit configures the rate limiter
// rate: requests per second
// burst: max burst size
func (c *Client) SetRateLimit(rate float64, burst int) {
	c.rateLimiter = NewLeakyBucket(rate, burst)
}

// detectProvider auto-detects the provider based on baseURL or model name
func detectProvider(baseURL, model string) ProviderType {
	baseURLLower := strings.ToLower(baseURL)

	// Check baseURL first
	if strings.Contains(baseURLLower, "openrouter") {
		return ProviderOpenRouter
	}
	if strings.Contains(baseURLLower, "openai") {
		return ProviderOpenAI
	}
	// Ollama detection: localhost:11434 or 127.0.0.1:11434 or contains "ollama"
	if strings.Contains(baseURLLower, "ollama") ||
		strings.Contains(baseURLLower, "127.0.0.1:11434") ||
		strings.Contains(baseURLLower, "localhost:11434") ||
		strings.HasSuffix(baseURLLower, ":11434") {
		return ProviderOllama
	}

	// Fallback: check model name
	if strings.HasPrefix(model, "openrouter/") || strings.HasPrefix(model, "anthropic/") ||
		strings.HasPrefix(model, "openai/") || strings.HasPrefix(model, "google/") ||
		strings.HasPrefix(model, "meta-llama/") || strings.HasPrefix(model, "mistralai/") ||
		strings.HasPrefix(model, "qwen/") {
		return ProviderOpenRouter
	}

	return ProviderAnthropic
}

// MessageRequest represents a request to the messages API
// See: https://docs.anthropic.com/en/api/messages
type MessageRequest struct {
	Model         string          `json:"model"`
	MaxTokens     int             `json:"max_tokens"`
	System        any             `json:"system,omitempty"` // string or []SystemBlock
	Messages      []MessageParam  `json:"messages"`
	Tools         []ToolParam     `json:"tools,omitempty"`
	ToolChoice    *ToolChoice     `json:"tool_choice,omitempty"`
	Stream        bool            `json:"stream,omitempty"`
	Temperature   float64         `json:"temperature,omitempty"`
	TopP          float64         `json:"top_p,omitempty"`
	StopSequences []string        `json:"stop_sequences,omitempty"`
	Thinking      *ThinkingConfig `json:"thinking,omitempty"`

	// MemorySummary is for logging/observability only, not sent to API.
	MemorySummary *MemorySummary `json:"-"`
}

// MemorySummary holds a human-readable summary of memories loaded
// into this request (for logging/observability only, not sent to API).
type MemorySummary struct {
	ClaudeMDFiles []string // paths of loaded AGENT.md / rules files
	SemanticHits  []string // names of semantically matched memories
	AutoMemPrompt bool     // whether auto-memory prompt was injected
	TotalFiles    int
}

// ThinkingConfig controls the model's thinking behavior
type ThinkingConfig struct {
	Type         string `json:"type"`                    // "enabled" or "disabled"
	BudgetTokens int    `json:"budget_tokens,omitempty"` // Max thinking tokens
}

// SystemBlock represents a system prompt block with optional cache control
type SystemBlock struct {
	Type         string        `json:"type"`
	Text         string        `json:"text"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

// CacheControl for prompt caching
type CacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// MessageParam represents a message in the conversation
// See: https://docs.anthropic.com/en/api/messages#body-messages
type MessageParam struct {
	Role    string `json:"role"`    // "user" or "assistant"
	Content any    `json:"content"` // string or []ContentBlockParam
}

// ContentBlockParam represents a content block in a message
// See: https://docs.anthropic.com/en/api/messages#body-messages-content
type ContentBlockParam struct {
	Type      string `json:"type"`                  // "text", "tool_use", "tool_result", "image"
	Text      string `json:"text,omitempty"`        // For text blocks
	ID        string `json:"id,omitempty"`          // For tool_use blocks
	Name      string `json:"name,omitempty"`        // For tool_use blocks
	Input     any    `json:"input,omitempty"`       // For tool_use blocks (JSON object)
	ToolUseID string `json:"tool_use_id,omitempty"` // For tool_result blocks
	Content   any    `json:"content,omitempty"`     // For tool_result blocks - MUST be []ContentBlockParam!
	IsError   bool   `json:"is_error,omitempty"`    // For tool_result blocks
}

// ToolParam represents a tool definition for the API
// See: https://docs.anthropic.com/en/api/messages#body-tools
type ToolParam struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"` // JSON Schema object
}

// ToolChoice specifies how the model should use tools
type ToolChoice struct {
	Type string `json:"type"`           // "auto", "any", "tool"
	Name string `json:"name,omitempty"` // Tool name when type="tool"
}

// ContentBlock represents a content block in the response
type ContentBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input any    `json:"input,omitempty"`
}

// Usage represents token usage
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// MessageResponse represents a response from the messages API
type MessageResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Content      []ContentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   string         `json:"stop_reason"` // "end_turn", "tool_use", "max_tokens", "stop_sequence"
	StopSequence string         `json:"stop_sequence"`
	Usage        Usage          `json:"usage"`
}

// StreamEvent represents a streaming event from the API
type StreamEvent struct {
	Type         string        `json:"type"`
	Index        int           `json:"index,omitempty"`
	Delta        *Delta        `json:"delta,omitempty"`
	Usage        *Usage        `json:"usage,omitempty"`
	ContentBlock *ContentBlock `json:"content_block,omitempty"`
}

// Delta represents a delta in a streaming event
type Delta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

// OpenAICompatibleRequest represents an OpenAI-compatible chat completion request
// Used for OpenRouter and other OpenAI-compatible APIs
type OpenAICompatibleRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Tools       []OpenAITool    `json:"tools,omitempty"`
	ToolChoice  any             `json:"tool_choice,omitempty"` // "auto", "required", or OpenAIToolChoice
	Stream      bool            `json:"stream,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	TopP        float64         `json:"top_p,omitempty"`
	Stop        []string        `json:"stop,omitempty"`
}

// OpenAIMessage represents a message in OpenAI format
type OpenAIMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content"` // string, nil, or []OpenAIContentBlock
	Name       string           `json:"name,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

// OpenAIContentBlock represents content in a message
type OpenAIContentBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	Index int    `json:"index,omitempty"`
}

// OpenAITool represents a tool definition in OpenAI format
type OpenAITool struct {
	Type     string         `json:"type"` // "function"
	Function OpenAIFunction `json:"function"`
}

// OpenAIFunction represents a function tool definition
type OpenAIFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"` // JSON Schema
}

// OpenAIToolCall represents a tool call in the response
type OpenAIToolCall struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"` // "function"
	Function OpenAIToolCallFunction `json:"function"`
}

// OpenAIToolCallFunction represents the function part of a tool call
type OpenAIToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// OpenAIToolChoice represents tool_choice in OpenAI format
type OpenAIToolChoice struct {
	Type     string            `json:"type"`
	Function map[string]string `json:"function,omitempty"`
}

// OpenAIResponse represents an OpenAI-compatible chat completion response
type OpenAIResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   *OpenAIUsage   `json:"usage,omitempty"`
}

// OpenAIChoice represents a choice in the response
type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
	Delta        *OpenAIDelta  `json:"delta,omitempty"`
}

// OpenAIDelta represents a delta in streaming response
type OpenAIDelta struct {
	Role      string           `json:"role,omitempty"`
	Content   string           `json:"content,omitempty"`
	ToolCalls []OpenAIToolCall `json:"tool_calls,omitempty"`
}

// OpenAIUsage represents token usage
type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAIStreamEvent represents a streaming event from OpenAI-compatible API
type OpenAIStreamEvent struct {
	ID      string         `json:"id,omitempty"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   *OpenAIUsage   `json:"usage,omitempty"`
}

// OpenAIError represents an error response from OpenAI-compatible API
type OpenAIError struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error"`
}

// SendMessage sends a message to the API and returns the response
// Supports both Anthropic and OpenRouter/OpenAI compatible APIs
func (c *Client) SendMessage(ctx context.Context, req *MessageRequest) (*MessageResponse, error) {
	// Log memory summary if present
	if req.MemorySummary != nil {
		ms := req.MemorySummary
		logger.Debug("[Memory] Loaded %d files", ms.TotalFiles)
		for _, f := range ms.ClaudeMDFiles {
			logger.Debug("[Memory] ClaudeMD: %s", f)
		}
		for _, m := range ms.SemanticHits {
			logger.Debug("[Memory] SemanticHit: %s", m)
		}
		if ms.AutoMemPrompt {
			logger.Debug("[Memory] AutoMemPrompt: yes")
		}
	}

	switch c.Provider {
	case ProviderOpenRouter, ProviderOpenAI:
		return c.sendOpenAICompatibleRequest(ctx, req)
	case ProviderOllama:
		return c.sendOllamaRequest(ctx, req)
	default:
		return c.sendAnthropicRequest(ctx, req)
	}
}

// sendAnthropicRequest sends a request to Anthropic API with retry logic
func (c *Client) sendAnthropicRequest(ctx context.Context, req *MessageRequest) (*MessageResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Wait for rate limiter before each attempt
		if err := c.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter wait failed: %w", err)
		}

		// For deadline exceeded errors, we need a derived context — the parent's
		// deadline has already passed, so using it directly would immediately
		// cancel again only if the context itself is still valid. If the parent
		// is still alive (only the inner HTTP request timed out), reuse it.
		reqCtx := ctx
		if lastErr != nil && isContextDeadlineExceeded(lastErr) {
			// Derive a fresh context from the parent; only use it if parent is
			// not itself cancelled/deadline-exceeded.
			if parentErr := ctx.Err(); parentErr != nil {
				// Parent context itself is done — cannot retry
				return nil, fmt.Errorf("context is done: %w", parentErr)
			}
			// Use parent directly — the deadline has not yet fired on parent,
			// only the HTTP client's internal timer fired.
			reqCtx = ctx
		}

		resp, err := c.doAnthropicRequest(reqCtx, body)
		if err == nil {
			return resp, nil
		}

		if isRetryableError(err) {
			lastErr = err
			if isRateLimitError(err) {
				logger.Info("[RateLimit] 429 Too Many Requests (attempt %d/%d)", attempt+1, maxRetries+1)
			} else if isContextDeadlineExceeded(err) {
				logger.Info("[Timeout] context deadline exceeded (attempt %d/%d)", attempt+1, maxRetries+1)
			} else {
				logger.Info("[TransientError] %v (attempt %d/%d)", err, attempt+1, maxRetries+1)
			}

			// Use Retry-After from error if available, otherwise fall back to backoff
			var retryAfter time.Duration
			if rateLimitErr, ok := err.(*RateLimitError); ok && rateLimitErr.RetryAfter > 0 {
				retryAfter = rateLimitErr.RetryAfter
			} else {
				retryAfter = c.calculateDelay(attempt)
			}
			logger.Debug("[Retry] Waiting %v before next attempt...", retryAfter)
			select {
			case <-time.After(retryAfter):
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled during retry: %w", ctx.Err())
			}
			continue
		}

		// Non-retryable error — but timeout errors get funnel retry
		if isContextDeadlineExceeded(err) {
			logger.Info("[Timeout] Entering funnel retry (window: %v)...", maxFunnelDuration)
			funnelCtx, cancel := context.WithTimeout(context.Background(), maxFunnelDuration)
			defer cancel()

			// Copy parent cancellation
			go func() {
				<-ctx.Done()
				cancel()
			}()

			return c.doWithFunnelRetry(funnelCtx, func(ctx context.Context) (*MessageResponse, error) {
				return c.doAnthropicRequest(ctx, body)
			})
		}

		return nil, err
	}

	return nil, fmt.Errorf("max retries exceeded after %d attempts: %w", maxRetries+1, lastErr)
}

// doAnthropicRequest performs a single request to Anthropic API
func (c *Client) doAnthropicRequest(ctx context.Context, body []byte) (*MessageResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, c.wrapNetworkError(ctx, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &RateLimitError{StatusCode: resp.StatusCode, Body: string(respBody), RetryAfter: parseRetryAfter(resp)}
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		if retryableStatusCodes[resp.StatusCode] {
			return nil, &TransientError{StatusCode: resp.StatusCode, Body: string(respBody)}
		}
		// Check for context length exceeded (non-retryable 400 error)
		if resp.StatusCode == http.StatusBadRequest && isContextLengthExceededError(string(respBody), "anthropic") {
			return nil, &ContextLengthExceededError{StatusCode: resp.StatusCode, Body: string(respBody), Provider: ProviderAnthropic}
		}
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(respBody))
	}

	var messageResp MessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&messageResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &messageResp, nil
}

// sendOpenAICompatibleRequest sends a request to OpenRouter/OpenAI compatible API with retry logic
func (c *Client) sendOpenAICompatibleRequest(ctx context.Context, req *MessageRequest) (*MessageResponse, error) {
	// Convert Anthropic-style request to OpenAI format
	openAIReq := c.convertToOpenAIRequest(req)

	body, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// OpenRouter uses /v1/chat/completions endpoint
	// Handle baseURL that already ends with /v1 to avoid double /v1/
	endpoint := c.BaseURL + "/v1/chat/completions"
	if strings.HasSuffix(c.BaseURL, "/v1") {
		endpoint = c.BaseURL + "/chat/completions"
	}
	fmt.Println(endpoint)
	logrus.Debug(endpoint)
	logrus.Infof("[OpenRouter] POST %s", endpoint)

	// Track consecutive failures for leaky bucket adjustment
	var consecutiveTimeouts int

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Wait for rate limiter before each attempt
		if err := c.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter wait failed: %w", err)
		}

		// For deadline exceeded errors, check if parent context is still valid
		reqCtx := ctx
		if lastErr != nil && isContextDeadlineExceeded(lastErr) {
			if parentErr := ctx.Err(); parentErr != nil {
				// Parent context itself is done — cannot retry
				return nil, fmt.Errorf("context is done: %w", parentErr)
			}
			consecutiveTimeouts++
			// After multiple timeouts, use reduced rate to avoid hammering
			if consecutiveTimeouts >= 2 {
				logger.Info("[Timeout] Multiple consecutive timeouts, waiting longer...")
			}
		} else {
			consecutiveTimeouts = 0
		}

		resp, err := c.doOpenAIRequest(reqCtx, endpoint, body)
		if err == nil {
			return resp, nil
		}

		// Check if it's a retryable (transient) error
		if isRetryableError(err) {
			lastErr = err
			if isRateLimitError(err) {
				logger.Info("[RateLimit] 429 Too Many Requests (attempt %d/%d)", attempt+1, maxRetries+1)
			} else if isContextDeadlineExceeded(err) {
				logger.Info("[Timeout] context deadline exceeded (attempt %d/%d)", attempt+1, maxRetries+1)
			} else {
				logger.Info("[TransientError] %v (attempt %d/%d)", err, attempt+1, maxRetries+1)
			}

			// Use Retry-After from error if available, otherwise fall back to backoff
			var retryAfter time.Duration
			if rateLimitErr, ok := err.(*RateLimitError); ok && rateLimitErr.RetryAfter > 0 {
				retryAfter = rateLimitErr.RetryAfter
			} else {
				retryAfter = c.calculateDelay(attempt)
			}
			logger.Debug("[Retry] Waiting %v before next attempt...", retryAfter)
			select {
			case <-time.After(retryAfter):
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled during retry: %w", ctx.Err())
			}
			continue
		}

		// Non-retryable error — but timeout errors get funnel retry
		if isContextDeadlineExceeded(err) {
			logger.Info("[Timeout] Entering funnel retry (window: %v)...", maxFunnelDuration)
			funnelCtx, cancel := context.WithTimeout(context.Background(), maxFunnelDuration)
			defer cancel()

			// Copy parent cancellation
			go func() {
				<-ctx.Done()
				cancel()
			}()

			return c.doWithFunnelRetry(funnelCtx, func(ctx context.Context) (*MessageResponse, error) {
				return c.doOpenAIRequest(ctx, endpoint, body)
			})
		}

		return nil, err
	}

	return nil, fmt.Errorf("max retries exceeded after %d attempts: %w", maxRetries+1, lastErr)
}

// doOpenAIRequest performs a single request to OpenAI-compatible API
func (c *Client) doOpenAIRequest(ctx context.Context, endpoint string, body []byte) (*MessageResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	// OpenRouter specific headers
	httpReq.Header.Set("HTTP-Referer", "https://github.com/xf33333/dogclaw")
	httpReq.Header.Set("X-Title", "DogClaw")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, c.wrapNetworkError(ctx, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &RateLimitError{StatusCode: resp.StatusCode, Body: string(respBody), RetryAfter: parseRetryAfter(resp)}
	}

	if resp.StatusCode != http.StatusOK {
		if retryableStatusCodes[resp.StatusCode] {
			return nil, &TransientError{StatusCode: resp.StatusCode, Body: string(respBody)}
		}
		// Check for context length exceeded (non-retryable 400 error)
		if resp.StatusCode == http.StatusBadRequest && isContextLengthExceededError(string(respBody), "anthropic") {
			return nil, &ContextLengthExceededError{StatusCode: resp.StatusCode, Body: string(respBody), Provider: ProviderAnthropic}
		}
		// Check OpenAI-style error (for direct OpenAI calls or OpenRouter)
		var openAIError OpenAIError
		if err := json.Unmarshal(respBody, &openAIError); err == nil && openAIError.Error.Message != "" {
			if isContextLengthExceededError(string(respBody), "openai") {
				return nil, &ContextLengthExceededError{StatusCode: resp.StatusCode, Body: string(respBody), Provider: c.Provider}
			}
			return nil, fmt.Errorf("API error: %s - %s", resp.Status, openAIError.Error.Message)
		}
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(respBody))
	}

	// Double check for error field even in 200 OK (some providers do this)
	var errObj struct {
		Error any `json:"error"`
	}
	if err := json.Unmarshal(respBody, &errObj); err == nil && errObj.Error != nil {
		return nil, fmt.Errorf("API error (200 OK): %v", errObj.Error)
	}

	var openAIResp OpenAIResponse
	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w (body: %s)", err, string(respBody))
	}

	// Validate choices
	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("API returned 200 OK with no choices: %s", string(respBody))
	}

	// Convert OpenAI response to Anthropic format
	return c.convertFromOpenAIResponse(&openAIResp), nil
}

// convertToOpenAIRequest converts an Anthropic-style request to OpenAI format
func (c *Client) convertToOpenAIRequest(req *MessageRequest) *OpenAICompatibleRequest {
	messages := c.convertMessages(req)
	tools := c.convertTools(req)

	openAIReq := &OpenAICompatibleRequest{
		Model:       req.Model,
		Messages:    messages,
		Tools:       tools,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.StopSequences,
	}

	// Handle tool_choice
	if req.ToolChoice != nil {
		switch req.ToolChoice.Type {
		case "auto":
			openAIReq.ToolChoice = "auto"
		case "any":
			openAIReq.ToolChoice = "required"
		case "tool":
			openAIReq.ToolChoice = OpenAIToolChoice{
				Type:     "function",
				Function: map[string]string{"name": req.ToolChoice.Name},
			}
		}
	} else if len(tools) > 0 {
		openAIReq.ToolChoice = "auto"
	}

	// Add system message if present
	if req.System != nil {
		systemMsg := OpenAIMessage{
			Role:    "system",
			Content: req.System,
		}
		openAIReq.Messages = append([]OpenAIMessage{systemMsg}, openAIReq.Messages...)
	}

	return openAIReq
}

// convertMessages converts Anthropic messages to OpenAI format
func (c *Client) convertMessages(req *MessageRequest) []OpenAIMessage {
	var messages []OpenAIMessage

	for _, msg := range req.Messages {
		switch msg.Role {
		case "user":
			openAIMsg := c.convertUserMessage(msg)
			messages = append(messages, openAIMsg)
		case "assistant":
			openAIMsg := c.convertAssistantMessage(msg)
			messages = append(messages, openAIMsg)
		case "tool":
			// Tool result messages
			openAIMsg := c.convertToolResultMessage(msg)
			messages = append(messages, openAIMsg)
		}
	}

	return messages
}

// convertUserMessage converts a user message
func (c *Client) convertUserMessage(msg MessageParam) OpenAIMessage {
	// Check if content is a simple string or array of content blocks
	if content, ok := msg.Content.(string); ok {
		return OpenAIMessage{
			Role:    "user",
			Content: content,
		}
	}

	// Content blocks
	if blocks, ok := msg.Content.([]ContentBlockParam); ok {
		var textParts []string
		for _, block := range blocks {
			if block.Type == "text" {
				textParts = append(textParts, block.Text)
			} else if block.Type == "tool_result" {
				// Tool results in user messages - extract text
				if contentBlocks, ok := block.Content.([]ContentBlockParam); ok {
					for _, cb := range contentBlocks {
						if cb.Type == "text" {
							textParts = append(textParts, cb.Text)
						}
					}
				}
			}
		}
		return OpenAIMessage{
			Role:    "user",
			Content: strings.Join(textParts, "\n"),
		}
	}

	return OpenAIMessage{
		Role:    "user",
		Content: msg.Content,
	}
}

// convertAssistantMessage converts an assistant message
func (c *Client) convertAssistantMessage(msg MessageParam) OpenAIMessage {
	if content, ok := msg.Content.(string); ok {
		return OpenAIMessage{
			Role:    "assistant",
			Content: content,
		}
	}

	// Content blocks - check for tool_use
	if blocks, ok := msg.Content.([]ContentBlockParam); ok {
		var textParts []string
		var toolCalls []OpenAIToolCall

		for _, block := range blocks {
			switch block.Type {
			case "text":
				if block.Text != "" {
					textParts = append(textParts, block.Text)
				}
			case "tool_use":
				// Convert tool_use to tool_calls
				inputJSON, _ := json.Marshal(block.Input)
				toolCalls = append(toolCalls, OpenAIToolCall{
					ID:   block.ID,
					Type: "function",
					Function: OpenAIToolCallFunction{
						Name:      block.Name,
						Arguments: string(inputJSON),
					},
				})
			}
		}

		content := strings.Join(textParts, "\n")
		var contentStr any = content
		if content == "" {
			contentStr = nil
		}

		openAIMsg := OpenAIMessage{
			Role:    "assistant",
			Content: contentStr,
		}
		if len(toolCalls) > 0 {
			openAIMsg.ToolCalls = toolCalls
		}
		return openAIMsg
	}

	return OpenAIMessage{
		Role:    "assistant",
		Content: msg.Content,
	}
}

// convertToolResultMessage converts a tool result message (role="user" with tool_result content)
func (c *Client) convertToolResultMessage(msg MessageParam) OpenAIMessage {
	if blocks, ok := msg.Content.([]ContentBlockParam); ok {
		for _, block := range blocks {
			if block.Type == "tool_result" {
				// Extract text content
				if contentBlocks, ok := block.Content.([]ContentBlockParam); ok {
					var textParts []string
					for _, cb := range contentBlocks {
						if cb.Type == "text" {
							textParts = append(textParts, cb.Text)
						}
					}
					return OpenAIMessage{
						Role:       "tool",
						Content:    strings.Join(textParts, "\n"),
						ToolCallID: block.ToolUseID,
					}
				}
			}
		}
	}

	return OpenAIMessage{
		Role:    "tool",
		Content: msg.Content,
	}
}

// convertTools converts Anthropic tools to OpenAI format
func (c *Client) convertTools(req *MessageRequest) []OpenAITool {
	var tools []OpenAITool

	for _, tool := range req.Tools {
		openAITool := OpenAITool{
			Type: "function",
			Function: OpenAIFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			},
		}
		tools = append(tools, openAITool)
	}

	return tools
}

// sendOllamaRequest sends a request to Ollama API (http://localhost:11434/api/chat)
func (c *Client) sendOllamaRequest(ctx context.Context, req *MessageRequest) (*MessageResponse, error) {
	// Build Ollama request body
	type ollamaMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type ollamaReq struct {
		Model    string          `json:"model"`
		Messages []ollamaMessage `json:"messages"`
		Stream   bool            `json:"stream"`
	}
	ollamaMessages := make([]ollamaMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		// Concatenate content blocks into plain text
		var content strings.Builder
		if text, ok := m.Content.(string); ok {
			content.WriteString(text)
		} else if blocks, ok := m.Content.([]ContentBlockParam); ok {
			for _, b := range blocks {
				if b.Type == "text" {
					content.WriteString(b.Text)
				}
			}
		}
		ollamaMessages = append(ollamaMessages, ollamaMessage{
			Role:    m.Role,
			Content: content.String(),
		})
	}
	reqBody := ollamaReq{
		Model:    req.Model,
		Messages: ollamaMessages,
		Stream:   false,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ollama request: %w", err)
	}

	// Endpoint: BaseURL + /api/chat (handle trailing slash)
	endpoint := strings.TrimSuffix(c.BaseURL, "/") + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create ollama request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, c.wrapNetworkError(ctx, err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama API error: %s - %s", resp.Status, string(respBody))
	}

	// Decode Ollama response
	type ollamaResp struct {
		Model     string        `json:"model"`
		CreatedAt string        `json:"created_at"`
		Message   ollamaMessage `json:"message"`
		Done      bool          `json:"done"`
	}
	var ollama struct {
		Model     string        `json:"model"`
		CreatedAt string        `json:"created_at"`
		Message   ollamaMessage `json:"message"`
		Done      bool          `json:"done"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ollama); err != nil {
		return nil, fmt.Errorf("failed to decode ollama response: %w", err)
	}

	// Convert to Anthropic-style MessageResponse
	messageResp := &MessageResponse{
		ID:    fmt.Sprintf("ollama-%d", time.Now().UnixNano()),
		Type:  "message",
		Role:  "assistant",
		Model: ollama.Model,
		Content: []ContentBlock{
			{
				Type: "text",
				Text: ollama.Message.Content,
			},
		},
		StopReason: "end_turn",
	}
	// Ollama doesn't provide token counts, leave Usage zero
	return messageResp, nil
}

// convertFromOpenAIResponse converts an OpenAI response to Anthropic format
func (c *Client) convertFromOpenAIResponse(resp *OpenAIResponse) *MessageResponse {
	messageResp := &MessageResponse{
		ID:    resp.ID,
		Type:  "message",
		Role:  "assistant",
		Model: resp.Model,
	}

	if resp.Usage != nil {
		messageResp.Usage = Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		}
	}

	if len(resp.Choices) == 0 {
		return messageResp
	}

	choice := resp.Choices[0]
	message := choice.Message

	var contentBlocks []ContentBlock

	// Add text content if present
	if content, ok := message.Content.(string); ok && content != "" {
		contentBlocks = append(contentBlocks, ContentBlock{
			Type: "text",
			Text: content,
		})
	} else if blocks, ok := message.Content.([]any); ok {
		for _, b := range blocks {
			if m, ok := b.(map[string]any); ok {
				// Standard format: { "type": "text", "text": "..." }
				if t, ok := m["type"].(string); ok && t == "text" {
					if text, ok := m["text"].(string); ok && text != "" {
						contentBlocks = append(contentBlocks, ContentBlock{
							Type: "text",
							Text: text,
						})
					}
				} else if text, ok := m["text"].(string); ok && text != "" {
					// Fallback: { "text": "..." }
					contentBlocks = append(contentBlocks, ContentBlock{
						Type: "text",
						Text: text,
					})
				}
			}
		}
	} else if m, ok := message.Content.(map[string]any); ok {
		// Single object fallback
		if text, ok := m["text"].(string); ok && text != "" {
			contentBlocks = append(contentBlocks, ContentBlock{
				Type: "text",
				Text: text,
			})
		}
	}

	// Convert tool_calls to tool_use content blocks
	for _, tc := range message.ToolCalls {
		var inputMap map[string]any
		json.Unmarshal([]byte(tc.Function.Arguments), &inputMap)

		contentBlocks = append(contentBlocks, ContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: inputMap,
		})
	}

	messageResp.Content = contentBlocks

	// Map finish_reason to stop_reason
	switch choice.FinishReason {
	case "stop":
		messageResp.StopReason = "end_turn"
	case "tool_calls":
		messageResp.StopReason = "tool_use"
	case "length":
		messageResp.StopReason = "max_tokens"
	case "content_filter":
		messageResp.StopReason = "stop_sequence"
	default:
		messageResp.StopReason = choice.FinishReason
	}

	return messageResp
}

// RateLimitError represents a 429 rate limit error
type RateLimitError struct {
	StatusCode int
	Body       string
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limit exceeded (HTTP %d): %s", e.StatusCode, e.Body)
}

// TransientError represents a retryable server error (5xx)
type TransientError struct {
	StatusCode int
	Body       string
}

func (e *TransientError) Error() string {
	return fmt.Sprintf("transient server error (HTTP %d): %s", e.StatusCode, e.Body)
}

// ContextLengthExceededError represents a non-retryable error where the
// prompt exceeds the model's context window (HTTP 400).
// Anthropic returns: {"type":"error","error":{"type":"overloaded_error","message":"prompt is too long"}}
// OpenRouter/OpenAI returns: {"error":{"message":"This model's maximum context length is...","type":"invalid_request_error"}}
type ContextLengthExceededError struct {
	StatusCode int
	Body       string
	Provider   ProviderType
}

func (e *ContextLengthExceededError) Error() string {
	return fmt.Sprintf("context length exceeded (HTTP %d): %s", e.StatusCode, e.Body)
}

// ContextDeadlineExceededError represents a request timeout (HTTP client deadline exceeded).
// This occurs when the server doesn't respond within the specified timeout,
// NOT when the prompt is too long. It's potentially retryable.
type ContextDeadlineExceededError struct {
	Err error
}

func (e *ContextDeadlineExceededError) Error() string {
	return fmt.Sprintf("context deadline exceeded: %v", e.Err)
}

func (e *ContextDeadlineExceededError) Unwrap() error {
	return e.Err
}

// wrapNetworkError wraps network-level errors with appropriate typed errors.
// Distinguishes between:
//   - context.Canceled: user cancelled (not retryable)
//   - context.DeadlineExceeded: request timeout (retryable)
//   - DNS/connection errors: network issues (retryable)
func (c *Client) wrapNetworkError(ctx context.Context, err error) error {
	// Check if context was cancelled first
	if errors.Is(ctx.Err(), context.Canceled) {
		return fmt.Errorf("request cancelled: %w", err)
	}

	// Check for deadline exceeded (timeout)
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &ContextDeadlineExceededError{Err: err}
	}

	// Check for network-level errors (DNS, connection refused, etc.)
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return &ContextDeadlineExceededError{Err: err}
		}
		// Other network errors are also potentially retryable
		return &TransientError{StatusCode: 0, Body: err.Error()}
	}

	// Check for URL errors (connection refused, etc.)
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		// URL errors are typically network issues
		return &TransientError{StatusCode: 0, Body: err.Error()}
	}

	return fmt.Errorf("failed to send request: %w", err)
}

// isContextDeadlineExceeded checks if an error is a context deadline exceeded error
func isContextDeadlineExceeded(err error) bool {
	if err == nil {
		return false
	}
	var deadlineErr *ContextDeadlineExceededError
	if errors.As(err, &deadlineErr) {
		return true
	}
	// Also check generic context.DeadlineExceeded (may be wrapped in fmt.Errorf)
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// Fallback: check error message
	errStr := err.Error()
	return strings.Contains(errStr, "context deadline exceeded") ||
		strings.Contains(errStr, "Client.Timeout exceeded while awaiting headers") ||
		strings.Contains(errStr, "i/o timeout")
}

// isRateLimitError checks if an error is a 429 rate limit error
func isRateLimitError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "Too Many Requests"))
}

// isContextLengthExceededError detects if a raw API response body indicates
// that the prompt exceeds the model's context window.
// Supports both Anthropic and OpenAI-style error formats.
func isContextLengthExceededError(body string, provider string) bool {
	bodyLower := strings.ToLower(body)

	// Anthropic patterns
	// {"type":"error","error":{"type":"overloaded_error","message":"prompt is too long"}}
	// {"type":"error","error":{"type":"invalid_request_error","message":"...too many tokens..."}}
	if provider == "anthropic" || provider == "" {
		if strings.Contains(bodyLower, "prompt is too long") ||
			strings.Contains(bodyLower, "too many tokens") ||
			strings.Contains(bodyLower, "overloaded_error") ||
			strings.Contains(bodyLower, "exceeds the maximum number of tokens") ||
			strings.Contains(bodyLower, "messages result in") && strings.Contains(bodyLower, "tokens which exceeds the remaining") {
			return true
		}
	}

	// OpenAI/OpenRouter patterns
	// {"error":{"message":"This model's maximum context length is...","type":"invalid_request_error"}}
	if provider == "openai" || provider == "" {
		if strings.Contains(bodyLower, "maximum context length") ||
			strings.Contains(bodyLower, "context_length_exceeded") ||
			strings.Contains(bodyLower, "prompt is too long") ||
			strings.Contains(bodyLower, "reduce the length") ||
			strings.Contains(bodyLower, "this model's maximum") && strings.Contains(bodyLower, "context") {
			return true
		}
	}

	return false
}

// isRetryableError checks if an error is retryable (rate limit, transient server error, or timeout)
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if isRateLimitError(err) {
		return true
	}
	// Check for TransientError
	var transientErr *TransientError
	if errAs(err, &transientErr) {
		return true
	}
	// Context deadline exceeded is retryable — it's a timeout, not a cancellation
	if isContextDeadlineExceeded(err) {
		return true
	}
	// Fallback: check error message for common transient patterns
	errStr := err.Error()
	return strings.Contains(errStr, "500") || strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") || strings.Contains(errStr, "504")
}

// errAs is a helper for type assertion (like errors.As)
func errAs(err error, target any) bool {
	switch t := target.(type) {
	case **TransientError:
		if te, ok := err.(*TransientError); ok {
			*t = te
			return true
		}
	case **RateLimitError:
		if rle, ok := err.(*RateLimitError); ok {
			*t = rle
			return true
		}
	}
	return false
}

// calculateDelay calculates delay with exponential backoff and jitter
func (c *Client) calculateDelay(attempt int) time.Duration {
	// Exponential backoff: baseDelay * backoffFactor^attempt
	delay := float64(baseDelay) * math.Pow(backoffFactor, float64(attempt-1))

	// Cap at maxDelay
	if delay > float64(maxDelay) {
		delay = float64(maxDelay)
	}

	// Add jitter: ±jitterFactor * delay
	jitter := (rand.Float64()*2 - 1) * jitterFactor * delay
	delay += jitter

	// Ensure positive delay
	if delay < 0 {
		delay = float64(baseDelay)
	}

	return time.Duration(delay)
}

// parseRetryAfter parses Retry-After header from response
func parseRetryAfter(resp *http.Response) time.Duration {
	// Check Retry-After header
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
		// Try parsing as seconds
		if seconds, err := strconv.Atoi(retryAfter); err == nil {
			return time.Duration(seconds) * time.Second
		}
		// Try parsing as HTTP date
		if t, err := time.Parse(time.RFC1123, retryAfter); err == nil {
			return time.Until(t)
		}
	}

	// Check RateLimit-Reset header (Unix timestamp)
	if reset := resp.Header.Get("RateLimit-Reset"); reset != "" {
		if seconds, err := strconv.Atoi(reset); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}

	// Default fallback
	return baseDelay
}
