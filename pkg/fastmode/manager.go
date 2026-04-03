package fastmode

import (
	"sync"
	"time"
)

// State represents the current state of Fast Mode
type State string

const (
	StateActive   State = "active"
	StateCooldown State = "cooldown"
	StateDisabled State = "disabled"
)

// Manager handles Fast Mode state transitions
type Manager struct {
	mu            sync.RWMutex
	enabled       bool
	state         State
	cooldownUntil time.Time
	modelName     string // e.g., "opus[1m]"
}

// NewManager creates a new Fast Mode manager
func NewManager(enabled bool) *Manager {
	return &Manager{
		enabled:   enabled,
		state:     StateActive,
		modelName: "", // Will be set to the user's model
	}
}

// IsEnabled checks if fast mode is globally enabled
func (m *Manager) IsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled
}

// IsActive checks if fast mode is currently active (not in cooldown)
func (m *Manager) IsActive() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.state == StateCooldown {
		if time.Now().After(m.cooldownUntil) {
			return true // Cooldown expired
		}
		return false
	}
	return m.state == StateActive && m.enabled
}

// GetModel returns the model name to use for fast mode
func (m *Manager) GetModel() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.modelName
}

// SetModel sets the fast mode model
func (m *Manager) SetModel(model string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.modelName = model
}

// EnterCooldown triggers cooldown state (e.g., after rate limit)
func (m *Manager) EnterCooldown(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = StateCooldown
	m.cooldownUntil = time.Now().Add(duration)
}

// ExitCooldown manually exits cooldown
func (m *Manager) ExitCooldown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == StateCooldown {
		m.state = StateActive
	}
}

// Disable permanently disables fast mode
func (m *Manager) Disable() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = false
	m.state = StateDisabled
}

// GetState returns the current state
func (m *Manager) GetState() State {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.state == StateCooldown && time.Now().After(m.cooldownUntil) {
		return StateActive
	}
	return m.state
}

// TimeUntilCooldownEnd returns time remaining in cooldown
func (m *Manager) TimeUntilCooldownEnd() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.state != StateCooldown {
		return 0
	}
	remaining := time.Until(m.cooldownUntil)
	if remaining < 0 {
		return 0
	}
	return remaining
}
