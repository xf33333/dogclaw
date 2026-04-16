package experience

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dogclaw/internal/api"
	"dogclaw/internal/logger"
	"dogclaw/pkg/transcript"
)

// generateSummaryForDate 为指定日期生成经验总结
func (m *Manager) generateSummaryForDate(ctx context.Context, date string) error {
	logger.Infof("[Experience] Generating summary for %s...", date)

	if err := m.recordSummaryAttempt(date, SummaryStatusProcessing, nil, nil); err != nil {
		logger.Errorf("[Experience] Failed to record summary start: %v", err)
	}

	sessions, err := m.loadSessionsForDate(date)
	if err != nil {
		m.recordSummaryAttempt(date, SummaryStatusFailed, err, nil)
		return fmt.Errorf("failed to load sessions for %s: %w", date, err)
	}

	if len(sessions) == 0 {
		logger.Infof("[Experience] No sessions found for %s, skipping", date)
		m.recordSummaryAttempt(date, SummaryStatusSkipped, nil, nil)
		return nil
	}

	logger.Infof("[Experience] Found %d sessions for %s", len(sessions), date)

	summary, err := m.generateSummaryWithLLM(ctx, date, sessions)
	if err != nil {
		m.recordSummaryAttempt(date, SummaryStatusFailed, err, getSessionIDs(sessions))
		return fmt.Errorf("failed to generate summary with LLM: %w", err)
	}

	if err := m.SaveExperience(date, summary); err != nil {
		m.recordSummaryAttempt(date, SummaryStatusFailed, err, getSessionIDs(sessions))
		return fmt.Errorf("failed to save experience: %w", err)
	}

	m.recordSummaryAttempt(date, SummaryStatusCompleted, nil, getSessionIDs(sessions))

	logger.Infof("[Experience] Successfully generated summary for %s", date)

	if err := m.UpdateExperienceIndex(); err != nil {
		logger.Errorf("[Experience] Failed to update EXPERIENCE.md: %v", err)
	}

	return nil
}

// generateSummaryWithLLM 使用LLM生成总结
func (m *Manager) generateSummaryWithLLM(ctx context.Context, date string, sessions []*SessionData) (string, error) {
	if m.client == nil {
		return "", fmt.Errorf("API client is not configured")
	}

	prompt := buildSummaryPrompt(date, sessions)

	systemPrompt := `You are an expert user behavior analyst. Your task is to analyze user conversations and generate a comprehensive experience summary.

Please analyze the following aspects:
1. User traits and personality characteristics (e.g., technical depth, communication style, priorities)
2. Topics and technologies the user is interested in
3. Work patterns and preferences (e.g., code style, tool preferences)
4. Important context that would be useful for future interactions

Generate the summary in Chinese, structured as markdown with the following sections:
- ## 用户特点
- ## 关注领域
- ## 工作偏好
- ## 重要背景

Be concise but informative. Focus on patterns and characteristics that would help provide better assistance in future interactions.`

	req := &api.MessageRequest{
		Model:     m.client.Model,
		MaxTokens: 4096,
		System:    systemPrompt,
		Messages: []api.MessageParam{
			{Role: "user", Content: prompt},
		},
	}

	resp, err := m.client.SendMessage(ctx, req)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
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

// buildSummaryPrompt 构建总结提示词
func buildSummaryPrompt(date string, sessions []*SessionData) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("请分析以下 %s 的用户对话记录，生成经验总结。\n\n", date))
	prompt.WriteString(fmt.Sprintf("共有 %d 个会话，详细对话如下：\n\n", len(sessions)))

	for i, session := range sessions {
		prompt.WriteString(fmt.Sprintf("--- 会话 %d (ID: %s) ---\n\n", i+1, session.SessionID))

		for _, msg := range session.Messages {
			timestamp := time.UnixMilli(msg.Timestamp).Format("15:04:05")
			role := "用户"
			if msg.Type == "assistant" {
				role = "助手"
			}

			content := msg.Content
			if len(content) > 1000 {
				content = content[:1000] + "... [内容已截断]"
			}

			prompt.WriteString(fmt.Sprintf("[%s] %s: %s\n\n", timestamp, role, content))
		}

		prompt.WriteString("\n")
	}

	prompt.WriteString("--- 对话结束 ---\n\n")
	prompt.WriteString("请根据以上对话内容，按照系统提示的要求生成经验总结。")

	return prompt.String()
}

