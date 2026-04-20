// Package channel defines the common interface for all messaging channels.
package channel

import (
	"context"
	"fmt"
	"sync"

	"dogclaw/pkg/query"
)

// EngineFactory creates a new QueryEngine for channel sessions
// channelName is the name of the channel (e.g. "qq", "weixin", "cli") to isolate sessions
type EngineFactory func(channelName string) *query.QueryEngine

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

	// SystemPrompt returns channel-specific system prompt for LLM
	// This tells the LLM how to use this channel's capabilities
	SystemPrompt() string
}

// Sender is the capability to proactively push text messages.
type Sender interface {
	// Send pushes a message to a specific chat target.
	// chatID is channel-specific (e.g. user_id, "group:xxx").
	// If chatID is empty, sends to all known active sessions.
	Send(ctx context.Context, chatID, message string) error
}

// FileSender is the capability to send files to users.
type FileSender interface {
	Sender
	// SendFile sends a file to a specific chat target.
	// chatID is channel-specific (e.g. user_id, "group:xxx").
	// fileType: 1=image, 2=video, 3=voice, 4=file
	// fileURL is the HTTP/HTTPS URL of the file to send.
	SendFile(ctx context.Context, chatID string, fileType int, fileURL, fileName string) error
}

// FileType constants for SendFile
const (
	FileTypeImage = 1
	FileTypeVideo = 2
	FileTypeVoice = 3
	FileTypeFile  = 4
)

// ActiveChatter is the capability to report active chat IDs.
type ActiveChatter interface {
	// ActiveChatIDs returns the list of currently active chat IDs.
	ActiveChatIDs() []string
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

func (r *Registry) SendFile(ctx context.Context, channelName, chatID string, fileType int, fileURL, fileName string) error {
	r.mu.RLock()
	s, ok := r.channels[channelName]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf("channel %s not found in registry (available: %v)", channelName, r.GetChannels())
	}

	fs, ok := s.(FileSender)
	if !ok {
		return fmt.Errorf("channel %s does not support file sending", channelName)
	}
	return fs.SendFile(ctx, chatID, fileType, fileURL, fileName)
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

func (r *Registry) GetFileSender(channelName string) (FileSender, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.channels[channelName]
	if !ok {
		return nil, false
	}
	fs, ok := s.(FileSender)
	return fs, ok
}

func (r *Registry) ActiveChatIDs(channelName string) []string {
	r.mu.RLock()
	s, ok := r.channels[channelName]
	r.mu.RUnlock()

	if !ok {
		return nil
	}
	ac, ok := s.(ActiveChatter)
	if !ok {
		return nil
	}
	return ac.ActiveChatIDs()
}
