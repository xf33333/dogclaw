package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// OAuthClient implements MCP client over HTTP with OAuth authentication
type OAuthClient struct {
	server      MCPServer
	client      *http.Client
	baseURL     string
	oauthConfig OAuthConfig
	token       string
	tokenMutex  sync.RWMutex
}

// NewOAuthClient creates a new OAuth MCP client
func NewOAuthClient(server MCPServer) *OAuthClient {
	return &OAuthClient{
		server:      server,
		client:      &http.Client{},
		baseURL:     server.URL,
		oauthConfig: server.OAuth,
		token:       server.OAuth.Token,
	}
}

// Connect establishes connection to OAuth MCP server and obtains access token
func (o *OAuthClient) Connect(ctx context.Context) error {
	// Validate OAuth configuration
	if o.oauthConfig.TokenURL == "" {
		return fmt.Errorf("OAuth token URL is required")
	}
	if o.oauthConfig.ClientID == "" {
		return fmt.Errorf("OAuth client ID is required")
	}
	if o.oauthConfig.ClientSecret == "" {
		return fmt.Errorf("OAuth client secret is required")
	}
	if o.baseURL == "" {
		return fmt.Errorf("OAuth server URL is required")
	}

	// If token is not provided, obtain one
	if o.token == "" {
		if err := o.obtainToken(ctx); err != nil {
			return fmt.Errorf("failed to obtain OAuth token: %w", err)
		}
	}

	return nil
}

// Disconnect closes OAuth client connection
func (o *OAuthClient) Disconnect() error {
	// OAuth client doesn't need explicit disconnect
	return nil
}

// ListTools retrieves all available tools from OAuth MCP server
func (o *OAuthClient) ListTools(ctx context.Context) ([]MCPTool, error) {
	// Call tools.list JSON-RPC method
	response, err := o.callEndpoint(ctx, "tools.list", nil)
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
		toolsResponse.Tools[i].ServerName = o.server.Name
	}

	return toolsResponse.Tools, nil
}

// CallTool executes a tool on OAuth MCP server
func (o *OAuthClient) CallTool(ctx context.Context, toolName string, arguments map[string]interface{}) (*MCPToolCallResult, error) {
	// Prepare request parameters
	requestParams := map[string]interface{}{
		"name":      toolName,
		"arguments": arguments,
	}

	// Call tools.call JSON-RPC method
	response, err := o.callEndpoint(ctx, "tools.call", requestParams)
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
func (o *OAuthClient) ServerName() string {
	return o.server.Name
}

// obtainToken obtains a new OAuth access token
func (o *OAuthClient) obtainToken(ctx context.Context) error {
	// Prepare OAuth token request
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", o.oauthConfig.ClientID)
	data.Set("client_secret", o.oauthConfig.ClientSecret)
	if o.oauthConfig.Scope != "" {
		data.Set("scope", o.oauthConfig.Scope)
	}

	// Make token request
	req, err := http.NewRequestWithContext(ctx, "POST", o.oauthConfig.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to obtain token: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read token response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse token response
	var tokenResponse struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		return fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResponse.AccessToken == "" {
		return fmt.Errorf("no access token in response")
	}

	// Store token
	o.tokenMutex.Lock()
	o.token = tokenResponse.AccessToken
	o.tokenMutex.Unlock()

	return nil
}

// callEndpoint makes an authenticated HTTP request to the specified endpoint using JSON-RPC
func (o *OAuthClient) callEndpoint(ctx context.Context, method string, params interface{}) ([]byte, error) {
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

	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Add OAuth token
	o.tokenMutex.RLock()
	token := o.token
	o.tokenMutex.RUnlock()

	if token == "" {
		return nil, fmt.Errorf("no OAuth token available")
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	// Make request
	resp, err := o.client.Do(req)
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
	if resp.StatusCode == http.StatusUnauthorized {
		// Token might be expired, try to obtain new token
		if err := o.obtainToken(ctx); err != nil {
			return nil, fmt.Errorf("failed to refresh token: %w", err)
		}
		// Retry request with new token
		return o.callEndpoint(ctx, method, params)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP request failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	// Parse JSON-RPC response
	var rpcResponse struct {
		JSONRPC string      `json:"jsonrpc"`
		Result  interface{} `json:"result"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
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