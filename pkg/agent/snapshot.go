package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dogclaw/pkg/types"
)

// ========== Agent Memory Snapshot ==========

// SnapshotMetadata 快照元数据
type SnapshotMetadata struct {
	SnapshotID  string `json:"snapshotId"`
	AgentID     string `json:"agentId"`
	CreatedAt   int64  `json:"createdAt"`
	Description string `json:"description,omitempty"`
	MemoryCount int    `json:"memoryCount"` // 快照中的记忆条数
	TotalSize   int64  `json:"totalSize"`   // 总大小（字节）
	Version     string `json:"version"`     // 快照格式版本
}

// MemorySnapshot 记忆快照
type MemorySnapshot struct {
	Metadata     *SnapshotMetadata      `json:"metadata"`
	Entries      []MemoryEntry          `json:"entries"`              // 记忆条目
	AgentState   map[string]interface{} `json:"agentState,omitempty"` // 代理状态
	SerializedAt int64                  `json:"serializedAt"`
}

// SnapshotManager 管理记忆快照
type SnapshotManager struct {
	basePath     string
	agentID      string
	maxSnapshots int
	mu           sync.RWMutex
}

// NewSnapshotManager 创建新的快照管理器
func NewSnapshotManager(baseDir string, agentID string) (*SnapshotManager, error) {
	if agentID == "" {
		agentID = "default"
	}

	snapshotsDir := filepath.Join(baseDir, "snapshots", agentID)
	if err := os.MkdirAll(snapshotsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create snapshots directory: %w", err)
	}

	return &SnapshotManager{
		basePath:     snapshotsDir,
		agentID:      agentID,
		maxSnapshots: 10, // 默认保留10个快照
	}, nil
}

// CreateSnapshot 创建记忆快照
func (sm *SnapshotManager) CreateSnapshot(description string, memoryStore MemoryStore, extraState map[string]interface{}) (*MemorySnapshot, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 1. 获取所有记忆条目
	entries, err := memoryStore.Query(sm.agentID, types.MemoryProject, 0) // 0 = 无限制
	if err != nil {
		return nil, fmt.Errorf("failed to query memories: %w", err)
	}

	// 2. 计算大小
	totalSize := int64(0)
	for _, entry := range entries {
		// 粗略计算：每条记忆的JSON大小
		if data, err := json.Marshal(entry); err == nil {
			totalSize += int64(len(data))
		}
	}

	// 3. 生成快照ID
	snapshotID := generateSnapshotID()

	// 4. 创建快照
	now := time.Now().UnixNano() / 1e6
	snapshot := &MemorySnapshot{
		Metadata: &SnapshotMetadata{
			SnapshotID:  snapshotID,
			AgentID:     sm.agentID,
			CreatedAt:   now,
			Description: description,
			MemoryCount: len(entries),
			TotalSize:   totalSize,
			Version:     "1.0",
		},
		Entries:      entries,
		AgentState:   extraState,
		SerializedAt: now,
	}

	// 5. 序列化到文件
	filePath := filepath.Join(sm.basePath, snapshotID+".json")
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write snapshot file: %w", err)
	}

	// 6. 清理旧快照
	if err := sm.cleanupOldSnapshots(); err != nil {
		// 记录但不失败
		fmt.Printf("[WARN] Failed to cleanup old snapshots: %v\n", err)
	}

	return snapshot, nil
}

// LoadSnapshot 加载快照
func (sm *SnapshotManager) LoadSnapshot(snapshotID string) (*MemorySnapshot, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	filePath := filepath.Join(sm.basePath, snapshotID+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read snapshot file: %w", err)
	}

	var snapshot MemorySnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("failed to parse snapshot: %w", err)
	}

	// 验证快照
	if snapshot.Metadata == nil {
		return nil, fmt.Errorf("invalid snapshot: missing metadata")
	}
	if snapshot.Metadata.AgentID != sm.agentID {
		return nil, fmt.Errorf("snapshot belongs to different agent: expected %s, got %s",
			sm.agentID, snapshot.Metadata.AgentID)
	}

	return &snapshot, nil
}

// RestoreSnapshot 恢复快照到记忆存储
func (sm *SnapshotManager) RestoreSnapshot(snapshotID string, memoryStore MemoryStore) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 加载快照
	snapshot, err := sm.LoadSnapshot(snapshotID)
	if err != nil {
		return err
	}

	// 清除当前记忆
	if err := memoryStore.Clear(sm.agentID, types.MemoryProject); err != nil {
		return fmt.Errorf("failed to clear existing memories: %w", err)
	}

	// 恢复记忆条目
	for _, entry := range snapshot.Entries {
		if err := memoryStore.Add(entry); err != nil {
			return fmt.Errorf("failed to restore memory entry %s: %w", entry.ID, err)
		}
	}

	return nil
}

