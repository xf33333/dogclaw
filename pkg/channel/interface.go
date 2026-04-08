// Package channel defines the common interface for all messaging channels.
package channel

import (
	"context"
	"fmt"
	"sync"

	"dogclaw/pkg/query"
)

// EngineFactory creates a new QueryEngine for channel sessions
type EngineFactory func() *query.QueryEngine

// Info holds metadata about a channel instance
type Info struct {
	Name string // e.g. "qq", "wechat", "telegram"
}

// Interface is the common interface for all messaging channels (QQ, WeChat, etc.)
type Interface interface {
	// Info returns channel metadata
	Info() Info

	// Start initializes and runs the channel
	Start(ctx context.Context, factory EngineFactory) error

	// Stop shuts down the channel gracefully
	Stop()
}

// Sender is the capability to proactively push text messages.
type Sender interface {
	// Send pushes a message to a specific chat target.
	// chatID is channel-specific (e.g. user_id, "group:xxx").
	// If chatID is empty, sends to all known active sessions.
	Send(ctx context.Context, chatID, message string) error
}

// Registry keeps a record of all running channels that support proactive sending.
type Registry struct {
	mu       sync.RWMutex
	channels map[string]Sender
}

func NewRegistry() *Registry {
	return &Registry{
		channels: make(map[string]Sender),
	}
}

func (r *Registry) Register(name string, s Sender) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channels[name] = s
}

func (r *Registry) Send(ctx context.Context, channelName, chatID, message string) error {
	r.mu.RLock()
	s, ok := r.channels[channelName]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf("channel %s not found in registry", channelName)
	}
	return s.Send(ctx, chatID, message)
}

func (r *Registry) GetChannels() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var names []string
	for name := range r.channels {
		names = append(names, name)
	}
	return names
}