// loadSessionsForDate 加载指定日期的所有会话
func (m *Manager) loadSessionsForDate(date string) ([]*SessionData, error) {
	// 从transcript路径读取session数据
	// transcript文件存储在: ~/.dogclaw/projects/<sanitized-cwd>/session/
	baseDir := getBaseDir()
	projectsDir := filepath.Join(baseDir, "projects")
	sanitized := sanitizePath(m.workingDir)
	sessionsDir := filepath.Join(projectsDir, sanitized, "session")

	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Infof("[Experience] No sessions directory found: %s", sessionsDir)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	var sessions []*SessionData

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		sessionPath := filepath.Join(sessionsDir, entry.Name())
		session, err := m.loadSessionFromFile(sessionPath, date)
		if err != nil {
			logger.Errorf("[Experience] Failed to load session %s: %v", entry.Name(), err)
			continue
		}

		if session != nil {
			sessions = append(sessions, session)
		}
	}

	logger.Infof("[Experience] Loaded %d sessions from %s for date %s", len(sessions), sessionsDir, date)
	return sessions, nil
}

// loadSessionFromFile 从文件中加载指定日期的会话数据
func (m *Manager) loadSessionFromFile(path string, date string) (*SessionData, error) {
	tfile := transcript.NewTranscriptFile(path)
	info, err := tfile.Replay()
	if err != nil {
		return nil, fmt.Errorf("failed to replay transcript: %w", err)
	}

	dateStart, _ := time.Parse(DateFormat, date)
	dateEnd := dateStart.AddDate(0, 0, 1)

	startMs := dateStart.UnixMilli()
	endMs := dateEnd.UnixMilli()

	var messages []TranscriptMessage
	var userInfo *UserInfo
	sessionID := ""

	for _, record := range info.Records {
		if record.Timestamp < startMs || record.Timestamp >= endMs {
			continue
		}

		if sessionID == "" && record.SessionID != "" && record.SessionID != "meta" {
			sessionID = record.SessionID
		}

		if userInfo == nil {
			userInfo = &UserInfo{
				Channel: "cli",
				UserID:  record.SessionID,
			}
		}

		switch record.Type {
		case transcript.MessageTypeUser:
			messages = append(messages, TranscriptMessage{
				Type:      "user",
				Content:   record.Content,
				Timestamp: record.Timestamp,
			})
		case transcript.MessageTypeAssistant:
			messages = append(messages, TranscriptMessage{
				Type:      "assistant",
				Content:   record.Content,
				Timestamp: record.Timestamp,
			})
		}
	}

	if len(messages) == 0 {
		return nil, nil
	}

	return &SessionData{
		SessionID: sessionID,
		Messages:  messages,
		UserInfo:  userInfo,
	}, nil
}

// recordSummaryAttempt 记录总结尝试
func (m *Manager) recordSummaryAttempt(date string, status SummaryStatus, err error, sessions []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	record := SummaryRecord{
		Date:      date,
		Status:    status,
		StartedAt: &now,
		Sessions:  sessions,
	}

	if status == SummaryStatusCompleted || status == SummaryStatusFailed || status == SummaryStatusSkipped {
		record.EndedAt = &now
	}

	if err != nil {
		record.ErrorMsg = err.Error()
	}

	found := false
	for i, r := range m.metadata.SummaryHistory {
		if r.Date == date {
			m.metadata.SummaryHistory[i] = record
			found = true
			break
		}
	}

	if !found {
		m.metadata.SummaryHistory = append(m.metadata.SummaryHistory, record)
	}

	if status == SummaryStatusCompleted {
		m.metadata.LastSummaryDate = date
	}

	return m.saveMetadataLocked()
}

func getSessionIDs(sessions []*SessionData) []string {
	ids := make([]string, len(sessions))
	for i, s := range sessions {
		ids[i] = s.SessionID
	}
	return ids
}

// SessionData 会话数据
type SessionData struct {
	SessionID string
	Messages  []TranscriptMessage
	UserInfo  *UserInfo
}

// TranscriptMessage 转录消息
type TranscriptMessage struct {
	Type      string
	Content   string
	Timestamp int64
}

// UserInfo 用户信息
type UserInfo struct {
	Channel string
	UserID  string
	Name    string
}