// ListSnapshots 列出所有快照
func (sm *SnapshotManager) ListSnapshots() ([]*SnapshotMetadata, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	entries, err := os.ReadDir(sm.basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []*SnapshotMetadata{}, nil
		}
		return nil, fmt.Errorf("failed to list snapshots: %w", err)
	}

	var snapshots []*SnapshotMetadata
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(sm.basePath, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var snapshot MemorySnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			continue
		}

		if snapshot.Metadata != nil {
			snapshots = append(snapshots, snapshot.Metadata)
		}
	}

	// 按创建时间倒序排序
	for i := 0; i < len(snapshots); i++ {
		for j := i + 1; j < len(snapshots); j++ {
			if snapshots[i].CreatedAt < snapshots[j].CreatedAt {
				snapshots[i], snapshots[j] = snapshots[j], snapshots[i]
			}
		}
	}

	return snapshots, nil
}

// DeleteSnapshot 删除快照
func (sm *SnapshotManager) DeleteSnapshot(snapshotID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	filePath := filepath.Join(sm.basePath, snapshotID+".json")
	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return nil // 已经删除
		}
		return fmt.Errorf("failed to delete snapshot: %w", err)
	}

	return nil
}

// GetSnapshotCount 获取快照数量
func (sm *SnapshotManager) GetSnapshotCount() (int, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	entries, err := os.ReadDir(sm.basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read snapshots directory: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".json") {
			count++
		}
	}
	return count, nil
}

// cleanupOldSnapshots 清理旧快照，保留最新的 maxSnapshots 个
func (sm *SnapshotManager) cleanupOldSnapshots() error {
	snapshots, err := sm.ListSnapshots()
	if err != nil {
		return err
	}

	if len(snapshots) <= sm.maxSnapshots {
		return nil
	}

	// 删除最旧的快照（保留最新的 maxSnapshots 个）
	toDelete := snapshots[sm.maxSnapshots:]
	for _, meta := range toDelete {
		if err := sm.DeleteSnapshot(meta.SnapshotID); err != nil {
			fmt.Printf("[WARN] Failed to delete old snapshot %s: %v\n", meta.SnapshotID, err)
		}
	}

	return nil
}

// SetMaxSnapshots 设置最大快照数量
func (sm *SnapshotManager) SetMaxSnapshots(max int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.maxSnapshots = max
}

// GetSnapshotPath 获取快照存储路径
func (sm *SnapshotManager) GetSnapshotPath() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.basePath
}

// ========== 工具函数 ==========

// generateSnapshotID 生成快照ID
func generateSnapshotID() string {
	timestamp := time.Now().UnixNano() / 1e6
	random := fmt.Sprintf("%06d", timestamp%1000000)
	return fmt.Sprintf("snap_%d_%s", timestamp, random)
}

// SnapshotDiff 比较两个快照之间的差异
type SnapshotDiff struct {
	AddedEntries    []MemoryEntry
	RemovedEntries  []MemoryEntry
	ModifiedEntries []MemoryEntry
}

// DiffSnapshots 比较两个快照
func DiffSnapshots(oldSnap, newSnap *MemorySnapshot) *SnapshotDiff {
	oldMap := make(map[string]MemoryEntry)
	newMap := make(map[string]MemoryEntry)

	for _, entry := range oldSnap.Entries {
		oldMap[entry.ID] = entry
	}
	for _, entry := range newSnap.Entries {
		newMap[entry.ID] = entry
	}

	diff := &SnapshotDiff{}

	// 查找新增的条目
	for id, newEntry := range newMap {
		if _, exists := oldMap[id]; !exists {
			diff.AddedEntries = append(diff.AddedEntries, newEntry)
		}
	}

	// 查找删除的条目
	for id, oldEntry := range oldMap {
		if _, exists := newMap[id]; !exists {
			diff.RemovedEntries = append(diff.RemovedEntries, oldEntry)
		}
	}

	// 查找修改的条目（简化：只比较内容字符串）
	for id, newEntry := range newMap {
		if oldEntry, exists := oldMap[id]; exists {
			if oldEntry.Content != newEntry.Content {
				diff.ModifiedEntries = append(diff.ModifiedEntries, newEntry)
			}
		}
	}

	return diff
}

// ExportSnapshot 导出快照为JSON字符串
func ExportSnapshot(snapshot *MemorySnapshot) (string, error) {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal snapshot for export: %w", err)
	}
	return string(data), nil
}

// ImportSnapshot 从JSON字符串导入快照
func ImportSnapshot(jsonData string) (*MemorySnapshot, error) {
	var snapshot MemorySnapshot
	if err := json.Unmarshal([]byte(jsonData), &snapshot); err != nil {
		return nil, fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}
	return &snapshot, nil
}

// ========== 全局快照管理器 ==========

var (
	globalSnapshotManager     *SnapshotManager
	globalSnapshotManagerOnce sync.Once
	globalSnapshotEnabled     = false
)

// InitializeGlobalSnapshots 初始化全局快照管理器
func InitializeGlobalSnapshots(basePath string, agentID string) error {
	var err error
	globalSnapshotManagerOnce.Do(func() {
		globalSnapshotManager, err = NewSnapshotManager(basePath, agentID)
	})
	globalSnapshotEnabled = (err == nil)
	return err
}

// GetGlobalSnapshotManager 获取全局快照管理器
func GetGlobalSnapshotManager() *SnapshotManager {
	if !globalSnapshotEnabled {
		return nil
	}
	return globalSnapshotManager
}

// IsSnapshotEnabled 检查快照功能是否启用
func IsSnapshotEnabled() bool {
	return globalSnapshotEnabled
}
