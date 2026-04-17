package experience

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dogclaw/internal/api"
	"dogclaw/internal/logger"
	"dogclaw/pkg/transcript"
)

// GetExperienceList 获取经验文件列表
func (m *Manager) GetExperienceList() ([]ExperienceFile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries, err := os.ReadDir(m.experiencePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read experience directory: %w", err)
	}

	var files []ExperienceFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}

		date := strings.TrimSuffix(name, ".md")
		if _, err := time.Parse(DateFormat, date); err != nil {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, ExperienceFile{
			Date:     date,
			FilePath: filepath.Join(m.experiencePath, name),
			Size:     info.Size(),
			ModTime:  info.ModTime(),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Date > files[j].Date
	})

	return files, nil
}

// GetExperience 获取指定日期的经验内容
func (m *Manager) GetExperience(date string) (string, error) {
	if _, err := time.Parse(DateFormat, date); err != nil {
		return "", fmt.Errorf("invalid date format, expected yyyy-mm-dd: %w", err)
	}

	filePath := filepath.Join(m.experiencePath, date+".md")

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("experience for %s not found", date)
		}
		return "", fmt.Errorf("failed to read experience file: %w", err)
	}

	return string(data), nil
}

// GetSummary 获取经验汇总内容（EXPERIENCE.md）
func (m *Manager) GetSummary() (string, error) {
	indexFile := filepath.Join(m.experiencePath, "EXPERIENCE.md")

	data, err := os.ReadFile(indexFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("experience summary (EXPERIENCE.md) not found")
		}
		return "", fmt.Errorf("failed to read experience summary: %w", err)
	}

	return string(data), nil
}

// SaveExperience 保存指定日期的经验
func (m *Manager) SaveExperience(date string, content string) error {
	if _, err := time.Parse(DateFormat, date); err != nil {
		return fmt.Errorf("invalid date format, expected yyyy-mm-dd: %w", err)
	}

	filePath := filepath.Join(m.experiencePath, date+".md")

	header := fmt.Sprintf("# %s 经验总结\n\n", date)
	if !strings.HasPrefix(content, header) {
		content = header + content
	}

	footer := fmt.Sprintf("\n\n---\n\n*自动生成于 %s*\n", time.Now().Format(time.RFC3339))
	content = content + footer

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write experience file: %w", err)
	}

	return nil
}

// HasExperience 检查指定日期是否已有经验
func (m *Manager) HasExperience(date string) bool {
	filePath := filepath.Join(m.experiencePath, date+".md")
	_, err := os.Stat(filePath)
	return err == nil
}

// DeleteExperience 删除指定日期的经验
func (m *Manager) DeleteExperience(date string) error {
	filePath := filepath.Join(m.experiencePath, date+".md")

	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("experience for %s not found", date)
		}
		return fmt.Errorf("failed to delete experience file: %w", err)
	}

	return nil
}

// GetPendingSummaries 获取需要总结的日期列表
func (m *Manager) GetPendingSummaries() []string {
	m.mu.RLock()
	lastSummaryDate := m.metadata.LastSummaryDate
	m.mu.RUnlock()

	var pendingDates []string

	// 如果 lastSummaryDate 为空，扫描有 session 数据的日期
	if lastSummaryDate == "" {
		return m.scanSessionDates()
	}

	lastDate, err := time.Parse(DateFormat, lastSummaryDate)
	if err != nil {
		return pendingDates
	}

	yesterday := time.Now().AddDate(0, 0, -1).Format(DateFormat)

	for d := lastDate.AddDate(0, 0, 1); d.Format(DateFormat) <= yesterday; d = d.AddDate(0, 0, 1) {
		dateStr := d.Format(DateFormat)
		if !m.HasExperience(dateStr) {
			pendingDates = append(pendingDates, dateStr)
		}
	}

	return pendingDates
}

