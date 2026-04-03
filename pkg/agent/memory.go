package agent

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dogclaw/pkg/types"
)

// ========== Agent Memory 系统（参考 agentMemory.ts） ==========

// MemoryEntry 记忆条目
type MemoryEntry struct {
	ID        string                 `json:"id"`
	Timestamp int64                  `json:"timestamp"`
	Type      string                 `json:"type"` // "fact", "preference", "note", "observation"
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	AgentID   string                 `json:"agentId,omitempty"` // 哪个代理添加的
}

// MemoryStore 记忆存储接口
type MemoryStore interface {
	// Add 添加记忆条目
	Add(entry MemoryEntry) error
	// Query 查询记忆
	Query(agentID string, scope types.AgentMemoryScope, limit int) ([]MemoryEntry, error)
	// Delete 删除记忆
	Delete(entryID string) error
	// Clear 清除指定代理的所有记忆
	Clear(agentID string, scope types.AgentMemoryScope) error
	// GetStoragePath 获取存储路径（用于持久化）
	GetStoragePath() string
}

// FileMemoryStore 基于文件的记忆存储
type FileMemoryStore struct {
	BasePath string
	scope    types.AgentMemoryScope
	agentID  string
	mu       sync.RWMutex
}

// NewFileMemoryStore 创建新的文件记忆存储
func NewFileMemoryStore(baseDir string, scope types.AgentMemoryScope, agentID string) (*FileMemoryStore, error) {
	if agentID == "" {
		agentID = "default"
	}

	storePath := filepath.Join(baseDir, string(scope), agentID)
	if err := os.MkdirAll(storePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create memory directory: %w", err)
	}

	return &FileMemoryStore{
		BasePath: baseDir,
		scope:    scope,
		agentID:  agentID,
	}, nil
}

// Add 添加记忆条目
func (f *FileMemoryStore) Add(entry MemoryEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// 确保有ID和时间戳
	if entry.ID == "" {
		entry.ID = generateULID()
	}
	if entry.Timestamp == 0 {
		entry.Timestamp = time.Now().UnixNano() / 1e6
	}

	// 设置代理ID
	if entry.AgentID == "" {
		entry.AgentID = f.agentID
	}

	// 序列化
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal memory entry: %w", err)
	}

	// 写入文件
	filePath := filepath.Join(f.BasePath, string(f.scope), f.agentID, entry.ID+".json")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write memory entry: %w", err)
	}

	return nil
}

// Query 查询记忆（简化版：返回所有条目并按时间排序）
func (f *FileMemoryStore) Query(agentID string, scope types.AgentMemoryScope, limit int) ([]MemoryEntry, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// 确定查询目录
	queryAgentID := agentID
	if queryAgentID == "" {
		queryAgentID = f.agentID
	}

	queryScope := scope
	if queryScope == "" {
		queryScope = f.scope
	}

	dirPath := filepath.Join(f.BasePath, string(queryScope), queryAgentID)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []MemoryEntry{}, nil
		}
		return nil, fmt.Errorf("failed to read memory directory: %w", err)
	}

	var results []MemoryEntry
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(dirPath, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue // 跳过无法读取的文件
		}

		var memEntry MemoryEntry
		if err := json.Unmarshal(data, &memEntry); err != nil {
			continue // 跳过无效文件
		}

		results = append(results, memEntry)
	}

	// 按时间倒序排序（最新的在前）
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].Timestamp < results[j].Timestamp {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	// 限制返回数量
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// Delete 删除记忆条目
func (f *FileMemoryStore) Delete(entryID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	filePath := filepath.Join(f.BasePath, string(f.scope), f.agentID, entryID+".json")
	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return nil // 已经删除
		}
		return fmt.Errorf("failed to delete memory entry: %w", err)
	}
	return nil
}

// Clear 清除指定代理的所有记忆
func (f *FileMemoryStore) Clear(agentID string, scope types.AgentMemoryScope) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	targetAgentID := agentID
	if targetAgentID == "" {
		targetAgentID = f.agentID
	}

	targetScope := scope
	if targetScope == "" {
		targetScope = f.scope
	}

	dirPath := filepath.Join(f.BasePath, string(targetScope), targetAgentID)
	return os.RemoveAll(dirPath)
}

// GetStoragePath 获取存储路径
func (f *FileMemoryStore) GetStoragePath() string {
	return filepath.Join(f.BasePath, string(f.scope), f.agentID)
}

// GetScope 获取记忆作用域
func (f *FileMemoryStore) GetScope() types.AgentMemoryScope {
	return f.scope
}

// GetAgentID 获取代理ID
func (f *FileMemoryStore) GetAgentID() string {
	return f.agentID
}

// ========== 内存管理器 ==========

// MemoryManager 管理多个内存存储
type MemoryManager struct {
	basePath     string
	stores       map[string]MemoryStore // key: scope/agentID
	defaultScope types.AgentMemoryScope
	mu           sync.RWMutex
}

