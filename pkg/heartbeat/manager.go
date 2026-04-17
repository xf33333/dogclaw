// Package heartbeat 提供可扩展的心跳系统，允许其他模块注册定时任务
package heartbeat

import (
	"context"
	"fmt"
	"sync"
	"time"

	"dogclaw/internal/logger"
)

// Task 心跳任务接口
type Task interface {
	// Name 任务名称，用于标识和日志
	Name() string
	// Execute 执行任务，传入当前检查日期（yyyy-mm-dd格式）
	Execute(checkDate string) error
	// Interval 任务执行间隔，如果返回0则使用管理器的默认间隔
	Interval() time.Duration
}

// TaskFunc 任务函数类型
type TaskFunc func(checkDate string) error

// simpleTask 简单的任务实现
type simpleTask struct {
	name     string
	interval time.Duration
	fn       TaskFunc
}

func (t *simpleTask) Name() string              { return t.name }
func (t *simpleTask) Execute(date string) error { return t.fn(date) }
func (t *simpleTask) Interval() time.Duration   { return t.interval }

// Manager 心跳管理器
type Manager struct {
	tasks         map[string]Task
	mu            sync.RWMutex
	interval      time.Duration
	lastCheckDate string
	running       bool
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// Config 心跳管理器配置
type Config struct {
	// Interval 默认心跳间隔，默认为5分钟
	Interval time.Duration
	// StartImmediately 是否立即启动
	StartImmediately bool
	// LastCheckDate 最后检查日期，如果为空则使用今天
	LastCheckDate string
}

// NewManager 创建新的心跳管理器
func NewManager(cfg *Config) *Manager {
	interval := 5 * time.Minute
	if cfg != nil && cfg.Interval > 0 {
		interval = cfg.Interval
	}

	lastCheckDate := time.Now().Format("2006-01-02")
	if cfg != nil && cfg.LastCheckDate != "" {
		lastCheckDate = cfg.LastCheckDate
	}

	m := &Manager{
		tasks:         make(map[string]Task),
		interval:      interval,
		lastCheckDate: lastCheckDate,
	}

	if cfg != nil && cfg.StartImmediately {
		m.Start()
	}

	return m
}

// Register 注册任务
func (m *Manager) Register(task Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.tasks[task.Name()]; exists {
		return fmt.Errorf("task '%s' already registered", task.Name())
	}

	m.tasks[task.Name()] = task
	logger.Infof("[Heartbeat] Task '%s' registered", task.Name())
	return nil
}

// RegisterFunc 使用函数注册任务（便捷方法）
func (m *Manager) RegisterFunc(name string, interval time.Duration, fn TaskFunc) error {
	task := &simpleTask{
		name:     name,
		interval: interval,
		fn:       fn,
	}
	return m.Register(task)
}

// Unregister 注销任务
func (m *Manager) Unregister(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.tasks[name]; exists {
		delete(m.tasks, name)
		logger.Infof("[Heartbeat] Task '%s' unregistered", name)
	}
}

// GetTask 获取任务
func (m *Manager) GetTask(name string) (Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	task, exists := m.tasks[name]
	return task, exists
}

// GetAllTasks 获取所有任务
func (m *Manager) GetAllTasks() []Task {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tasks := make([]Task, 0, len(m.tasks))
	for _, task := range m.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

// Start 启动心跳
func (m *Manager) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.mu.Unlock()

	logger.Info("[Heartbeat] Manager started")

	// 立即执行一次心跳
	m.executeHeartbeat()

	// 启动定时器
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				m.executeHeartbeat()
			case <-ctx.Done():
				logger.Info("[Heartbeat] Manager stopped")
				return
			}
		}
	}()
}

// Stop 停止心跳
func (m *Manager) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}

	if m.cancel != nil {
		m.cancel()
	}
	m.running = false
	m.mu.Unlock()

	// 等待goroutine结束
	m.wg.Wait()
	logger.Info("[Heartbeat] Manager stopped completely")
}

// IsRunning 检查是否正在运行
func (m *Manager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// SetInterval 设置心跳间隔（需要重启生效）
func (m *Manager) SetInterval(interval time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.interval = interval
}

// GetInterval 获取心跳间隔
func (m *Manager) GetInterval() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.interval
}

// executeHeartbeat 执行心跳
func (m *Manager) executeHeartbeat() {
	now := time.Now()
	today := now.Format("2006-01-02")

	m.mu.RLock()
	lastCheckDate := m.lastCheckDate
	m.mu.RUnlock()

	// 如果日期没有变化，不需要执行
	if lastCheckDate == today {
		logger.Debugf("[Heartbeat] No date change, skipping heartbeat")
		return
	}

	logger.Infof("[Heartbeat] Date changed from %s to %s, executing tasks", lastCheckDate, today)

	// 更新最后检查日期
	m.mu.Lock()
	m.lastCheckDate = today
	m.mu.Unlock()

	// 执行所有注册的任务
	tasks := m.GetAllTasks()

	for _, task := range tasks {
		interval := task.Interval()
		if interval == 0 {
			interval = m.GetInterval()
		}

		logger.Infof("[Heartbeat] Executing task '%s'", task.Name())

		go func(t Task) {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("[Heartbeat] Task '%s' panicked: %v", t.Name(), r)
				}
			}()

			if err := t.Execute(today); err != nil {
				logger.Errorf("[Heartbeat] Task '%s' failed: %v", t.Name(), err)
			} else {
				logger.Infof("[Heartbeat] Task '%s' completed successfully", t.Name())
			}
		}(task)
	}
}

// Stats 获取统计信息
func (m *Manager) Stats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"running":         m.running,
		"interval_ms":     m.interval.Milliseconds(),
		"last_check_date": m.lastCheckDate,
		"task_count":      len(m.tasks),
	}
}
