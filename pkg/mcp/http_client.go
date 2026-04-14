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
	// Call tools/list endpoint
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
	// Prepare request body
	requestBody := map[string]interface{}{
		"name":      toolName,
		"arguments": arguments,
	}

	// Call tools/call endpoint
	response, err := h.callEndpoint(ctx, "tools/call", requestBody)
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

// callEndpoint makes an HTTP request to the specified endpoint
func (h *HTTPClient) callEndpoint(ctx context.Context, endpoint string, body interface{}) ([]byte, error) {
	// Prepare request
	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonData)
	}

	url := fmt.Sprintf("%s/%s", h.baseURL, endpoint)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bodyReader)
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

	return responseBody, nil
}