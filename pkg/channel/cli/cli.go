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

func (c *Channel) SystemPrompt() string {
	return `## CLI频道能力说明

### 概述
- CLI是命令行交互界面
- 直接在终端中与用户交互

### 消息发送
- 支持发送文本消息到终端输出`
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
