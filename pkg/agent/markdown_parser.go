package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseFrontmatter 解析 markdown 文件的 frontmatter 和内容
// 支持 YAML frontmatter (--- 分隔) 和 无 frontmatter
func ParseFrontmatter(rawContent string, filePath string) (*FrontmatterResult, error) {
	content := strings.TrimRight(rawContent, "\n\r")

	// 检查是否有 frontmatter (以 --- 开头)
	if !strings.HasPrefix(content, "---") {
		return &FrontmatterResult{
			Data:           make(map[string]interface{}),
			Content:        content,
			HasFrontmatter: false,
		}, nil
	}

	// 查找第二个 --- 结束标记
	endIdx := strings.Index(content[3:], "\n---")
	if endIdx == -1 {
		// 没有结束标记，视为无 frontmatter
		return &FrontmatterResult{
			Data:           make(map[string]interface{}),
			Content:        content,
			HasFrontmatter: false,
		}, nil
	}

	endIdx += 3 // 跳过开头的 "---"
	frontmatterStr := content[3:endIdx]
	body := content[endIdx+4:] // 跳过 "---\n"

	// 解析 YAML
	var data map[string]interface{}
	if err := yaml.Unmarshal([]byte(frontmatterStr), &data); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter in %s: %w", filepath.Base(filePath), err)
	}

	return &FrontmatterResult{
		Data:           data,
		Content:        body,
		HasFrontmatter: true,
	}, nil
}

// LoadMarkdownFiles 从指定目录加载所有 .md 文件
func LoadMarkdownFiles(dir string) ([]MarkdownFile, error) {
	if dir == "" {
		return []MarkdownFile{}, nil
	}

	// 检查目录是否存在
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return []MarkdownFile{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat directory %s: %w", dir, err)
	}

	// 查找所有 .md 文件
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	var files []MarkdownFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}

		filePath := filepath.Join(dir, name)
		raw, err := os.ReadFile(filePath)
		if err != nil {
			// 跳过无法读取的文件
			LogDebug("Failed to read %s: %v", filePath, err)
			continue
		}

		frontmatter, err := ParseFrontmatter(string(raw), filePath)
		if err != nil {
			LogDebug("Failed to parse frontmatter in %s: %v", filePath, err)
			// 即使 frontmatter 解析失败，也视为无效文件跳过
			continue
		}

		files = append(files, MarkdownFile{
			FilePath:    filePath,
			BaseDir:     dir,
			Frontmatter: frontmatter.Data,
			Content:     frontmatter.Content,
			Source:      "", // 由调用者设置
		})
	}

	return files, nil
}

// ParseAgentToolsFromFrontmatter 从 frontmatter 解析工具列表
// 对于 agent: missing = undefined (所有工具), empty = [] (无工具)
func ParseAgentToolsFromFrontmatter(toolsValue interface{}) []string {
	if toolsValue == nil {
		return nil // nil 表示未设置，使用所有工具
	}

	var toolsArray []string
	switch v := toolsValue.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return []string{} // 空字符串 = 无工具
		}
		toolsArray = []string{v}
	case []interface{}:
		toolsArray = make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				toolsArray = append(toolsArray, s)
			}
		}
	default:
		return nil
	}

	if len(toolsArray) == 0 {
		return []string{}
	}

	// 检查是否通配符
	for _, tool := range toolsArray {
		if tool == "*" {
			return nil // nil 表示所有工具
		}
	}

	return toolsArray
}

// ParseSlashCommandToolsFromFrontmatter 解析 slash command 的工具列表
// 对于 slash command: missing/empty = [] (无工具)
func ParseSlashCommandToolsFromFrontmatter(toolsValue interface{}) []string {
	if toolsValue == nil {
		return []string{}
	}

	var toolsArray []string
	switch v := toolsValue.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return []string{}
		}
		toolsArray = []string{v}
	case []interface{}:
		toolsArray = make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				toolsArray = append(toolsArray, s)
			}
		}
	default:
		return []string{}
	}

	if len(toolsArray) == 0 {
		return []string{}
	}

	// 检查通配符
	for _, tool := range toolsArray {
		if tool == "*" {
			return []string{}
		}
	}

	return toolsArray
}

// DeduplicateFiles 基于设备+inode 去重文件，防止硬链接/符号链接导致的重复
func DeduplicateFiles(files []MarkdownFile) []MarkdownFile {
	seen := make(map[string]SettingSource)
	var result []MarkdownFile

	for _, file := range files {
		// 获取文件标识（设备:inode）
		fileID, err := getFileIdentity(file.FilePath)
		if err != nil || fileID == "" {
			// 无法获取标识，保留该文件（fail open）
			result = append(result, file)
			continue
		}

		if existingSource, exists := seen[fileID]; exists {
			LogDebug("Skipping duplicate file '%s' from %s (same inode already loaded from %s)",
				file.FilePath, file.Source, existingSource)
			continue
		}

		seen[fileID] = file.Source
		result = append(result, file)
	}

	return result
}

// LogDebug 调试日志
func LogDebug(format string, args ...interface{}) {
	if os.Getenv("CLAUDE_DEBUG") != "" || os.Getenv("GOCLAUDE_DEBUG") != "" {
		fmt.Printf("[DEBUG] "+format+"\n", args...)
	}
}