// scanSessionDates 扫描有 session 数据的日期
func (m *Manager) scanSessionDates() []string {
	baseDir := getBaseDir()
	projectsDir := filepath.Join(baseDir, "projects")
	sanitized := sanitizePath(m.workingDir)
	sessionsDir := filepath.Join(projectsDir, sanitized, "session")

	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Infof("[Experience] No sessions directory found: %s", sessionsDir)
			return nil
		}
		logger.Errorf("[Experience] Failed to read sessions directory: %v", err)
		return nil
	}

	// 收集所有有 session 数据的日期
	dateSet := make(map[string]bool)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		sessionPath := filepath.Join(sessionsDir, entry.Name())
		tfile := transcript.NewTranscriptFile(sessionPath)
		info, err := tfile.Replay()
		if err != nil {
			continue
		}

		// 从 records 中提取日期
		for _, record := range info.Records {
			t := time.UnixMilli(record.Timestamp)
			dateStr := t.Format(DateFormat)
			dateSet[dateStr] = true
		}
	}

	// 转换为排序的日期列表
	var dates []string
	for date := range dateSet {
		dates = append(dates, date)
	}
	sort.Strings(dates)

	// 过滤掉已经有 experience 的日期和今天的日期
	yesterday := time.Now().AddDate(0, 0, -1).Format(DateFormat)
	var pendingDates []string
	for _, date := range dates {
		if date > yesterday {
			continue
		}
		if !m.HasExperience(date) {
			pendingDates = append(pendingDates, date)
		}
	}

	logger.Infof("[Experience] Scanned %d session dates, %d pending", len(dates), len(pendingDates))
	return pendingDates
}

// ManualTriggerSummary 手动触发指定日期的总结
func (m *Manager) ManualTriggerSummary(ctx context.Context, date string) error {
	if _, err := time.Parse(DateFormat, date); err != nil {
		return fmt.Errorf("invalid date format: %w", err)
	}

	if m.HasExperience(date) {
		return fmt.Errorf("experience for %s already exists", date)
	}

	return m.generateSummaryForDate(ctx, date)
}

// ForceRegenerateSummary 强制重新生成指定日期的总结
func (m *Manager) ForceRegenerateSummary(ctx context.Context, date string) error {
	if _, err := time.Parse(DateFormat, date); err != nil {
		return fmt.Errorf("invalid date format: %w", err)
	}

	if m.HasExperience(date) {
		if err := m.DeleteExperience(date); err != nil {
			return fmt.Errorf("failed to delete existing experience: %w", err)
		}
	}

	return m.generateSummaryForDate(ctx, date)
}

// GetManagerStats 获取管理器统计信息
func (m *Manager) GetManagerStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	files, _ := m.GetExperienceList()

	var totalSize int64
	for _, f := range files {
		totalSize += f.Size
	}

	return map[string]interface{}{
		"experience_count":      len(files),
		"total_size_bytes":      totalSize,
		"last_summary_date":     m.metadata.LastSummaryDate,
		"summary_history_count": len(m.metadata.SummaryHistory),
		"user_profile_count":    len(m.metadata.UserProfiles),
		"heartbeat_running":     m.hbManager != nil,
	}
}

// UpdateExperienceIndex 更新EXPERIENCE.md汇总文件
// 使用LLM根据现有汇总和新经验生成更新的汇总
func (m *Manager) UpdateExperienceIndex() error {
	indexFile := filepath.Join(m.experiencePath, "EXPERIENCE.md")

	// 读取现有的EXPERIENCE.md内容
	var existingSummary string
	if data, err := os.ReadFile(indexFile); err == nil {
		existingSummary = string(data)
	}

	// 获取最新的经验文件列表
	files, err := m.GetExperienceList()
	if err != nil {
		return fmt.Errorf("failed to get experience list: %w", err)
	}

	if len(files) == 0 {
		return nil
	}

	// 获取最新的经验内容（最后一条）
	latestFile := files[len(files)-1]
	latestContent, err := m.GetExperience(latestFile.Date)
	if err != nil {
		return fmt.Errorf("failed to read latest experience: %w", err)
	}

	// 使用LLM生成更新的汇总
	updatedSummary, err := m.generateUpdatedSummary(existingSummary, latestContent, latestFile.Date)
	if err != nil {
		logger.Errorf("[Experience] Failed to generate updated summary with LLM: %v", err)
		// 如果LLM失败，回退到简单合并
		return m.updateExperienceIndexSimple(files)
	}

	// 保存更新后的汇总
	if err := os.WriteFile(indexFile, []byte(updatedSummary), 0644); err != nil {
		return fmt.Errorf("failed to write EXPERIENCE.md: %w", err)
	}

	logger.Infof("[Experience] Updated EXPERIENCE.md with latest experience from %s", latestFile.Date)
	return nil
}

