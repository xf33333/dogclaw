package cli

import (
	"context"
	"fmt"

	"dogclaw/pkg/channel"
)

// Channel implements a CLI "channel" that simply prints to stdout.
type Channel struct{}

func NewChannel() *Channel {
	return &Channel{}
}

func (c *Channel) Info() channel.Info {
	return channel.Info{Name: "cli"}
}

func (c *Channel) Start(ctx context.Context, factory channel.EngineFactory) error {
	// CLI is started externally by the main loop
	return nil
}

func (c *Channel) Stop() {}

func (c *Channel) Send(ctx context.Context, chatID, message string) error {
	fmt.Printf("\n🔔 [CLI Notification] %s\n", message)
	return nil
}

// Assert Channel implements channel.Interface and channel.Sender
var _ channel.Interface = (*Channel)(nil)
var _ channel.Sender = (*Channel)(nil)