// NewMemoryManager 创建新的内存管理器
func NewMemoryManager(basePath string, defaultScope types.AgentMemoryScope) (*MemoryManager, error) {
	if basePath == "" {
		// 使用默认路径
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		basePath = filepath.Join(home, ".dogclaw", "memory")
	}

	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create memory base directory: %w", err)
	}

	return &MemoryManager{
		basePath:     basePath,
		stores:       make(map[string]MemoryStore),
		defaultScope: defaultScope,
	}, nil
}

// GetStore 获取指定代理和范围的记忆存储
func (m *MemoryManager) GetStore(agentID string, scope types.AgentMemoryScope) (MemoryStore, error) {
	m.mu.RLock()
	key := fmt.Sprintf("%s/%s", scope, agentID)
	if store, ok := m.stores[key]; ok {
		m.mu.RUnlock()
		return store, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	// 双重检查
	if store, ok := m.stores[key]; ok {
		return store, nil
	}

	store, err := NewFileMemoryStore(m.basePath, scope, agentID)
	if err != nil {
		return nil, err
	}

	m.stores[key] = store
	return store, nil
}

// GetDefaultStore 获取默认记忆存储（用于当前会话）
func (m *MemoryManager) GetDefaultStore() (MemoryStore, error) {
	return m.GetStore("default", m.defaultScope)
}

// Close 关闭内存管理器
func (m *MemoryManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 目前只有文件存储，不需要特殊关闭
	return nil
}

// ========== 自动记忆功能 ==========

// AutoMemoryConfig 自动记忆配置
type AutoMemoryConfig struct {
	Enabled    bool                   `json:"enabled"`
	Scope      types.AgentMemoryScope `json:"scope"`
	MaxEntries int                    `json:"maxEntries"`    // 每个代理最大记忆数
	MaxSize    int64                  `json:"maxSize"`       // 总大小限制（字节）
	Patterns   []string               `json:"patterns"`      // 需要自动记忆的模式
	Tools      []string               `json:"requiredTools"` // 需要的工具
}

// DefaultAutoMemoryConfig 默认自动记忆配置
func DefaultAutoMemoryConfig() *AutoMemoryConfig {
	return &AutoMemoryConfig{
		Enabled:    false,
		Scope:      types.MemoryProject,
		MaxEntries: 1000,
		MaxSize:    10 * 1024 * 1024, // 10MB
	}
}

// ShouldAutoRemember 判断是否应该自动记住某些内容
// 这是一个简化的启发式方法，实际实现会更复杂
func ShouldAutoRemember(content string, config *AutoMemoryConfig) bool {
	if !config.Enabled {
		return false
	}

	// 简单启发式：包含关键词或者达到特定长度
	content = strings.ToLower(content)

	// 检查是否有重要信息模式
	importantPatterns := []string{
		"todo:", "note:", "important:", "remember:",
		"api:", "endpoint:", "config:", "secret:", "key:",
	}

	for _, pattern := range config.Patterns {
		if strings.Contains(content, pattern) {
			return true
		}
	}

	for _, pattern := range importantPatterns {
		if strings.Contains(content, pattern) {
			return true
		}
	}

	return false
}

// CreateMemoryEntry 创建记忆条目
func CreateMemoryEntry(content string, memType string, agentID string, metadata map[string]interface{}) MemoryEntry {
	return MemoryEntry{
		ID:        generateULID(),
		Timestamp: time.Now().UnixNano() / 1e6,
		Type:      memType,
		Content:   content,
		Metadata:  metadata,
		AgentID:   agentID,
	}
}

// generateULID 生成一个简单的 ULID（在实际项目中应该使用完整实现）
func generateULID() string {
	// 使用时间戳和随机数生成一个简单的ID
	now := time.Now()
	timestamp := now.UnixNano() / 1e6
	random := make([]byte, 10)
	// 注意：在实际应用中应该使用加密安全的随机数生成器
	for i := range random {
		random[i] = byte(timestamp % 256)
		timestamp /= 256
	}
	hash := md5.Sum([]byte(fmt.Sprintf("%d%s", now.UnixNano(), random)))
	return hex.EncodeToString(hash[:])[:26]
}

// ========== 全局状态 ==========

// Global memory manager instance (initialized on demand)
var (
	globalMemoryManager     *MemoryManager
	globalMemoryManagerOnce sync.Once
	globalMemoryEnabled     = false
)

// InitializeGlobalMemory 初始化全局内存管理器
func InitializeGlobalMemory(basePath string) error {
	var err error
	globalMemoryManagerOnce.Do(func() {
		globalMemoryManager, err = NewMemoryManager(basePath, types.MemoryProject)
	})
	globalMemoryEnabled = (err == nil)
	return err
}

// GetGlobalMemoryManager 获取全局内存管理器
func GetGlobalMemoryManager() *MemoryManager {
	if !globalMemoryEnabled {
		return nil
	}
	return globalMemoryManager
}

// IsMemoryEnabled 检查内存功能是否启用
func IsMemoryEnabled() bool {
	return globalMemoryEnabled
}
