package weixin

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dogclaw/internal/config"
	"dogclaw/internal/logger"
	"dogclaw/pkg/query"
)

var testMode = false

func RunContinuousSendTest() error {
	settings, err := config.LoadSettings()
	if err != nil {
		return fmt.Errorf("failed to load settings: %w", err)
	}

	wxSettings := config.WeixinSettingsFromEnv(settings)
	if !wxSettings.Enabled || wxSettings.Token == "" {
		return fmt.Errorf("weixin channel not configured. Run 'dogclaw onboard' first.")
	}

	testMode = true

	ch, err := NewWeixinChannel(wxSettings)
	if err != nil {
		return fmt.Errorf("failed to create weixin channel: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("[weixin-test] Received shutdown signal")
		cancel()
	}()

	logger.Info("[weixin-test] Starting continuous send test...")
	logger.Info("[weixin-test] Send any message to your bot, it will reply with numbers 1-30")

	err = ch.Start(ctx, func(channelName string) *query.QueryEngine {
		return nil
	})
	if err != nil {
		return fmt.Errorf("channel error: %w", err)
	}

	<-ctx.Done()
	logger.Info("[weixin-test] Shutting down...")
	ch.Stop()

	return nil
}

func (c *WeixinChannel) handleTestMessage(ctx context.Context, fromUserID string) {
	logger.Info("[weixin-test] Starting to send numbers 1-30...")

	go func() {
		for i := 1; i <= 30; i++ {
			select {
			case <-ctx.Done():
				return
			default:
			}

			msg := fmt.Sprintf("%d", i)
			logger.Infof("[weixin-test] Sending: %s", msg)
			c.sendMessage(ctx, fromUserID, msg)
			time.Sleep(1500 * time.Millisecond)
		}
		logger.Info("[weixin-test] Finished sending 1-30")
	}()
}