// generateUpdatedSummary 使用LLM生成更新的汇总
func (m *Manager) generateUpdatedSummary(existingSummary, newExperience, date string) (string, error) {
	if m.client == nil {
		return "", fmt.Errorf("API client is not configured")
	}

	prompt := buildUpdateSummaryPrompt(existingSummary, newExperience, date)

	systemPrompt := "你是一个经验总结专家。你的任务是根据现有的经验汇总和新的经验，生成一份更新后的综合汇总。"

	req := &api.MessageRequest{
		Model:     m.client.Model,
		MaxTokens: 4096,
		System:    systemPrompt,
		Messages: []api.MessageParam{
			{Role: "user", Content: prompt},
		},
	}

	resp, err := m.client.SendMessage(context.Background(), req)
	if err != nil {
		return "", fmt.Errorf("failed to call LLM: %w", err)
	}

	var summary strings.Builder
	for _, block := range resp.Content {
		if block.Type == "text" && block.Text != "" {
			summary.WriteString(block.Text)
		}
	}

	result := summary.String()
	if result == "" {
		return "", fmt.Errorf("LLM returned empty summary")
	}

	return result, nil
}

// buildUpdateSummaryPrompt 构建更新汇总的提示词
func buildUpdateSummaryPrompt(existingSummary, newExperience, date string) string {
	var prompt strings.Builder

	prompt.WriteString("你是一个经验总结专家。你的任务是根据现有的经验汇总和新的经验，生成一份更新后的综合汇总。\n\n")

	if existingSummary != "" {
		prompt.WriteString("## 现有的经验汇总：\n\n")
		prompt.WriteString(existingSummary)
		prompt.WriteString("\n\n")
	}

	prompt.WriteString("## 今天的新经验 (")
	prompt.WriteString(date)
	prompt.WriteString(")：\n\n")
	prompt.WriteString(newExperience)
	prompt.WriteString("\n\n")

	prompt.WriteString("## 任务要求：\n\n")
	prompt.WriteString("1. 合并现有汇总和新经验，生成一份综合性的经验汇总\n")
	prompt.WriteString("2. 去除重复内容，保留重要信息\n")
	prompt.WriteString("3. 按主题组织内容，而不是按日期罗列\n")
	prompt.WriteString("4. 使用中文撰写\n")
	prompt.WriteString("5. 使用Markdown格式，包含以下部分：\n")
	prompt.WriteString("   - # 经验总结汇总\n")
	prompt.WriteString("   - > 最后更新时间\n")
	prompt.WriteString("   - ## 用户特点\n")
	prompt.WriteString("   - ## 关注领域\n")
	prompt.WriteString("   - ## 工作偏好\n")
	prompt.WriteString("   - ## 重要背景\n")
	prompt.WriteString("   - ## 常见问题和解决方案\n")
	prompt.WriteString("   - ## 最佳实践\n")
	prompt.WriteString("\n")
	prompt.WriteString("请直接输出更新后的汇总内容，不要包含任何解释或说明：\n")

	return prompt.String()
}

// updateExperienceIndexSimple 简单合并方式（回退方案）
func (m *Manager) updateExperienceIndexSimple(files []ExperienceFile) error {
	var content strings.Builder
	content.WriteString("# 经验总结汇总\n\n")
	content.WriteString(fmt.Sprintf("> 最后更新: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
	content.WriteString(fmt.Sprintf("共 %d 条经验记录\n\n", len(files)))
	content.WriteString("---\n\n")

	for i, f := range files {
		expContent, err := m.GetExperience(f.Date)
		if err != nil {
			logger.Errorf("[Experience] Failed to read %s: %v", f.Date, err)
			continue
		}

		content.WriteString(fmt.Sprintf("## %s\n\n", f.Date))

		lines := strings.Split(expContent, "\n")
		startIdx := 0
		for idx, line := range lines {
			if strings.HasPrefix(line, "# ") {
				startIdx = idx + 1
				break
			}
		}

		if startIdx < len(lines) {
			actualContent := strings.Join(lines[startIdx:], "\n")
			if idx := strings.Index(actualContent, "\n\n---"); idx > 0 {
				actualContent = actualContent[:idx]
			}
			content.WriteString(strings.TrimSpace(actualContent))
		}

		content.WriteString("\n\n")

		if i < len(files)-1 {
			content.WriteString("---\n\n")
		}
	}

	indexFile := filepath.Join(m.experiencePath, "EXPERIENCE.md")
	if err := os.WriteFile(indexFile, []byte(content.String()), 0644); err != nil {
		return fmt.Errorf("failed to write EXPERIENCE.md: %w", err)
	}

	logger.Infof("[Experience] Updated EXPERIENCE.md with %d entries (simple mode)", len(files))
	return nil
}

// GetLastSummaryDate 获取最后总结日期
func (m *Manager) GetLastSummaryDate() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.metadata.LastSummaryDate
}
