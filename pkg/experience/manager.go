// Package experience 提供经验系统功能，包括经验的存储、检索、心跳检查和自动总结
package experience

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dogclaw/internal/api"
	"dogclaw/internal/logger"
	"dogclaw/pkg/heartbeat"
)

const (
	ExperienceDirName = "experience"
	DateFormat        = "2006-01-02"
	MetadataFileName  = "metadata.json"
)

type SummaryStatus string

const (
	SummaryStatusPending    SummaryStatus = "pending"
	SummaryStatusProcessing SummaryStatus = "processing"
	SummaryStatusCompleted  SummaryStatus = "completed"
	SummaryStatusFailed     SummaryStatus = "failed"
	SummaryStatusSkipped    SummaryStatus = "skipped"
)

type ExperienceMetadata struct {
	Version         string                  `json:"version"`
	CreatedAt       time.Time               `json:"created_at"`
	UpdatedAt       time.Time               `json:"updated_at"`
	LastSummaryDate string                  `json:"last_summary_date"`
	SummaryHistory  []SummaryRecord         `json:"summary_history"`
	UserProfiles    map[string]*UserProfile `json:"user_profiles"`
}

type SummaryRecord struct {
	Date      string        `json:"date"`
	Status    SummaryStatus `json:"status"`
	StartedAt *time.Time    `json:"started_at,omitempty"`
	EndedAt   *time.Time    `json:"ended_at,omitempty"`
	ErrorMsg  string        `json:"error_msg,omitempty"`
	Sessions  []string      `json:"sessions,omitempty"`
}

type UserProfile struct {
	UserID       string            `json:"user_id"`
	Channel      string            `json:"channel"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	Traits       []string          `json:"traits"`
	Interests    []string          `json:"interests"`
	Preferences  map[string]string `json:"preferences"`
	SummaryCount int               `json:"summary_count"`
}

type ExperienceFile struct {
	Date     string    `json:"date"`
	FilePath string    `json:"file_path"`
	Size     int64     `json:"size"`
	ModTime  time.Time `json:"mod_time"`
}

// Manager 经验系统管理器
type Manager struct {
	experiencePath string
	workingDir     string // 工作目录（用于定位transcript文件）
	metadata       *ExperienceMetadata
	mu             sync.RWMutex
	client         *api.Client
	hbManager      *heartbeat.Manager // 心跳管理器引用
}

// ManagerConfig 管理器配置
type ManagerConfig struct {
	WorkingDir string // 工作目录（用于计算experience路径）
	APIClient  *api.Client
	HBManager  *heartbeat.Manager // 心跳管理器（可选，提供则自动注册）
}

// getBaseDir 获取基础目录
func getBaseDir() string {
	if dir := os.Getenv("CLAUDE_CODE_REMOTE_MEMORY_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".dogclaw")
}

// sanitizePath 清理路径用于作为目录名
func sanitizePath(path string) string {
	result := strings.ReplaceAll(path, string(filepath.Separator), "_")
	result = strings.ReplaceAll(result, "/", "_")
	result = strings.ReplaceAll(result, "\\", "_")
	result = strings.Trim(result, "_")
	for strings.Contains(result, "__") {
		result = strings.ReplaceAll(result, "__", "_")
	}
	if result == "" {
		result = "root"
	}
	return result
}

// GetExperiencePath 获取经验目录路径（与memory同级）
func GetExperiencePath(workingDir ...string) string {
	var cwd string
	if len(workingDir) > 0 && workingDir[0] != "" {
		cwd = workingDir[0]
	} else {
		cwd, _ = os.Getwd()
	}

	projectsDir := filepath.Join(getBaseDir(), "projects")
	sanitized := sanitizePath(cwd)
	return filepath.Join(projectsDir, sanitized, ExperienceDirName)
}

// NewManager 创建新的经验系统管理器
func NewManager(cfg *ManagerConfig) (*Manager, error) {
	experiencePath := GetExperiencePath(cfg.WorkingDir)

	if err := os.MkdirAll(experiencePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create experience directory: %w", err)
	}

	m := &Manager{
		experiencePath: experiencePath,
		workingDir:     cfg.WorkingDir,
		client:         cfg.APIClient,
		hbManager:      cfg.HBManager,
	}

	if err := m.loadMetadata(); err != nil {
		return nil, fmt.Errorf("failed to load metadata: %w", err)
	}

	// 如果提供了心跳管理器，自动注册经验总结任务
	if cfg.HBManager != nil {
		if err := m.registerWithHeartbeat(); err != nil {
			logger.Errorf("[Experience] Failed to register with heartbeat: %v", err)
		}
	}

	return m, nil
}

// registerWithHeartbeat 注册到心跳管理器
func (m *Manager) registerWithHeartbeat() error {
	task := &ExperienceHeartbeatTask{
		manager: m,
	}

	if err := m.hbManager.Register(task); err != nil {
		return err
	}

	logger.Info("[Experience] Registered with heartbeat manager")
	return nil
}

// SetHeartbeatManager 设置心跳管理器并注册任务
func (m *Manager) SetHeartbeatManager(hbManager *heartbeat.Manager) error {
	m.hbManager = hbManager
	return m.registerWithHeartbeat()
}

// ExperienceHeartbeatTask 经验系统心跳任务
type ExperienceHeartbeatTask struct {
	manager *Manager
}

// Name 任务名称
func (t *ExperienceHeartbeatTask) Name() string {
	return "experience-summary"
}

// Execute 执行任务
func (t *ExperienceHeartbeatTask) Execute(checkDate string) error {
	// 检查需要总结的日期
	pendingDates := t.manager.GetPendingSummaries()

	if len(pendingDates) == 0 {
		logger.Debugf("[Experience] No pending summaries")
		return nil
	}

	logger.Infof("[Experience] Found %d pending summary dates: %v", len(pendingDates), pendingDates)

	// 依次处理每个待总结的日期
	for _, date := range pendingDates {
		if err := t.manager.generateSummaryForDate(context.Background(), date); err != nil {
			logger.Errorf("[Experience] Failed to generate summary for %s: %v", date, err)
			// 继续处理其他日期
		}
	}

	return nil
}

// Interval 任务间隔（返回30分钟）
func (t *ExperienceHeartbeatTask) Interval() time.Duration {
	return 30 * time.Minute
}

func (m *Manager) loadMetadata() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	metadataPath := filepath.Join(m.experiencePath, MetadataFileName)

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			m.metadata = &ExperienceMetadata{
				Version:        "1.0",
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
				SummaryHistory: make([]SummaryRecord, 0),
				UserProfiles:   make(map[string]*UserProfile),
			}
			return m.saveMetadataLocked()
		}
		return fmt.Errorf("failed to read metadata: %w", err)
	}

	var metadata ExperienceMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	m.metadata = &metadata
	return nil
}

func (m *Manager) saveMetadataLocked() error {
	m.metadata.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(m.metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	metadataPath := filepath.Join(m.experiencePath, MetadataFileName)
	if err := os.WriteFile(metadataPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}

// SaveMetadata 保存元数据
func (m *Manager) SaveMetadata() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveMetadataLocked()
}

// Close 关闭管理器
func (m *Manager) Close() error {
	return m.SaveMetadata()
}
