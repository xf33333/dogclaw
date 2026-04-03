package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultBaseURL      = "https://api.anthropic.com"
	anthropicAPIVersion = "2023-06-01"

	// Retry configuration for 429 rate limiting
	maxRetries    = 5
	baseDelay     = 2 * time.Second
	maxDelay      = 60 * time.Second
	backoffFactor = 2.0
	jitterFactor  = 0.5

	// Leaky bucket rate limiter defaults
	defaultRate  = 1.0 // requests per second
	defaultBurst = 3   // max burst size
)

// ProviderType represents the API provider type
type ProviderType string

const (
	ProviderAnthropic  ProviderType = "anthropic"
	ProviderOpenRouter ProviderType = "openrouter"
	ProviderOpenAI     ProviderType = "openai"
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
		HTTPClient:  &http.Client{},
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
	switch c.Provider {
	case ProviderOpenRouter, ProviderOpenAI:
		return c.sendOpenAICompatibleRequest(ctx, req)
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

		if attempt > 0 {
			delay := c.calculateDelay(attempt)
			log.Printf("[Retry] Attempt %d/%d, waiting %v...", attempt, maxRetries, delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled during retry: %w", ctx.Err())
			}
		}

		resp, err := c.doAnthropicRequest(ctx, body)
		if err == nil {
			return resp, nil
		}

		// Check if it's a 429 error
		if isRateLimitError(err) {
			lastErr = err
			log.Printf("[RateLimit] 429 Too Many Requests (attempt %d/%d)", attempt+1, maxRetries+1)
			continue
		}

		// Non-retryable error
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
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &RateLimitError{StatusCode: resp.StatusCode, Body: string(respBody), RetryAfter: parseRetryAfter(resp)}
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
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

	log.Printf("[OpenRouter] POST %s", endpoint)

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Wait for rate limiter before each attempt
		if err := c.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter wait failed: %w", err)
		}

		if attempt > 0 {
			delay := c.calculateDelay(attempt)
			log.Printf("[Retry] Attempt %d/%d, waiting %v...", attempt, maxRetries, delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled during retry: %w", ctx.Err())
			}
		}

		resp, err := c.doOpenAIRequest(ctx, endpoint, body)
		if err == nil {
			return resp, nil
		}

		// Check if it's a 429 error
		if isRateLimitError(err) {
			lastErr = err
			log.Printf("[RateLimit] 429 Too Many Requests (attempt %d/%d)", attempt+1, maxRetries+1)
			continue
		}

		// Non-retryable error
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
	//httpReq.Header.Set("HTTP-Referer", "https://dogclaw.ai")
	//httpReq.Header.Set("X-Title", "DogClaw")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &RateLimitError{StatusCode: resp.StatusCode, Body: string(respBody), RetryAfter: parseRetryAfter(resp)}
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		var openAIError OpenAIError
		if err := json.Unmarshal(respBody, &openAIError); err == nil && openAIError.Error.Message != "" {
			return nil, fmt.Errorf("API error: %s - %s", resp.Status, openAIError.Error.Message)
		}
		return nil, fmt.Errorf("API error: %s - %s", resp.Status, string(respBody))
	}

	var openAIResp OpenAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
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

// isRateLimitError checks if an error is a 429 rate limit error
func isRateLimitError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "Too Many Requests"))
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
