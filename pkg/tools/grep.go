package tools

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"dogclaw/pkg/types"
)

// GrepTool implements content search using ripgrep
type GrepTool struct{}

func NewGrepTool() *GrepTool {
	return &GrepTool{}
}

func (t *GrepTool) Name() string      { return "Grep" }
func (t *GrepTool) Aliases() []string { return []string{"search", "rg", "ripgrep"} }

func (t *GrepTool) InputSchema() types.ToolInputSchema {
	return types.ToolInputSchema{
		Type: "object",
		Properties: map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Regex pattern to search for",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "File or directory to search in",
			},
			"glob": map[string]any{
				"type":        "string",
				"description": "Optional glob to filter files (e.g., '*.go')",
			},
			"output_mode": map[string]any{
				"type":        "string",
				"description": "Output mode: 'content' shows matching lines, 'files_with_matches' shows file names only",
				"enum":        []string{"content", "files_with_matches"},
			},
			"case_sensitive": map[string]any{
				"type":        "boolean",
				"description": "Whether to use case-sensitive matching",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of results to return",
			},
		},
		Required: []string{"pattern"},
	}
}

func (t *GrepTool) Description(input map[string]any, opts types.ToolDescriptionOptions) string {
	return "Search for a regex pattern in files using ripgrep. " +
		"Fast content search across directories with regex support. " +
		"Returns matching lines with file paths and line numbers."
}

func (t *GrepTool) Call(ctx context.Context, input map[string]any, toolCtx types.ToolUseContext, onProgress types.ToolCallProgress) (*types.ToolResult, error) {
	pattern, ok := input["pattern"].(string)
	if !ok || pattern == "" {
		return &types.ToolResult{
			Data:    "Error: 'pattern' parameter is required",
			IsError: true,
		}, nil
	}

	searchPath := "."
	if path, ok := input["path"].(string); ok && path != "" {
		searchPath = path
	}

	bashTool := NewBashTool()

	// Check if rg is available
	checkCmd := "rg --version > /dev/null 2>&1"
	checkResult, _ := bashTool.Call(ctx, map[string]any{"command": checkCmd}, toolCtx, nil)
	useRipgrep := !checkResult.IsError

	var cmd string
	if useRipgrep {
		// Build ripgrep command
		var args []string
		args = append(args, "rg")

		if outputMode, ok := input["output_mode"].(string); ok && outputMode == "files_with_matches" {
			args = append(args, "-l")
		}

		if caseSensitive, ok := input["case_sensitive"].(bool); !ok || !caseSensitive {
			args = append(args, "-i") // case insensitive by default
		}

		if maxResults, ok := input["max_results"].(int); ok && maxResults > 0 {
			args = append(args, "-m", fmt.Sprintf("%d", maxResults))
		} else {
			args = append(args, "-m", "50") // default limit
		}

		if glob, ok := input["glob"].(string); ok && glob != "" {
			args = append(args, "-g", QuoteShellArg(glob))
		}

		args = append(args, "--line-number")
		args = append(args, QuoteShellArg(pattern))
		args = append(args, QuoteShellArg(searchPath))

		cmd = fmt.Sprintf("%s 2>&1", strings.Join(args, " "))
	} else {
		// Fallback to standard grep
		var args []string
		args = append(args, "grep", "-r", "-n")

		if outputMode, ok := input["output_mode"].(string); ok && outputMode == "files_with_matches" {
			args = append(args, "-l")
		}

		if caseSensitive, ok := input["case_sensitive"].(bool); !ok || !caseSensitive {
			args = append(args, "-i")
		}

		// Standard grep doesn't have an easy -m flag across all platforms, but we can use head
		limit := 50
		if maxResults, ok := input["max_results"].(int); ok && maxResults > 0 {
			limit = maxResults
		}

		if glob, ok := input["glob"].(string); ok && glob != "" {
			args = append(args, "--include", QuoteShellArg(glob))
		}

		args = append(args, QuoteShellArg(pattern))
		args = append(args, QuoteShellArg(searchPath))

		cmd = fmt.Sprintf("%s 2>&1 | head -n %d", strings.Join(args, " "), limit)
	}

	// Execute via bash tool
	result, err := bashTool.Call(ctx, map[string]any{"command": cmd}, toolCtx, nil)
	if err != nil {
		return &types.ToolResult{
			Data:    fmt.Sprintf("Error executing grep: %v", err),
			IsError: true,
		}, nil
	}

	// Correctly pass through result data and error status
	output := result.Data.(string)

	// In case of error (like file not found), result.IsError will be true
	if result.IsError {
		return result, nil
	}

	// 检查输出内容长度，避免超过上下文限制
	const maxContentLength = 16000 // 大约 16KB 的字符限制
	if utf8.RuneCountInString(output) > maxContentLength {
		return &types.ToolResult{
			Data:    fmt.Sprintf("搜索结果内容太多，超过了长度限制。请缩小搜索范围，例如使用更精确的模式、限定目录路径或使用 glob 过滤文件类型。当前匹配行数: %d", strings.Count(output, "\n")+1),
			IsError: true,
		}, nil
	}

	// Optimization for model feedback
	if !strings.Contains(output, "\n") && output == "" {
		return &types.ToolResult{
			Data:    fmt.Sprintf("No matches found for pattern: %s in %s", pattern, searchPath),
			IsError: false,
		}, nil
	}

	return result, nil
}

func (t *GrepTool) IsConcurrencySafe(input map[string]any) bool { return true }
func (t *GrepTool) IsReadOnly(input map[string]any) bool        { return true }
func (t *GrepTool) IsDestructive(input map[string]any) bool     { return false }
func (t *GrepTool) IsEnabled() bool                             { return true }
func (t *GrepTool) SearchHint() string                          { return "search content regex ripgrep find text" }
