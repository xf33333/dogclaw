// Package channel defines the common interface for all messaging channels.
package channel

import (
	"context"

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
