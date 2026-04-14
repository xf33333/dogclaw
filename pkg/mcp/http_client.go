package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// HTTPClient implements MCP client over HTTP with custom headers
type HTTPClient struct {
	server  MCPServer
	client  *http.Client
	baseURL string
}

// NewHTTPClient creates a new HTTP MCP client
func NewHTTPClient(server MCPServer) *HTTPClient {
	return &HTTPClient{
		server:  server,
		client:  &http.Client{},
		baseURL: server.URL,
	}
}

// Connect establishes connection to the HTTP MCP server
func (h *HTTPClient) Connect(ctx context.Context) error {
	// For HTTP, connection is stateless, so we just validate the URL
	if h.baseURL == "" {
		return fmt.Errorf("HTTP server URL is required")
	}
	return nil
}

// Disconnect closes the HTTP client connection
func (h *HTTPClient) Disconnect() error {
	// HTTP client doesn't need explicit disconnect
	return nil
}

// ListTools retrieves all available tools from the HTTP MCP server
func (h *HTTPClient) ListTools(ctx context.Context) ([]MCPTool, error) {
	// Call tools.list JSON-RPC method
	response, err := h.callEndpoint(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

	var toolsResponse struct {
		Tools []MCPTool `json:"tools"`
	}
	if err := json.Unmarshal(response, &toolsResponse); err != nil {
		return nil, fmt.Errorf("failed to parse tools response: %w", err)
	}

	// Set server name for each tool
	for i := range toolsResponse.Tools {
		toolsResponse.Tools[i].ServerName = h.server.Name
	}

	return toolsResponse.Tools, nil
}

// CallTool executes a tool on the HTTP MCP server
func (h *HTTPClient) CallTool(ctx context.Context, toolName string, arguments map[string]interface{}) (*MCPToolCallResult, error) {
	// Prepare request parameters
	requestParams := map[string]interface{}{
		"name":      toolName,
		"arguments": arguments,
	}

	// Call tools.call JSON-RPC method
	response, err := h.callEndpoint(ctx, "tools/call", requestParams)
	if err != nil {
		return nil, err
	}

	var result MCPToolCallResult
	if err := json.Unmarshal(response, &result); err != nil {
		return nil, fmt.Errorf("failed to parse tool call result: %w", err)
	}

	return &result, nil
}

// ServerName returns the name of the MCP server
func (h *HTTPClient) ServerName() string {
	return h.server.Name
}

// callEndpoint makes an HTTP request to the specified endpoint using JSON-RPC
func (h *HTTPClient) callEndpoint(ctx context.Context, method string, params interface{}) ([]byte, error) {
	// Prepare JSON-RPC request
	requestBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      1, // Simple incrementing ID
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON-RPC request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", h.baseURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	for key, value := range h.server.Headers {
		req.Header.Set(key, value)
	}

	// Make request
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP request failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	// Parse JSON-RPC response
	var rpcResponse struct {
		JSONRPC string      `json:"jsonrpc"`
		Result  interface{} `json:"result"`
		Error   *struct {
			Code    int         `json:"code"`
			Message string      `json:"message"`
			Data    interface{} `json:"data,omitempty"`
		} `json:"error,omitempty"`
		ID int `json:"id"`
	}

	if err := json.Unmarshal(responseBody, &rpcResponse); err != nil {
		return nil, fmt.Errorf("failed to parse JSON-RPC response: %w", err)
	}

	// Check for JSON-RPC error
	if rpcResponse.Error != nil {
		return nil, fmt.Errorf("JSON-RPC error: code=%d, message=%s", rpcResponse.Error.Code, rpcResponse.Error.Message)
	}

	// Convert result back to JSON
	resultJSON, err := json.Marshal(rpcResponse.Result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON-RPC result: %w", err)
	}

	return resultJSON, nil
}
